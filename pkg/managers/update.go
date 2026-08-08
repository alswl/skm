package managers

import (
	"context"
	"fmt"
	"os"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services/repository"
)

// UpdateResult is the CLI JSON report for update (contract/cli-json.md).
type UpdateResult = repository.UpdateResult

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
	return tmp, func() { _ = os.RemoveAll(tmp) }, nil
}

// BatchUpdateResult is the CLI JSON report for batch-update
// (contract/cli-json.md). Only active items are processed (FR-024).
type BatchUpdateResult struct {
	Updated []string `json:"updated"`
	Current []string `json:"current"`
	Failed  []string `json:"failed"`
	Skipped []string `json:"skipped"`
	Total   int      `json:"total"`
}

// BatchUpdate refreshes all active entries with an origin, classifying each as
// updated/current/failed/skipped (FR-024).
func (s *Services) BatchUpdate(ctx context.Context, dryRun bool) *BatchUpdateResult {
	res := &BatchUpdateResult{
		Updated: []string{},
		Current: []string{},
		Failed:  []string{},
		Skipped: []string{},
	}
	for _, e := range s.Scan() {
		if e.Status != common.StatusActive {
			continue
		}
		if e.Origin == nil {
			res.Skipped = append(res.Skipped, e.Name)
			continue
		}
		staged, cleanup, err := s.fetchFromOrigin(ctx, e.Origin.Address)
		if err != nil {
			res.Failed = append(res.Failed, e.Name)
			continue
		}
		var u *UpdateResult
		if dryRun {
			_, _, changed, cErr := s.Repo.CompareUpdate(e, staged)
			cleanup()
			if cErr != nil {
				res.Failed = append(res.Failed, e.Name)
				continue
			}
			u = &UpdateResult{Changed: changed}
		} else {
			u, err = s.Repo.UpdateEntry(ctx, e, staged)
			cleanup()
			if err != nil {
				res.Failed = append(res.Failed, e.Name)
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
