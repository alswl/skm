package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
)

// UpdateResult is the CLI JSON report for update (contract/cli-json.md).

// UpdateOptions controls a single update.
type UpdateOptions struct {
	DryRun bool
}

// fetchedOrigin is an immutable acquisition result.  In particular, fetching
// must not edit Entry.Origin: a failed candidate is not an accepted update.
type fetchedOrigin struct {
	Path    string
	Version string
	Cleanup func()
}

// Update refreshes a single entry from its origin. It fails (exit 1) when the
// entry is missing, not active, or has no origin (FR-023).
func (s *Services) Update(ctx context.Context, name string, opts UpdateOptions) (*engines.UpdateResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	if entry == nil {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q not found", name), common.ExitObject)
	}
	if entry.Status == common.StatusError && entry.Error != nil && strings.Contains(*entry.Error, "recovery_required:") {
		return nil, common.WithExitCode(fmt.Errorf("%s", *entry.Error), common.ExitObject)
	}
	if entry.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q is %s; only active entries can be updated", name, entry.Status), common.ExitObject)
	}
	if entry.Kind == common.KindSkill {
		return s.updateSkill(ctx, name, opts)
	}
	if entry.Origin == nil {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q has no origin; nothing to fetch", name), common.ExitObject)
	}
	fetched, err := s.fetchFromOrigin(ctx, entry, !opts.DryRun)
	if err != nil {
		return nil, err
	}
	defer fetched.Cleanup()

	if opts.DryRun {
		// Fetch + probe + hash comparison without replacing.
		before, after, changed, err := s.Repo.CompareUpdate(entry, fetched.Path)
		if err != nil {
			return nil, err
		}
		return &engines.UpdateResult{Before: before, After: after, Changed: changed}, nil
	}
	res, err := s.Repo.UpdateEntry(ctx, entry, fetched.Path)
	if err != nil {
		return nil, err
	}
	if err := s.refreshInstalls(ctx, entry); err != nil {
		return nil, fmt.Errorf("update: refresh installs: %w", err)
	}
	return res, nil
}

