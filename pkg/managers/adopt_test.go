package managers

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

	// The repository now has a managed entry under the local layer.
	require.FileExists(t, filepath.Join(root, "skills", "local", "ext", "SKILL.md"))

	// The external directory is now a managed symlink into the repo.
	fi, err := os.Lstat(extPath)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "external dir replaced by a symlink")
	dst, err := os.Readlink(extPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "local", "ext"), dst)
}

func TestAdoptExternalRejectsSymlink(t *testing.T) {
	svc, root, _ := adoptFixture(t)
	link := filepath.Join(root, "skills", "link")
	require.NoError(t, os.Symlink(root, link))

	_, err := svc.AdoptExternal(context.Background(), link, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}
