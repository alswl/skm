//go:build linux

package dal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestInspectMachineLinkCapabilityIsReadOnly(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("target", link))
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	require.NoError(t, err)
	defer func() { _ = unix.Close(fd) }()
	capabilityErr := machineCheckExclusiveRename(fd)
	identity, inspectErr := InspectMachineLink(root, "link")
	if capabilityErr != nil {
		require.Error(t, inspectErr)
	} else {
		require.NoError(t, inspectErr)
		require.Equal(t, "target", identity.Target)
	}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	actual, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "target", actual)
}

func TestMachineLinuxExclusiveRenameCapability(t *testing.T) {
	for _, fs := range []int64{unix.EXT4_SUPER_MAGIC, unix.BTRFS_SUPER_MAGIC, unix.TMPFS_MAGIC, unix.XFS_SUPER_MAGIC} {
		require.True(t, machineLinuxExclusiveRenameSupported(fs, "6.8.0-custom"))
		require.True(t, machineLinuxExclusiveRenameSupported(fs, "4.9.0"))
		require.True(t, machineLinuxExclusiveRenameSupported(fs, "6.1-custom"))
		require.False(t, machineLinuxExclusiveRenameSupported(fs, "4.8.99"))
	}
	require.False(t, machineLinuxExclusiveRenameSupported(0, "6.8.0"))
	require.False(t, machineLinuxExclusiveRenameSupported(unix.EXT4_SUPER_MAGIC, "unknown"))
	require.False(t, machineLinuxExclusiveRenameSupported(unix.EXT4_SUPER_MAGIC, "6.custom"))
}
