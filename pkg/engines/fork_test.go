package engines

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

const skillMD = "---\nname: review\ndescription: Review\n---\nbody\n"

func TestScanFoldsForkIntoOneEntry(t *testing.T) {
	root := t.TempDir()
	put(t, root, "skills/local/review/SKILL.md", skillMD)
	put(t, root, "skills/local/review/meta.json", `{"address":"/src","mode_id":"local"}`)
	put(t, root, "skills/local/plain/SKILL.md", "---\nname: plain\ndescription: Plain\n---\n")
	put(t, root, "skills-fork/local/review/SKILL.md", "---\nname: review\ndescription: My review\n---\nmine\n")
	put(t, root, "skills-fork/local/orphan/SKILL.md", "---\nname: orphan\ndescription: No upstream\n---\n")

	entries := NewRepository(root).Scan()
	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	require.Len(t, entries, 2, "the fork is not a separate entry and a fork without upstream is not listed")
	review := byName["review"]
	require.Equal(t, common.StatusActive, review.Status)
	require.Equal(t, filepath.Join(root, "skills-fork/local/review"), review.Path)
	require.Equal(t, filepath.Join(root, "skills/local/review"), review.UpstreamPath)
	require.Equal(t, "My review", review.Description, "the fork's own marker is what is shown")
	require.NotNil(t, review.Origin, "origin still comes from the upstream's meta.json")
	require.Equal(t, "/src", review.Origin.Address)
	require.Empty(t, byName["plain"].UpstreamPath)
}

func TestScanFlagsBrokenOrRenamedFork(t *testing.T) {
	root := t.TempDir()
	put(t, root, "skills/local/review/SKILL.md", skillMD)
	put(t, root, "skills-fork/local/review/notes.txt", "no marker")
	put(t, root, "skills/local/other/SKILL.md", "---\nname: other\ndescription: Other\n---\n")
	put(t, root, "skills-fork/local/other/SKILL.md", "---\nname: renamed\ndescription: Other\n---\n")

	for _, e := range NewRepository(root).Scan() {
		require.Equal(t, common.StatusError, e.Status, e.Name)
		require.Contains(t, *e.Error, "fork")
		require.Equal(t, filepath.Join(root, "skills", "local", e.Name), e.Path, "an unusable fork never silently falls back or becomes active")
	}
}

func TestCommitForkUpdateRestoresBothDirectoriesOnFailure(t *testing.T) {
	root := t.TempDir()
	put(t, root, "skills/local/review/SKILL.md", skillMD)
	put(t, root, "skills-fork/local/review/SKILL.md", skillMD)
	repo := NewRepository(root)
	entry := &common.Entry{
		Path:         filepath.Join(root, "skills-fork/local/review"),
		UpstreamPath: filepath.Join(root, "skills/local/review"),
		Origin:       &common.Origin{Address: "/src"},
	}
	newUp := t.TempDir()
	put(t, newUp, "SKILL.md", skillMD+"new\n")
	before := map[string]string{}
	for _, d := range []string{entry.UpstreamPath, entry.Path} {
		h, err := dal.DirHash(d, nil)
		require.NoError(t, err)
		before[d] = h
	}

	// The fork replacement fails (its staged directory is gone) after the
	// upstream was already replaced: both must come back.
	err := repo.CommitForkUpdate(context.Background(), entry, &ForkMerge{NewUpstream: newUp, Merged: filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)
	for d, want := range before {
		got, err := dal.DirHash(d, nil)
		require.NoError(t, err)
		require.Equal(t, want, got, d)
	}
	require.NoFileExists(t, filepath.Join(entry.UpstreamPath, "meta.json"), "the aborted update left no meta behind")
	entries, err := os.ReadDir(filepath.Join(root, "skills/local"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no backup directories left next to the Skill")
}

func TestMergeForkIgnoresUpstreamMetaJSON(t *testing.T) {
	root := t.TempDir()
	up, fork, fetched := filepath.Join(root, "skills/local/review"), filepath.Join(root, "skills-fork/local/review"), t.TempDir()
	put(t, root, "skills/local/review/SKILL.md", skillMD)
	put(t, root, "skills/local/review/meta.json", `{"address":"/src"}`)
	put(t, root, "skills-fork/local/review/SKILL.md", skillMD)
	put(t, fetched, "SKILL.md", skillMD)

	m, err := NewRepository(root).MergeFork(context.Background(), up, fork, fetched, ForkMergeTimeout)
	require.NoError(t, err)
	require.False(t, m.Changed, "only skm's own meta.json differs, which is not content")
}

func TestForkPairsGroupedPaths(t *testing.T) {
	root := t.TempDir()
	put(t, root, "skills/github/acme/tools/review/SKILL.md", skillMD)
	put(t, root, "skills-fork/github/acme/tools/review/SKILL.md", skillMD)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, filepath.Join(root, "skills-fork/github/acme/tools/review"), entries[0].Path)
	require.Equal(t, filepath.Join(root, "skills/github/acme/tools/review"), entries[0].UpstreamPath)
	require.Equal(t, "acme/tools", *entries[0].Group)
}

func TestForkPathRejectsPathsOutsideSkills(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(root)
	got, err := repo.ForkPath(filepath.Join(root, "skills", "local", "review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills-fork", "local", "review"), got)
	for _, bad := range []string{
		filepath.Join(root, "skills"),
		filepath.Join(root, "commands", "local", "x"),
		filepath.Join(root, "skills", "..", "elsewhere"),
		filepath.Join(filepath.Dir(root), "other"),
	} {
		_, err := repo.ForkPath(bad)
		require.Error(t, err, bad)
	}
}
