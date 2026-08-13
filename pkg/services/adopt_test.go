package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// adoptFixture builds an empty repo and a skill target holding one real,
// unmanaged external skill directory.
func adoptFixture(t *testing.T) (*Services, string, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))

	targetDir := filepath.Join(t.TempDir(), "target")
	extPath := filepath.Join(targetDir, "ext")
	require.NoError(t, os.MkdirAll(extPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extPath, "SKILL.md"),
		[]byte("---\nname: ext\ndescription: external skill\n---\nbody\n"), 0o644))

	target := common.InstallTarget{Name: "t", Path: targetDir, Kind: common.KindSkill}
	svc, err := New(newCfg(root, []common.InstallTarget{target}), common.NewLogger(false))
	require.NoError(t, err)
	return svc, root, extPath
}

func TestAdoptExternalCreatesEntryAndSymlink(t *testing.T) {
	svc, root, extPath := adoptFixture(t)

	res, err := svc.AdoptExternal(context.Background(), extPath, false)
	require.NoError(t, err)
	require.Equal(t, "ext", res.Name)
	require.Equal(t, "t", res.Target)
	entry := svc.FindEntry(filepath.Join("skills", "unknown", "t", "ext"))
	require.NotNil(t, entry)
	require.NotNil(t, entry.Origin)
	require.Equal(t, "ext", entry.Origin.InstallSlot)
	require.False(t, svc.Updatable(entry))

	// The repository now has a managed entry in the source-agnostic bucket.
	require.FileExists(t, filepath.Join(root, "skills", "unknown", "t", "ext", "SKILL.md"))

	// The external directory is now a managed symlink into the repo.
	fi, err := os.Lstat(extPath)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "external dir replaced by a symlink")
	dst, err := os.Readlink(extPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "unknown", "t", "ext"), dst)
}

func TestAdoptExternalRejectsSymlink(t *testing.T) {
	svc, root, _ := adoptFixture(t)
	link := filepath.Join(root, "skills", "link")
	require.NoError(t, os.Symlink(root, link))

	_, err := svc.AdoptExternal(context.Background(), link, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func TestAdoptExternalKeepsSameNamedSkillsInUnknownGroups(t *testing.T) {
	root := t.TempDir()
	firstTarget := filepath.Join(t.TempDir(), "first-target")
	secondTarget := filepath.Join(t.TempDir(), "second-target")
	first := filepath.Join(firstTarget, "atc-cli")
	second := filepath.Join(secondTarget, "atc-cli")
	writeFile(t, first, "SKILL.md", frontmatter("atc", "first external copy"))
	writeFile(t, second, "SKILL.md", frontmatter("atc", "second external copy"))

	svc, err := New(newCfg(root, []common.InstallTarget{
		{Name: "first", Path: firstTarget, Kind: common.KindSkill},
		{Name: "second", Path: secondTarget, Kind: common.KindSkill},
	}), common.NewLogger(false))
	require.NoError(t, err)

	firstResult, err := svc.AdoptExternal(context.Background(), first, false)
	require.NoError(t, err)
	secondResult, err := svc.AdoptExternal(context.Background(), second, false)
	require.NoError(t, err)

	require.Equal(t, "atc", firstResult.Name)
	require.Equal(t, filepath.Join(root, "skills", "unknown", "first", "atc-cli"), firstResult.Path)
	require.Equal(t, filepath.Join(root, "skills", "unknown", "second", "atc-cli"), secondResult.Path)
	entries := svc.Scan()
	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.Equal(t, "atc", entry.Name)
		require.Equal(t, "unknown", entry.ProviderIDValue())
	}
	firstLink, err := os.Readlink(first)
	require.NoError(t, err)
	secondLink, err := os.Readlink(second)
	require.NoError(t, err)
	require.Equal(t, firstResult.Path, firstLink)
	require.Equal(t, secondResult.Path, secondLink)
}
