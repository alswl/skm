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

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", false, nil)
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

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", false, nil)
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

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", false, origin)
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

func TestImportCollisionRejectedWithoutForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/existing/SKILL.md", frontmatter("dup", "first"))
	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "second"))

	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", false, nil)
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

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", true, nil)
	require.NoError(t, err)
	require.Equal(t, "dup", res.Name)
	require.Equal(t, filepath.Join(root, "skills/local/dup"), res.Path)
	after, _ := os.ReadFile(orig)
	require.NotEqual(t, string(before), string(after), "force overwrites the marker")
}

func TestImportRejectsUnidentifiableSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "plain-dir")
	writeFile(t, src, "notes.txt", "no marker here")
	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot identify")
}
