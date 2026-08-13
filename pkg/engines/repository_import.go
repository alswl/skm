package engines

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// RepositoryImportResult reports a successfully placed import.
type RepositoryImportResult struct {
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
		// A directory name is a safe identity fallback for a skill directory.
		// ImportStaged repairs the copied marker before it is committed. This
		// lets a repository maintainer claim a legacy or manually authored
		// SKILL.md whose required metadata is incomplete, without loosening the
		// validation rules for commands or arbitrary markdown files.
		if err != nil {
			if _, readErr := os.ReadFile(filepath.Join(staged, "SKILL.md")); readErr != nil {
				return common.KindSkill, "", err
			}
			return common.KindSkill, filepath.Base(staged), nil
		}
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
// <repo>/<kind>/<providerID>/<name>/ (or <repo>/<kind>/<providerID>/<group>/<name>/
// when group is non-empty — e.g. "owner/repo" for a GitHub/GitLab import, see
// gitHostProvider.Group), recording origin for remote imports (I3).
func (r *Repository) ImportStaged(ctx context.Context, staged, providerID, group string, force bool, origin *common.Origin) (*RepositoryImportResult, error) {
	return r.importStaged(ctx, staged, providerID, group, "", force, origin)
}

// ImportStagedAs preserves a discovery source's directory identity.
func (r *Repository) ImportStagedAs(ctx context.Context, staged, providerID, group, entryID string, force bool, origin *common.Origin) (*RepositoryImportResult, error) {
	return r.importStaged(ctx, staged, providerID, group, entryID, force, origin)
}

func (r *Repository) importStaged(ctx context.Context, staged, providerID, group, entryID string, force bool, origin *common.Origin) (*RepositoryImportResult, error) {
	kind, name, err := r.ProbeStaged(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if entryID == "" {
		entryID = name
	}
	if entryID == "." || entryID == ".." || filepath.Base(entryID) != entryID {
		return nil, common.WithExitCode(fmt.Errorf("import: invalid entry id %q", entryID), common.ExitError)
	}
	// Normalize to a temp staged tree (copy for dirs, build for single .md).
	tmp, err := r.stageCopy(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := normalizeStagedSkill(tmp, name); err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}

	// Importing non-standard entries is an explicit claim into standard layout.
	sourceEntry := r.findByPath(staged)
	sameManagedSource := sourceEntry != nil
	claimSource := sameManagedSource && (sourceEntry.Status == common.StatusNonStandard || sourceEntry.Status == common.StatusError)
	// Reject reimports so force cannot replace their own source.
	if sameManagedSource && !claimSource {
		return nil, common.WithExitCode(
			fmt.Errorf("import: source %q is already managed at %s; install or move it instead of importing", name, sourceEntry.Path),
			common.ExitObject)
	}

	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	dest := filepath.Join(r.root, kind.TopDir(), providerID, group, entryID)
	if dal.PathExists(dest) && (!claimSource || !samePath(dest, staged)) && !force {
		return nil, common.WithExitCode(
			common.WithNeedsForce(fmt.Errorf("import: destination %q already exists; use --force to overwrite", dest)),
			common.ExitObject)
	}
	tx := &dal.FileTransaction{}
	if dal.PathExists(dest) && (!claimSource || !samePath(dest, staged)) {
		if err := tx.BackupRemove(dest); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	if claimSource {
		if err := tx.BackupRemove(staged); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	if err := tx.MoveStage(tmp, dest); err != nil {
		_ = tx.Rollback()
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if origin != nil {
		// Record the installed location (relative to the repo root) so the
		// meta.json tracks url / provider / path for the entry.
		if rel, err := filepath.Rel(r.root, dest); err == nil {
			origin.Path = rel
		}
		if err := dal.WriteMeta(dest, origin); err != nil {
			_ = tx.Rollback()
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	tx.Commit()
	return &RepositoryImportResult{Name: name, Kind: kind, Provider: providerID, Path: dest, Origin: origin}, nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(aa) == filepath.Clean(bb)
}

// normalizeStagedSkill repairs only a directory-style SKILL.md that fails the
// normal required-field validation. Repair runs on ImportStaged's temporary
// copy, which becomes the managed repository entry. A non-standard source
// inside the repository is then claimed by moving it into that entry; an
// external source is left untouched. A malformed YAML header is retained as
// body content instead of being thrown away, so users can recover any original
// instructions manually if needed.
func normalizeStagedSkill(staged, fallbackName string) error {
	marker := filepath.Join(staged, "SKILL.md")
	if !dal.PathExists(marker) {
		return nil
	}
	if _, err := frontmatterName(marker); err == nil {
		return nil
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fm, body, parseErr := dal.ParseFrontmatter(data)
	if parseErr != nil {
		body = data
		fm = &dal.Frontmatter{}
	}
	if fm.Name == "" {
		fm.Name = fallbackName
	}
	if fm.Description == "" {
		fm.Description = "Imported and normalized skill"
	}
	if err := os.WriteFile(marker, dal.EncodeFrontmatter(fm, body), 0o644); err != nil {
		return fmt.Errorf("import: repair SKILL.md: %w", err)
	}
	return nil
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
// vcsMetadataDir is the one name copyTree refuses to carry into the
// repository. When a skill *is* a repository — SKILL.md at its root — the
// staged tree a provider hands over is a working clone, and only its content
// belongs to skm: the git database is often far larger than the skill, and
// nothing here ever reads it (update re-clones from the recorded origin rather
// than pulling in place). Leaving it in also made every update report a change
// forever, since two clones never share a byte-identical .git
// (TestUpdateComparisonIgnoresVCSMetadata). ".gitignore" and ".github" are
// content and are kept — only the database itself is dropped, whether it is a
// directory or the pointer file a submodule/worktree checkout leaves.
const vcsMetadataDir = ".git"

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.Name() == vcsMetadataDir && rel != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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

// findByPath permits several entries to share a frontmatter name.
func (r *Repository) findByPath(path string) *common.Entry {
	for _, e := range r.Scan() {
		if samePath(e.Path, path) {
			return e
		}
	}
	return nil
}
