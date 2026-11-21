//go:build linux

package dal

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Linux has no generic per-mount RENAME_NOREPLACE query. Restrict preview to
// in-kernel filesystems with documented support; unknown types fail closed.
func machineCheckExclusiveRename(fd int) error {
	var fs unix.Statfs_t
	if err := unix.Fstatfs(fd, &fs); err != nil {
		return fmt.Errorf("inspect filesystem for exclusive rename: %w", err)
	}
	var system unix.Utsname
	if err := unix.Uname(&system); err != nil {
		return fmt.Errorf("inspect kernel for exclusive rename: %w", err)
	}
	release := make([]byte, 0, len(system.Release))
	for _, c := range system.Release {
		if c == 0 {
			break
		}
		release = append(release, byte(c))
	}
	if !machineLinuxExclusiveRenameSupported(int64(fs.Type), string(release)) {
		return fmt.Errorf("cannot establish exclusive rename support for filesystem %#x on kernel %s", fs.Type, release)
	}
	return nil
}

func machineLinuxExclusiveRenameSupported(fsType int64, release string) bool {
	// ext2/3/4 share a magic value; 4.9 is the first version that supports all.
	switch uint32(fsType) {
	case unix.EXT4_SUPER_MAGIC, unix.BTRFS_SUPER_MAGIC, unix.TMPFS_MAGIC, unix.XFS_SUPER_MAGIC:
	default:
		return false
	}
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minorPart := parts[1]
	if i := strings.IndexFunc(minorPart, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		minorPart = minorPart[:i]
	}
	minor, err := strconv.Atoi(minorPart)
	return err == nil && (major > 4 || major == 4 && minor >= 9)
}
