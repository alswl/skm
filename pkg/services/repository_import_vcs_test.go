package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// When the skill *is* the repository — SKILL.md at its root — the staged tree
// the provider hands over is a working clone, .git and all. Only the content
// belongs in the skm repository: the .git database is the provider's business,
// it is often far larger than the skill itself, and skm never reads it (update
// re-fetches from the recorded origin rather than pulling in place).

func TestImportLeavesVCSMetadataBehind(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "clone")
	writeFile(t, staged, "SKILL.md", frontmatter("repo-skill", "the repo is the skill"))
	writeFile(t, staged, "docs/guide.md", "content")
	writeFile(t, staged, ".git/config", "[core]\n")
	writeFile(t, staged, ".git/objects/ab/cdef", "binary junk")
	writeFile(t, staged, ".gitignore", "node_modules\n")

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "o/r", false, nil)
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(res.Path, "SKILL.md"))
	require.FileExists(t, filepath.Join(res.Path, "docs", "guide.md"), "real content is kept")
	require.FileExists(t, filepath.Join(res.Path, ".gitignore"), ".gitignore is content, not VCS metadata")
	require.NoDirExists(t, filepath.Join(res.Path, ".git"), "the git database must not be imported")
}

// A submodule or linked worktree checkout has .git as a *file* pointing at the
// real database, not a directory.
func TestImportLeavesVCSPointerFileBehind(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "worktree")
	writeFile(t, staged, "SKILL.md", frontmatter("wt-skill", "checked out as a worktree"))
	writeFile(t, staged, ".git", "gitdir: /elsewhere/.git/worktrees/wt\n")

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "", false, nil)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(res.Path, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(res.Path, ".git"), "the gitdir pointer must not be imported")
}

// The imported tree has no .git, so a freshly cloned one must not either —
// otherwise every comparison sees a difference and `update` reports the entry
// as changed forever, on every run.
func TestUpdateComparisonIgnoresVCSMetadata(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "clone")
	writeFile(t, staged, "SKILL.md", frontmatter("repo-skill", "the repo is the skill"))
	writeFile(t, staged, ".git/HEAD", "ref: refs/heads/main\n")

	repo := NewRepository(root)
	res, err := repo.ImportStaged(context.Background(), staged, "github", "", false, nil)
	require.NoError(t, err)

	entries := repo.Scan()
	require.Len(t, entries, 1)

	// A re-clone of unchanged content, with a different .git (new object ids,
	// different HEAD) — exactly what the next `git clone` produces.
	refetched := filepath.Join(t.TempDir(), "reclone")
	writeFile(t, refetched, "SKILL.md", frontmatter("repo-skill", "the repo is the skill"))
	writeFile(t, refetched, ".git/HEAD", "ref: refs/heads/other\n")
	writeFile(t, refetched, ".git/objects/99/zzzz", "different junk")

	_, _, changed, err := repo.CompareUpdate(entries[0], refetched)
	require.NoError(t, err)
	require.False(t, changed, "identical content must compare equal however the clones' .git differ")
	require.NotEmpty(t, res.Path)
}

// A nested .git belongs to a submodule and is metadata for the same reason.
func TestImportLeavesNestedVCSMetadataBehind(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "clone")
	writeFile(t, staged, "SKILL.md", frontmatter("nested-skill", "has a submodule"))
	writeFile(t, staged, "vendor/dep/.git/config", "[core]\n")
	writeFile(t, staged, "vendor/dep/README.md", "kept")

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "", false, nil)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(res.Path, "vendor", "dep", "README.md"))
	require.NoDirExists(t, filepath.Join(res.Path, "vendor", "dep", ".git"))

	// The staged tree itself is untouched — skm copies out of it, never prunes it.
	_, err = os.Stat(filepath.Join(staged, ".git"))
	require.True(t, os.IsNotExist(err) || err == nil)
}
