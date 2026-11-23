//go:build darwin || linux

package dal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRemoveMachineLinkUnsupportedRenamePreservesLink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("target", link))
	identity, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	err = removeMachineLinkWithRename(root, "link", identity, nil, func(int, string, int, string) error { return unix.ENOTSUP })
	require.ErrorIs(t, err, unix.ENOTSUP)
	actual, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	require.Equal(t, identity, actual)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestMachineUnlinkRejectsSymlinkParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(outside, "link")
	require.NoError(t, os.Symlink("target", link))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "redirect")))
	relative := filepath.Join("redirect", "link")
	_, err := InspectMachineLink(root, relative)
	require.Error(t, err)
	require.Error(t, RemoveMachineLink(root, relative, MachineLinkIdentity{}))
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "target", target)
}

func TestRemoveMachineLinkRejectsReplacedLinkWithSameTarget(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("target", link))
	identity, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	// Keep the original inode allocated so the replacement cannot reuse it.
	require.NoError(t, os.Rename(link, filepath.Join(root, "old")))
	require.NoError(t, os.Symlink("target", link))
	require.ErrorContains(t, RemoveMachineLink(root, "link", identity), "restored")
	actual, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "target", actual)
}

func TestRemoveMachineLinkSeizesAndVerifies(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("first", link))
	identity, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	require.NoError(t, RemoveMachineLink(root, "link", identity))
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, os.Symlink("first", link))
	identity, err = InspectMachineLink(root, "link")
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.WriteFile(link, []byte("external"), 0o644))
	require.Error(t, RemoveMachineLink(root, "link", identity))
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "external", string(data))
}

func TestRemoveMachineLinkRecoveryDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("old", link))
	identity, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.WriteFile(link, []byte("seized"), 0o644))
	err = removeMachineLink(root, "link", identity, func() { require.NoError(t, os.WriteFile(link, []byte("replacement"), 0o644)) })
	require.ErrorContains(t, err, "retained at")
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
	matches, err := filepath.Glob(filepath.Join(root, ".skm-seized-*", "object"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	data, err = os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Equal(t, "seized", string(data))
}

func TestRemoveMachineLinkPreservesReplacementAfterSeize(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("old", link))
	identity, err := InspectMachineLink(root, "link")
	require.NoError(t, err)
	err = removeMachineLink(root, "link", identity, func() { require.NoError(t, os.WriteFile(link, []byte("replacement"), 0o644)) })
	require.NoError(t, err)
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
}
