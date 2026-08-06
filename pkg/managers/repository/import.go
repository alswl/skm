package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// ImportResult reports a successfully placed import.
type ImportResult struct {
	Name     string
	Kind     common.EntryKind
	Provider string
	Path     string
	Origin   *common.Origin
}

// ProbeStaged identifies the kind and name of a staged asset (a directory
// holding a marker, or a single markdown file) without copying it.
func (r *Repository) ProbeStaged(staged string) (common.EntryKind, string, error) {
	fi, err := os.Stat(staged)
	if err != nil {
		return "", "", fmt.Errorf("import: %w", err)
	}
	if !fi.IsDir() {
		if isMarkdown(filepath.Base(staged)) {
			return r.probeSingleFile(staged)
		}
		return "", "", fmt.Errorf("import: unsupported source %q (not a directory or .md file)", staged)
	}
	switch {
	case dal.PathExists(filepath.Join(staged, "SKILL.md")):
		name, err := frontmatterName(filepath.Join(staged, "SKILL.md"))
		return common.KindSkill, name, err
	case dal.PathExists(filepath.Join(staged, "command.md")):
		name, err := frontmatterName(filepath.Join(staged, "command.md"))
		return common.KindCommand, name, err
	}
	return "", "", fmt.Errorf("import: cannot identify a skill (SKILL.md) or command (command.md) in %q", staged)
}

// probeSingleFile validates a single markdown import that becomes a directory
// command (UC-06 / FR-021).
func (r *Repository) probeSingleFile(path string) (common.EntryKind, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("import: %w", err)
	}
	fm, _, err := dal.ParseFrontmatter(data)
	if err != nil {
		return "", "", err
	}
	if fm.Name == "" {
		return "", "", errors.New("import: single-file markdown needs a name in frontmatter to become a directory command")
	}
	if fm.Description == "" {
		return "", "", errors.New("import: frontmatter missing required field: description")
	}
	return common.KindCommand, fm.Name, nil
}

// frontmatterName reads and validates the required fields from a marker file.
func frontmatterName(marker string) (string, error) {
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", fmt.Errorf("import: %w", err)
	}
	fm, _, err := dal.ParseFrontmatter(data)
	if err != nil {
		return "", err
	}
	if fm.Name == "" {
		return "", errors.New("import: frontmatter missing required field: name")
	}
	if fm.Description == "" {
		return "", errors.New("import: frontmatter missing required field: description")
	}
	return fm.Name, nil
}

// ImportStaged validates a staged asset and places it under
// <repo>/<kind>/<providerID>/<name>/, recording origin for remote imports
// (I3). The global name space is checked first; --force overwrites the
// existing entry, restoring it on failure (FR-021). Temp dirs are removed on
// failure (UC-05).
func (r *Repository) ImportStaged(ctx context.Context, staged, providerID string, force bool, origin *common.Origin) (*ImportResult, error) {
	kind, name, err := r.ProbeStaged(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	// Normalize to a temp staged tree (copy for dirs, build for single .md).
	tmp, err := r.stageCopy(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// Global name collision (I1 / FR-021).
	if existing := r.findByName(name); existing != nil && !force {
		return nil, common.WithExitCode(
			fmt.Errorf("import: name %q already exists (at %s); use --force to overwrite", name, existing.Path),
			common.ExitObject)
	}

	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	dest := filepath.Join(r.root, kind.TopDir(), providerID, name)
	tx := &dal.FileTransaction{}
	if dal.PathExists(dest) {
		if err := tx.BackupRemove(dest); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	if err := tx.MoveStage(tmp, dest); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if origin != nil {
		if err := dal.WriteMeta(dest, origin); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	tx.Commit()
	return &ImportResult{Name: name, Kind: kind, Provider: providerID, Path: dest, Origin: origin}, nil
}

// stageCopy produces a temp copy of the staged asset: a full tree copy for
// directories, or a directory command for a single markdown file.
func (r *Repository) stageCopy(staged string) (string, error) {
	fi, err := os.Stat(staged)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return copyDirToTemp(staged)
	}
	// single .md -> directory command with command.md
	tmp, err := os.MkdirTemp("", "skm-import-*")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(staged)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, "command.md"), data, 0o644); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

func copyDirToTemp(src string) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-import-*")
	if err != nil {
		return "", err
	}
	if err := copyTree(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

// copyTree copies a directory tree, preserving symlinks.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// findByName returns the first entry with the given name (global name space).
func (r *Repository) findByName(name string) *common.Entry {
	for _, e := range r.Scan() {
		if e.Name == name {
			return e
		}
	}
	return nil
}
