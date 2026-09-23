package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// BackupCreateResult is the CLI/TUI-facing result of `backup create`.
type BackupCreateResult struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Entries []string `json:"entries"`
}

type BackupRestoreItemResult struct {
	Path    string                 `json:"path"`
	Status  string                 `json:"status"` // restored | failed
	Reason  string                 `json:"reason,omitempty"`
	Results []common.InstallReport `json:"results,omitempty"`
}

type BackupRestoreResult struct {
	ID      string                    `json:"id"`
	Items   []BackupRestoreItemResult `json:"items"`
	Success bool                      `json:"success"`
}

const backupFileSuffix = ".json"

// backupEntryInstall records which currently-installed targets one entry root
// was on at backup time, so restore can put it back on the same targets.
type backupEntryInstall struct {
	Root    string   `json:"root"`
	Targets []string `json:"targets"`
}

// backupDocument is the whole snapshot: a single minified JSON document
// recording which entries existed and where they were installed. It never
// carries entry file content — the repository (typically git) is the sole
// source of truth for that; restoring content is that system's job, not
// backup's.
type backupDocument struct {
	ID        string               `json:"id"`
	CreatedAt time.Time            `json:"created_at"`
	Entries   []string             `json:"entries"`
	Roots     []string             `json:"roots"`
	Installs  []backupEntryInstall `json:"installs,omitempty"`
}

func (s *Services) backupsDir() string {
	return filepath.Join(s.Cfg.ConfigDir, "backups")
}

func (s *Services) backupPath(id string) string {
	return filepath.Join(s.backupsDir(), id+backupFileSuffix)
}

// CreateBackup never produces or consumes a share payload; backup and share
// are deliberately separate local vs. cross-user mechanisms (research.md #5).
func (s *Services) CreateBackup(_ context.Context) (*BackupCreateResult, error) {
	id := time.Now().UTC().Format("20060102T150405Z")
	doc := backupDocument{ID: id, CreatedAt: time.Now().UTC()}

	for _, top := range []string{"skills", "commands"} {
		src := filepath.Join(s.Cfg.Root, top)
		if !dal.PathExists(src) {
			continue
		}
		roots, err := collectBackupRoots(s.Cfg.Root, src)
		if err != nil {
			return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
		}
		doc.Roots = append(doc.Roots, roots...)
		doc.Entries = append(doc.Entries, top)
	}
	if len(doc.Entries) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("backup: no local repository content to back up"), common.ExitObject)
	}
	sort.Strings(doc.Roots)
	for _, root := range doc.Roots {
		entry, err := s.ResolveEntry(root)
		if err != nil || entry == nil {
			continue
		}
		var targets []string
		for _, t := range s.Installer.Targets(entry) {
			if s.Installer.State(entry, t) == common.InstallInstalled {
				targets = append(targets, t.Name)
			}
		}
		if len(targets) > 0 {
			doc.Installs = append(doc.Installs, backupEntryInstall{Root: root, Targets: targets})
		}
	}

	dest := s.backupPath(id)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}
	if err := writeBackupDocument(dest, doc); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}
	return &BackupCreateResult{ID: id, Path: dest, Entries: doc.Entries}, nil
}

// RestoreBackup reinstalls every backed-up entry that still exists in the
// repository to whichever of its previously-installed targets still exist
// and still accept it. It never recreates an entry's content — backup
// carries no file bytes — so an entry that no longer exists is reported as
// failed rather than restored.
func (s *Services) RestoreBackup(ctx context.Context, id string) (*BackupRestoreResult, error) {
	if id == "" {
		latest, err := s.latestBackupID()
		if err != nil {
			return nil, err
		}
		id = latest
	}
	src := s.backupPath(id)
	if !dal.PathExists(src) {
		return nil, common.WithExitCode(fmt.Errorf("backup: %q not found", id), common.ExitObject)
	}
	doc, err := readBackupDocument(src)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("backup: %w", err), common.ExitError)
	}
	if len(doc.Roots) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("backup: %q contains no entries", id), common.ExitObject)
	}

	result := &BackupRestoreResult{ID: id, Items: make([]BackupRestoreItemResult, 0, len(doc.Roots)), Success: true}
	for _, root := range doc.Roots {
		item := BackupRestoreItemResult{Path: root}
		entry, err := s.ResolveEntry(root)
		if err != nil || entry == nil {
			item.Status, item.Reason = "failed", "entry no longer exists in the repository; backup carries no file content to recreate it"
			result.Success = false
			result.Items = append(result.Items, item)
			continue
		}
		item.Status = "restored"
		item.Results, item.Reason = s.reinstallBackupEntry(ctx, root, installTargetsFor(doc.Installs, root))
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
		if !e.IsDir() && strings.HasSuffix(e.Name(), backupFileSuffix) {
			ids = append(ids, strings.TrimSuffix(e.Name(), backupFileSuffix))
		}
	}
	if len(ids) == 0 {
		return "", common.WithExitCode(fmt.Errorf("backup: no backups found"), common.ExitObject)
	}
	sort.Strings(ids) // the timestamp id format sorts lexicographically = chronologically
	return ids[len(ids)-1], nil
}

// collectBackupRoots walks src (an absolute path under root) and returns the
// root-relative directory of every entry it finds (identified by its marker
// file), mirroring how the repository itself identifies an entry. It reads
// no file content — a backup records which entries exist, never their bytes.
func collectBackupRoots(root, src string) ([]string, error) {
	var roots []string
	walkErr := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (d.Name() != "SKILL.md" && d.Name() != "command.md") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		roots = append(roots, filepath.ToSlash(filepath.Dir(rel)))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return roots, nil
}

func installTargetsFor(installs []backupEntryInstall, root string) []string {
	for _, ins := range installs {
		if ins.Root == root {
			return ins.Targets
		}
	}
	return nil
}

// reinstallBackupEntry reinstalls a just-restored entry to whichever of its
// recorded targets still exist and still accept it. A target that was
// removed or reconfigured since the backup was taken is silently dropped
// rather than failing the whole reinstall.
func (s *Services) reinstallBackupEntry(ctx context.Context, root string, targets []string) ([]common.InstallReport, string) {
	if len(targets) == 0 {
		return nil, ""
	}
	entry, err := s.ResolveEntry(root)
	if err != nil || entry == nil {
		return nil, "restored, but could not resolve the entry to reinstall it"
	}
	var valid []string
	for _, name := range targets {
		t, ok := s.Installer.TargetByName(name)
		if !ok || !s.Installer.Matches(entry, t) {
			continue
		}
		valid = append(valid, name)
	}
	if len(valid) == 0 {
		return nil, ""
	}
	installed, err := s.Install(ctx, root, InstallOptions{Targets: valid})
	if err != nil {
		return nil, "restored; reinstalling to its prior targets failed: " + err.Error()
	}
	return installed.Results, ""
}

func writeBackupDocument(dest string, doc backupDocument) error {
	raw, err := json.Marshal(doc) // compact: no indentation
	if err != nil {
		return err
	}
	return os.WriteFile(dest, raw, 0o644)
}

func readBackupDocument(src string) (backupDocument, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return backupDocument{}, err
	}
	var doc backupDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return backupDocument{}, err
	}
	return doc, nil
}