// refreshInstalls re-applies the entry's existing installs after a content
// update, so target-side copies pick up the new content — a command adapter
// keeps a copy of the entry marker, which the replacement just outdated.
// Only targets that already hold a managed install are touched
// (Installer.RefreshTargets); this refresh never claims new targets. It runs
// under the repository lock in one transaction, mirroring runInstall.
func (s *Services) refreshInstalls(ctx context.Context, entry *common.Entry) error {
	targets := s.Installer.RefreshTargets(entry)
	if len(targets) == 0 {
		return nil
	}
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()

	tx := &dal.FileTransaction{}
	for _, t := range targets {
		if _, err := s.Installer.Install(tx, entry, t, false); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// Updatable reports whether an entry can be refreshed from its origin (active
// and carries an origin). Local origins must point outside this repository:
// older skm versions could persist an origin pointing at another managed
// entry, which makes the entry self-referential once that source is moved or
// claimed. Those records have no external source to refresh from and must not
// be offered to the TUI batch-update action. The eligibility rule is shared
// by the CLI batch-update and the TUI's per-entry batch jobs, so "what can be
// updated" lives with the update domain instead of being re-derived in the
// presentation layer. (Update keeps its own checks to report which one
// failed.)
func (s *Services) Updatable(e *common.Entry) bool {
	if e.Status != common.StatusActive || e.Origin == nil || e.ProviderIDValue() == "unknown" || e.ProviderIDValue() == "self-build" {
		return false
	}
	return e.ProviderIDValue() != "local" || !isManagedRepositoryEntry(e.Origin.Address, s.Cfg.Root)
}

// fetchFromOrigin refreshes an entry through its recorded provider. Choosing
// by address would let a later provider with overlapping capabilities replace
// content from a different origin. Entries without a provider id are legacy
// metadata and retain the former address-matching fallback.
func (s *Services) fetchFromOrigin(ctx context.Context, entry *common.Entry, persistCache bool) (fetchedOrigin, error) {
	address := entry.Origin.Address
	p := s.Registry.Get(entry.ProviderIDValue())
	if entry.ProviderIDValue() != "" && p == nil {
		return fetchedOrigin{}, common.WithExitCode(
			fmt.Errorf("origin provider %q is unavailable for %q", entry.ProviderIDValue(), address), common.ExitError)
	}
	if p == nil {
		p = s.Registry.Match(address)
	}
	if p == nil {
		return fetchedOrigin{}, common.WithExitCode(fmt.Errorf("no provider can handle %q", address), common.ExitError)
	}
	if vp, ok := p.(providers.VersionedFetcher); ok {
		got, err := vp.FetchVersioned(ctx, providers.FetchRequest{Address: address, RepositoryAddress: entry.Origin.RepositoryAddress, RequestedRef: entry.Origin.RequestedRef, Subpath: entry.Origin.Subpath})
		if err != nil {
			return fetchedOrigin{}, err
		}
		cleanup := fetchCleanup(p, got.Path)
		if persistCache {
			cached, cacheErr := dal.StoreGitCache(s.Cfg.Root, got.RepositoryAddress, got.Revision, got.Path)
			if cacheErr != nil {
				cleanup()
				return fetchedOrigin{}, cacheErr
			}
			cleanup()
			got.Path = cached
			cleanup = func() {}
		}
		return fetchedOrigin{Path: filepath.Join(got.Path, got.ResolvedSubpath), Version: got.Revision, Cleanup: cleanup}, nil
	}
	tmp, err := p.Fetch(ctx, address)
	if err != nil {
		return fetchedOrigin{}, common.WithExitCode(err, common.ExitError)
	}
	cleanup := fetchCleanup(p, tmp)
	if entry.Origin.Subpath != "" {
		rel := entry.Origin.Subpath
		if !filepath.IsLocal(rel) {
			cleanup()
			return fetchedOrigin{}, fmt.Errorf("origin subpath must stay inside fetched source")
		}
		root, err := filepath.EvalSymlinks(tmp)
		var child string
		if err == nil {
			child, err = filepath.EvalSymlinks(filepath.Join(root, rel))
		}
		if err == nil {
			var relative string
			relative, err = filepath.Rel(root, child)
			if err == nil && !filepath.IsLocal(relative) {
				err = fmt.Errorf("origin subpath escapes fetched source")
			}
		}
		if err != nil {
			cleanup()
			return fetchedOrigin{}, err
		}
		return fetchedOrigin{Path: child, Cleanup: cleanup}, nil
	}
	return fetchedOrigin{Path: tmp, Cleanup: cleanup}, nil
}

// FailedUpdate is one batch-update failure: which entry, and why (the error
// that caused it, previously discarded — a user could see "failed=2" but not
// which two entries or the reason, FR-005).
type FailedUpdate struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// BatchUpdateResult is the CLI JSON report for batch-update
// (contract/cli-json.md). Only active items are processed (FR-024).
type BatchUpdateResult struct {
	Items    []*engines.UpdateResult `json:"items"`
	ExitCode int                     `json:"-"`
	Success  bool                    `json:"success"`
	Preview  bool                    `json:"preview"`
	Updated  []string                `json:"updated"`
	Current  []string                `json:"current"`
	Failed   []FailedUpdate          `json:"failed"`
	Skipped  []string                `json:"skipped"`
	Total    int                     `json:"total"`
}

// BatchUpdate refreshes all active entries with an origin, classifying each as
// updated/current/failed/skipped (FR-024).
func (s *Services) BatchUpdate(ctx context.Context, dryRun bool) *BatchUpdateResult {
	res := &BatchUpdateResult{Items: []*engines.UpdateResult{}, Success: true, Preview: dryRun,
		Updated: []string{},
		Current: []string{},
		Failed:  []FailedUpdate{},
		Skipped: []string{},
	}
	for _, e := range s.Scan() {
		if e.Status != common.StatusActive {
			continue
		}
		if !s.Updatable(e) { // active but originless: skipped, not failed
			res.Skipped = append(res.Skipped, e.Name)
			res.Items = append(res.Items, &engines.UpdateResult{Entry: s.Repo.RelPath(e.Path), Outcome: "not_applicable", Preview: dryRun})
			continue
		}
		u, err := s.Update(ctx, e.Path, UpdateOptions{DryRun: dryRun})
		if u == nil {
			u = &engines.UpdateResult{Entry: s.Repo.RelPath(e.Path), Outcome: "failed", Preview: dryRun}
			if err != nil {
				u.Reason = err.Error()
				u.ReasonCode = syncReasonCode(err)
			}
		}
		if u != nil {
			res.Items = append(res.Items, u)
		}
		if err != nil {
			res.Success = false
			code := common.ExitCodeOf(err, common.ExitError)
			if code > res.ExitCode {
				res.ExitCode = code
			}
			res.Failed = append(res.Failed, FailedUpdate{Name: e.Name, Reason: err.Error()})
			continue
		}

		if u.Changed || u.Outcome == "updated" {
			res.Updated = append(res.Updated, e.Name)
		} else {
			res.Current = append(res.Current, e.Name)
		}
	}
	res.Total = len(res.Updated) + len(res.Current) + len(res.Failed) + len(res.Skipped)
	return res
}
