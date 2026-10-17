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

func TestPayloadTreeExcludesOnlyRootManagementMeta(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("skill"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "meta.json"), []byte(`{"active":"upstream"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "meta.json"), []byte(`{"user":"data"}`), 0600))
	before, err := PayloadSyncTree(root)
	require.NoError(t, err)
	fullBefore, err := TreeFingerprint(root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "meta.json"), []byte(`{"active":"fork"}`), 0600))
	after, err := PayloadSyncTree(root)
	require.NoError(t, err)
	fullAfter, err := TreeFingerprint(root)
	require.NoError(t, err)
	require.Equal(t, before.Digest, after.Digest)
	require.NotEqual(t, fullBefore, fullAfter)
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "meta.json"), []byte(`{"user":"changed"}`), 0600))
	nestedChange, err := PayloadSyncTree(root)
	require.NoError(t, err)
	require.NotEqual(t, after.Digest, nestedChange.Digest)
	out := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, CopyPayloadSyncTree(root, out))
	require.NoFileExists(t, filepath.Join(out, "meta.json"))
	require.FileExists(t, filepath.Join(out, "nested", "meta.json"))
	copyManifest, err := PayloadSyncTree(out)
	require.NoError(t, err)
	require.Equal(t, nestedChange.Digest, copyManifest.Digest)
}
