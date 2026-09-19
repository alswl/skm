package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// BackupSnapshot is a local-only recovery representation of the repository's
// entry directories. It is never a valid SharePayload and is never emitted by
// CreateShare (FR-017, data-model.md BackupSnapshot).
type BackupSnapshot struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Entries   []string  `json:"entries"`
}

type BackupCreateResult struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
}

type BackupRestoreItemResult struct {
	Path   string `json:"path"`
	Status string `json:"status"` // restored | skipped | failed
	Reason string `json:"reason,omitempty"`
}

type BackupRestoreResult struct {
	ID      string                    `json:"id"`
	Items   []BackupRestoreItemResult `json:"items"`
	Success bool                      `json:"success"`
}

const backupMetaFile = "backup.json"

func (s *Services) backupsDir() string {
	return filepath.Join(s.Cfg.ConfigDir, "backups")
}

// CreateBackup never produces or consumes a share payload; backup and share
// are deliberately separate local vs. cross-user mechanisms (research.md #5).
func (s *Services) CreateBackup(_ context.Context) (*BackupCreateResult, error) {
	id := time.Now().UTC().Format("20060102T150405Z")
	dest := filepath.Join(s.backupsDir(), id)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}

	var captured []string
	for _, top := range []string{"skills", "commands"} {
		src := filepath.Join(s.Cfg.Root, top)
		if !dal.PathExists(src) {
			continue
		}
		if err := copyBackupTree(src, filepath.Join(dest, top)); err != nil {
			_ = os.RemoveAll(dest)
			return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
		}
		captured = append(captured, top)
	}
	if len(captured) == 0 {
		_ = os.RemoveAll(dest)
		return nil, common.WithExitCode(fmt.Errorf("backup: no local repository content to back up"), common.ExitObject)
	}

	snapshot := BackupSnapshot{ID: id, CreatedAt: time.Now().UTC(), Entries: captured}
	if err := writeBackupMeta(dest, snapshot); err != nil {
		_ = os.RemoveAll(dest)
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}
	return &BackupCreateResult{ID: id, Path: dest, Entries: captured}, nil
}

// RestoreBackup reports an existing same-path entry as a conflict and leaves
// it unchanged; it never overwrites (research.md #5).
func (s *Services) RestoreBackup(_ context.Context, id string) (*BackupRestoreResult, error) {
	if id == "" {
		latest, err := s.latestBackupID()
		if err != nil {
			return nil, err
		}
		id = latest
	}
	src := filepath.Join(s.backupsDir(), id)
	if !dal.PathExists(src) {
		return nil, common.WithExitCode(fmt.Errorf("backup: %q not found", id), common.ExitObject)
	}
	roots, err := backupEntryRoots(src)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}
	if len(roots) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("backup: %q contains no entries", id), common.ExitObject)
	}

	result := &BackupRestoreResult{ID: id, Items: make([]BackupRestoreItemResult, 0, len(roots)), Success: true}
	for _, rel := range roots {
		item := BackupRestoreItemResult{Path: rel}
		live := filepath.Join(s.Cfg.Root, rel)
		if dal.PathExists(live) {
			item.Status, item.Reason = "skipped", "an entry already exists at this path"
			result.Success = false
			result.Items = append(result.Items, item)
			continue
		}
		if err := copyBackupTree(filepath.Join(src, rel), live); err != nil {
			item.Status, item.Reason = "failed", err.Error()
			result.Success = false
		} else {
			item.Status = "restored"
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *Services) latestBackupID() (string, error) {
	entries, err := os.ReadDir(s.backupsDir())
	if err != nil {
		return "", common.WithExitCode(fmt.Errorf("backup: no backups found"), common.ExitObject)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	if len(ids) == 0 {
		return "", common.WithExitCode(fmt.Errorf("backup: no backups found"), common.ExitObject)
	}
	sort.Strings(ids) // the timestamp id format sorts lexicographically = chronologically
	return ids[len(ids)-1], nil
}

// backupEntryRoots finds every directory in a snapshot that carries a marker
// file, mirroring how the repository itself identifies an entry.
func backupEntryRoots(snapshotDir string) ([]string, error) {
	var roots []string
	for _, top := range []string{"skills", "commands"} {
		base := filepath.Join(snapshotDir, top)
		if !dal.PathExists(base) {
			continue
		}
		walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || (d.Name() != "SKILL.md" && d.Name() != "command.md") {
				return nil
			}
			rel, relErr := filepath.Rel(snapshotDir, filepath.Dir(p))
			if relErr != nil {
				return relErr
			}
			roots = append(roots, filepath.ToSlash(rel))
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func writeBackupMeta(dest string, snapshot BackupSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, backupMetaFile), data, 0o644)
}

func copyBackupTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
