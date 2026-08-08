package services

import (
	"context"
	"fmt"
	"os"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// UpdateResult reports a single-entry update (contract/cli-json.md). before
// and after are content hashes excluding meta.json.
type UpdateResult struct {
	Before  string `json:"before"`
	After   string `json:"after"`
	Changed bool   `json:"changed"`
}

// excludeMeta is the update hash exclusion predicate (FR-023).
func excludeMeta(rel string) bool { return rel == "meta.json" }

// UpdateEntry replaces entry's content with a fresh fetched copy, validating
// first and preserving the old content on failure (FR-023). The comparison
// excludes meta.json; byte-identical content classifies as current.
func (r *Repository) UpdateEntry(ctx context.Context, entry *common.Entry, newCopy string) (*UpdateResult, error) {
	kind, name, err := r.ProbeStaged(newCopy)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if kind != entry.Kind || name != entry.Name {
		return nil, common.WithExitCode(
			fmt.Errorf("update: fetched copy is %s %q, expected %s %q", kind, name, entry.Kind, entry.Name),
			common.ExitObject)
	}

	tmp, err := r.stageCopy(newCopy)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	before, err := dal.DirHash(entry.Path, excludeMeta)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	after, err := dal.DirHash(tmp, excludeMeta)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}

	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	tx := &dal.FileTransaction{}
	if err := tx.BackupRemove(entry.Path); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if err := tx.MoveStage(tmp, entry.Path); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	// Preserve the existing origin (the fetched copy carries no meta.json).
	if entry.Origin != nil {
		if err := dal.WriteMeta(entry.Path, entry.Origin); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	tx.Commit()
	return &UpdateResult{Before: before, After: after, Changed: before != after}, nil
}

// CompareUpdate computes before/after hashes for an entry and a fetched copy
// without writing anything (dry-run / batch preview).
func (r *Repository) CompareUpdate(entry *common.Entry, newCopy string) (before, after string, changed bool, err error) {
	if _, _, err := r.ProbeStaged(newCopy); err != nil {
		return "", "", false, err
	}
	before, err = dal.DirHash(entry.Path, excludeMeta)
	if err != nil {
		return "", "", false, err
	}
	tmp, err := r.stageCopy(newCopy)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	after, err = dal.DirHash(tmp, excludeMeta)
	if err != nil {
		return "", "", false, err
	}
	return before, after, before != after, nil
}
