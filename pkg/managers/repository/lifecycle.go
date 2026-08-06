package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// LifecycleOptions controls archive/unarchive/delete/convert.
type LifecycleOptions struct {
	Force  bool
	DryRun bool
}

// Archive moves an active entry into the archived tree, preserving the
// provider/group layout (FR-025).
func (r *Repository) Archive(ctx context.Context, entry *common.Entry, opts LifecycleOptions) (string, error) {
	if entry.Status != common.StatusActive {
		return "", common.WithExitCode(fmt.Errorf("archive: entry %q is %s; only active entries can be archived", entry.Name, entry.Status), common.ExitObject)
	}
	dest := r.archivedPath(entry)
	if opts.DryRun {
		return dest, nil
	}
	if err := r.moveUnderLock(ctx, entry.Path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Unarchive moves an archived entry back to its kind's active tree (FR-025).
func (r *Repository) Unarchive(ctx context.Context, entry *common.Entry, opts LifecycleOptions) (string, error) {
	if entry.Status != common.StatusArchived {
		return "", common.WithExitCode(fmt.Errorf("unarchive: entry %q is %s; only archived entries can be unarchived", entry.Name, entry.Status), common.ExitObject)
	}
	rel, err := filepath.Rel(filepath.Join(r.root, "archived"), entry.Path)
	if err != nil {
		return "", common.WithExitCode(err, common.ExitError)
	}
	dest := filepath.Join(r.root, entry.Kind.TopDir(), rel)
	if opts.DryRun {
		return dest, nil
	}
	if err := r.moveUnderLock(ctx, entry.Path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Delete permanently removes an entry from the repository (authorized by
// --force, FR-025/FR-031).
func (r *Repository) Delete(ctx context.Context, entry *common.Entry, opts LifecycleOptions) error {
	if !opts.Force {
		return common.WithExitCode(fmt.Errorf("delete: entry %q requires --force to be permanently removed", entry.Name), common.ExitObject)
	}
	if opts.DryRun {
		return nil
	}
	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()
	if err := os.RemoveAll(entry.Path); err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	return nil
}

// ConvertContent flips a directory entry's kind: rewrites the marker, moves it
// to the new-kind tree and removes origin (I3). Only active directory entries
// are convertible; single-file commands, archived and error entries fail
// without side effects (FR-026).
func (r *Repository) ConvertContent(ctx context.Context, entry *common.Entry, newKind common.EntryKind) (*common.Entry, error) {
	if entry.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("convert: entry %q is %s; only active entries can be converted", entry.Name, entry.Status), common.ExitObject)
	}
	if !entry.IsDirectory() {
		return nil, common.WithExitCode(fmt.Errorf("convert: entry %q is a single-file command and cannot be converted", entry.Name), common.ExitObject)
	}
	if entry.Kind == newKind {
		return nil, common.WithExitCode(fmt.Errorf("convert: entry %q is already a %s", entry.Name, newKind), common.ExitObject)
	}

	// Stage a new tree: copy, rename marker, drop meta.json.
	tmp, err := copyDirToTemp(entry.Path)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	oldMarker := entry.Kind.MarkerFile()
	newMarker := newKind.MarkerFile()
	if err := os.Rename(filepath.Join(tmp, oldMarker), filepath.Join(tmp, newMarker)); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("convert: rename marker: %w", err), common.ExitError)
	}
	if err := dal.RemoveMeta(tmp); err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}

	rel, err := filepath.Rel(r.root, entry.Path)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	rest := parts[1:] // drop old top kind dir
	dest := filepath.Join(r.root, newKind.TopDir(), filepath.Join(rest...))

	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	tx := &dal.FileTransaction{}
	// Remove the old-kind location and place the converted tree (rollback
	// restores the original entry on failure).
	if err := tx.BackupRemove(entry.Path); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if err := tx.MoveStage(tmp, dest); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	tx.Commit()

	return &common.Entry{
		Name:        entry.Name,
		Description: entry.Description,
		Kind:        newKind,
		Status:      common.StatusActive,
		Path:        dest,
		Version:     entry.Version,
		ModeID:      entry.ModeID,
		Group:       entry.Group,
	}, nil
}

// ConvertDest computes the destination path a conversion would move the entry
// to, without writing (dry-run).
func (r *Repository) ConvertDest(entry *common.Entry, newKind common.EntryKind) (string, error) {
	rel, err := filepath.Rel(r.root, entry.Path)
	if err != nil {
		return "", common.WithExitCode(err, common.ExitError)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	rest := parts[1:]
	return filepath.Join(r.root, newKind.TopDir(), filepath.Join(rest...)), nil
}

// archivedPath maps an active entry path into the archived tree preserving the
// provider/group layout.
func (r *Repository) archivedPath(entry *common.Entry) string {
	rel, err := filepath.Rel(r.root, entry.Path)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	rest := parts[1:] // drop "skills" / "commands"
	return filepath.Join(r.root, "archived", filepath.Join(rest...))
}

// moveUnderLock moves src to dest under a repository lock + transaction.
func (r *Repository) moveUnderLock(ctx context.Context, src, dest string) error {
	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()
	tx := &dal.FileTransaction{}
	if err := tx.MoveStage(src, dest); err != nil {
		_ = tx.Rollback()
		return common.WithExitCode(err, common.ExitError)
	}
	tx.Commit()
	return nil
}
