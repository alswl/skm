package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestImportLocalDirPlacesUnderLocalLayerNoOrigin(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "review")
	writeFile(t, src, "SKILL.md", frontmatter("review", "a review skill"))
	writeFile(t, src, "prompt.txt", "content")

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, "review", res.Name)
	require.Equal(t, common.KindSkill, res.Kind)
	require.Nil(t, res.Origin)

	dest := filepath.Join(root, "skills", "local", "review")
	require.Equal(t, dest, res.Path)
	require.FileExists(t, filepath.Join(dest, "SKILL.md"))
	require.FileExists(t, filepath.Join(dest, "prompt.txt"), "resource files copied")
	// Local import must not record origin.
	_, err = os.Stat(filepath.Join(dest, "meta.json"))
	require.True(t, os.IsNotExist(err))
	// The source must remain (copied, not moved).
	require.FileExists(t, filepath.Join(src, "SKILL.md"))
}

func TestImportSingleFileMarkdownBecomesDirCommand(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "hello.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nname: hello\ndescription: greets\n---\nhi\n"), 0o644))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, common.KindCommand, res.Kind)
	require.Equal(t, filepath.Join(root, "commands", "local", "hello"), res.Path)
	require.FileExists(t, filepath.Join(res.Path, "command.md"))
}

func TestImportProviderRecordsOrigin(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "fetched")
	writeFile(t, staged, "SKILL.md", frontmatter("remote-skill", "from a remote"))
	mode := "github"
	origin := &common.Origin{Address: "https://github.com/x/y", ProviderID: &mode}

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "", false, origin)
	require.NoError(t, err)
	require.Equal(t, "remote-skill", res.Name)
	require.Equal(t, filepath.Join(root, "skills", "github", "remote-skill"), res.Path)
	require.NotNil(t, res.Origin)
	require.Equal(t, "https://github.com/x/y", res.Origin.Address)
	require.Equal(t, "github", *res.Origin.ProviderID)
	require.Equal(t, "skills/github/remote-skill", res.Origin.Path, "meta.json tracks the installed path relative to the repo root")
	require.FileExists(t, filepath.Join(res.Path, "meta.json"))

	// The on-disk meta.json carries url / provider / path for management.
	stored, err := dal.ReadMeta(res.Path)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/x/y", stored.Address)
	require.Equal(t, "github", *stored.ProviderID)
	require.Equal(t, "skills/github/remote-skill", stored.Path)
}

// TestImportWithGroupNestsUnderOwnerRepoAndScanReportsIt: a non-empty group
// (as gitHostProvider.Group derives for a GitHub/GitLab address) places the
// import under <provider>/<group>/<name> instead of the flat
// <provider>/<name> layout, so the entry's Group is recovered on the next
// scan the same way any other nested provider directory already is
// (repository_scan.go scanGroup) — "GitHub 要显示 group/repo/name".
func TestImportWithGroupNestsUnderOwnerRepoAndScanReportsIt(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "fetched")
	writeFile(t, staged, "SKILL.md", frontmatter("remote-skill", "from a remote"))
	mode := "github"
	origin := &common.Origin{Address: "https://github.com/octocat/hello-world", ProviderID: &mode}

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "octocat/hello-world", false, origin)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "github", "octocat", "hello-world", "remote-skill"), res.Path)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, "remote-skill", entries[0].Name)
	require.Equal(t, "octocat/hello-world", entries[0].GroupValue(), "the scan recovers the owner/repo group from the nested directory layout")
	require.Equal(t, "github", entries[0].ProviderIDValue())
}

func TestImportCollisionRejectedWithoutForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/existing/SKILL.md", frontmatter("dup", "first"))
	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "second"))

	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.Error(t, err, "global name collision must be rejected without force")
	// Original intact.
	require.FileExists(t, filepath.Join(root, "skills/local/existing/SKILL.md"))
}

func TestImportForceOverwriteAndRollback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/dup/SKILL.md", frontmatter("dup", "original"))
	orig := filepath.Join(root, "skills/local/dup/SKILL.md")
	before, _ := os.ReadFile(orig)

	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "overwritten"))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", true, nil)
	require.NoError(t, err)
	require.Equal(t, "dup", res.Name)
	require.Equal(t, filepath.Join(root, "skills/local/dup"), res.Path)
	after, _ := os.ReadFile(orig)
	require.NotEqual(t, string(before), string(after), "force overwrites the marker")
}

// TestImportForceReplacesTheCollidingEntryWhereverItLives: --force must leave
// exactly one entry per name even when the import lands somewhere else than
// the entry it collides with. Grouped imports made that routine — an entry
// imported flat as skills/github/<name> re-imports to
// skills/github/<owner>/<repo>/<name> — and only the destination path used to
// be cleared, so the old copy survived and the repository ended up with two
// entries answering to one name.
func TestImportForceReplacesTheCollidingEntryWhereverItLives(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/dup/SKILL.md", frontmatter("dup", "flat, pre-group import"))

	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "regrouped"))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "github", "octocat/hello-world", true, nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/github/octocat/hello-world/dup"), res.Path)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1, "the flat copy must be gone, not left behind as a second entry named dup")
	require.Equal(t, res.Path, entries[0].Path)
	require.NoDirExists(t, filepath.Join(root, "skills/github/dup"))
}

func TestImportRejectsUnidentifiableSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "plain-dir")
	writeFile(t, src, "notes.txt", "no marker here")
	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot identify")
}
