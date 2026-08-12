package services

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
)

// UpdateResult is the CLI JSON report for update (contract/cli-json.md).

// UpdateOptions controls a single update.
type UpdateOptions struct {
	DryRun bool
}

// Update refreshes a single entry from its origin. It fails (exit 1) when the
// entry is missing, not active, or has no origin (FR-023).
func (s *Services) Update(ctx context.Context, name string, opts UpdateOptions) (*UpdateResult, error) {
	entry := s.FindEntry(name)
	if entry == nil {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q not found", name), common.ExitObject)
	}
	if entry.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q is %s; only active entries can be updated", name, entry.Status), common.ExitObject)
	}
	if entry.Origin == nil {
		return nil, common.WithExitCode(fmt.Errorf("update: entry %q has no origin; nothing to fetch", name), common.ExitObject)
	}
	staged, cleanup, err := s.fetchFromOrigin(ctx, entry.Origin.Address)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if opts.DryRun {
		// Fetch + probe + hash comparison without replacing.
		before, after, changed, err := s.Repo.CompareUpdate(entry, staged)
		if err != nil {
			return nil, err
		}
		return &UpdateResult{Before: before, After: after, Changed: changed}, nil
	}
	return s.Repo.UpdateEntry(ctx, entry, staged)
}

// Updatable reports whether an entry can be refreshed from its origin (active
// and carries an origin). It is the eligibility rule shared by the CLI
// batch-update and the TUI's per-entry batch jobs, so "what can be updated"
// lives with the update domain instead of being re-derived in the
// presentation layer. (Update keeps its own two checks because it must report
// which half of the rule failed.)
func (s *Services) Updatable(e *common.Entry) bool {
	return e.Status == common.StatusActive && e.Origin != nil
}

// fetchFromOrigin fetches an address through the first matching provider.
func (s *Services) fetchFromOrigin(ctx context.Context, address string) (string, func(), error) {
	p := s.Registry.Match(address)
	if p == nil {
		return "", func() {}, common.WithExitCode(fmt.Errorf("no provider can handle %q", address), common.ExitError)
	}
	tmp, err := p.Fetch(ctx, address)
	if err != nil {
		return "", func() {}, common.WithExitCode(err, common.ExitError)
	}
	return tmp, fetchCleanup(p, tmp), nil
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
	Updated []string       `json:"updated"`
	Current []string       `json:"current"`
	Failed  []FailedUpdate `json:"failed"`
	Skipped []string       `json:"skipped"`
	Total   int            `json:"total"`
}

// BatchUpdate refreshes all active entries with an origin, classifying each as
// updated/current/failed/skipped (FR-024).
func (s *Services) BatchUpdate(ctx context.Context, dryRun bool) *BatchUpdateResult {
	res := &BatchUpdateResult{
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
			continue
		}
		staged, cleanup, err := s.fetchFromOrigin(ctx, e.Origin.Address)
		if err != nil {
			res.Failed = append(res.Failed, FailedUpdate{Name: e.Name, Reason: err.Error()})
			continue
		}
		var u *UpdateResult
		if dryRun {
			_, _, changed, cErr := s.Repo.CompareUpdate(e, staged)
			cleanup()
			if cErr != nil {
				res.Failed = append(res.Failed, FailedUpdate{Name: e.Name, Reason: cErr.Error()})
				continue
			}
			u = &UpdateResult{Changed: changed}
		} else {
			u, err = s.Repo.UpdateEntry(ctx, e, staged)
			cleanup()
			if err != nil {
				res.Failed = append(res.Failed, FailedUpdate{Name: e.Name, Reason: err.Error()})
				continue
			}
		}
		if u.Changed {
			res.Updated = append(res.Updated, e.Name)
		} else {
			res.Current = append(res.Current, e.Name)
		}
	}
	res.Total = len(res.Updated) + len(res.Current) + len(res.Failed) + len(res.Skipped)
	return res
}
