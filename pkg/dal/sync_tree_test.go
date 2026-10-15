package dal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncTreeIdentity(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "script"), []byte("hello\n"), 0600))
	a, err := SyncTree(root)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(root, "script"), 0700))
	b, err := SyncTree(root)
	require.NoError(t, err)
	require.NotEqual(t, a.Digest, b.Digest)
	require.NoError(t, os.Mkdir(filepath.Join(root, "empty"), 0700))
	c, err := SyncTree(root)
	require.NoError(t, err)
	require.NotEqual(t, b.Digest, c.Digest)
	out := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, CopySyncTree(root, out))
	d, err := SyncTree(out)
	require.NoError(t, err)
	require.Equal(t, c.Digest, d.Digest)
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(root, "escape")))
	_, err = SyncTree(root)
	require.Error(t, err)
}

func TestSyncPathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"../escape", "/absolute", "."} {
		_, err := SyncPath(root, p)
		require.Error(t, err)
	}
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "link")))
	_, err := SyncPath(root, "link/file")
	require.Error(t, err)
}
