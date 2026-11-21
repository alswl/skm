//go:build darwin || linux

package dal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"golang.org/x/sys/unix"
)

func machineOpenParent(root, relative string) (int, string, error) {
	if !filepath.IsAbs(root) || !filepath.IsLocal(relative) || relative == "." || filepath.Clean(relative) != relative {
		return -1, "", fmt.Errorf("invalid machine link path")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", err
	}
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(fd)
		if err != nil {
			return -1, "", err
		}
		fd = next
	}
	return fd, parts[len(parts)-1], nil
}

func machineLinkIdentityAt(fd int, name string) (MachineLinkIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return MachineLinkIdentity{}, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFLNK {
		return MachineLinkIdentity{}, fmt.Errorf("selected object is not a symbolic link")
	}
	for size := 256; size <= 65536; size *= 2 {
		buf := make([]byte, size)
		n, err := unix.Readlinkat(fd, name, buf)
		if err != nil {
			return MachineLinkIdentity{}, err
		}
		if n < len(buf) {
			return MachineLinkIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Target: string(buf[:n])}, nil
		}
	}
	return MachineLinkIdentity{}, fmt.Errorf("symbolic link target too long")
}

// InspectMachineLink snapshots a link without following parent symlinks.
func InspectMachineLink(root, relative string) (MachineLinkIdentity, error) {
	fd, name, err := machineOpenParent(root, relative)
	if err != nil {
		return MachineLinkIdentity{}, err
	}
	defer func() { _ = unix.Close(fd) }()
	if err := machineCheckExclusiveRename(fd); err != nil {
		return MachineLinkIdentity{}, fmt.Errorf("%w: %w", common.ErrMachineRenameUnavailable, err)
	}
	return machineLinkIdentityAt(fd, name)
}

// RemoveMachineLink seizes the current object before comparing its identity.
// On mismatch it restores without replacing a newly created destination; if
// restoration fails, the returned error names the retained private object.
func RemoveMachineLink(root, relative string, expected MachineLinkIdentity) error {
	return removeMachineLink(root, relative, expected, nil)
}

func removeMachineLink(root, relative string, expected MachineLinkIdentity, afterSeize func()) error {
	return removeMachineLinkWithRename(root, relative, expected, afterSeize, machineRenameExclusive)
}

func removeMachineLinkWithRename(root, relative string, expected MachineLinkIdentity, afterSeize func(), rename func(int, string, int, string) error) error {
	parent, name, err := machineOpenParent(root, relative)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parent) }()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	private := ".skm-seized-" + hex.EncodeToString(random[:])
	if err := unix.Mkdirat(parent, private, 0o700); err != nil {
		return err
	}
	privateFD, err := unix.Openat(parent, private, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		_ = unix.Unlinkat(parent, private, unix.AT_REMOVEDIR)
		return err
	}
	defer func() { _ = unix.Close(privateFD) }()
	if err := rename(parent, name, privateFD, "object"); err != nil {
		_ = unix.Unlinkat(parent, private, unix.AT_REMOVEDIR)
		return fmt.Errorf("seize link: %w", err)
	}
	if afterSeize != nil {
		afterSeize()
	}
	actual, checkErr := machineLinkIdentityAt(privateFD, "object")
	if checkErr != nil || actual != expected {
		if err := rename(privateFD, "object", parent, name); err != nil {
			retained := filepath.Join(root, filepath.Dir(relative), private, "object")
			return fmt.Errorf("selected object changed; recovery failed: %w; retained at %s", err, retained)
		}
		_ = unix.Unlinkat(parent, private, unix.AT_REMOVEDIR)
		return fmt.Errorf("selected object changed; restored without removing it")
	}
	if err := unix.Unlinkat(privateFD, "object", 0); err != nil {
		retained := filepath.Join(root, filepath.Dir(relative), private, "object")
		return fmt.Errorf("remove seized link: %w; retained at %s", err, retained)
	}
	if err := unix.Unlinkat(parent, private, unix.AT_REMOVEDIR); err != nil {
		return fmt.Errorf("remove private directory: %w", err)
	}
	return nil
}
