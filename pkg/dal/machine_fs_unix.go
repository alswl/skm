//go:build darwin || linux

package dal

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// CreateMachineLink creates a link beneath root without following parent
// symlinks or replacing an existing destination. Missing parents are made.
func CreateMachineLink(root, relative, source string) error {
	if !filepath.IsAbs(root) || !filepath.IsAbs(source) || !filepath.IsLocal(relative) || relative == "." || filepath.Clean(relative) != relative {
		return fmt.Errorf("invalid machine link path")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open machine root: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	for _, part := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr == unix.ENOENT {
			if mkdirErr := unix.Mkdirat(fd, part, 0o755); mkdirErr != nil && mkdirErr != unix.EEXIST {
				return fmt.Errorf("create machine parent %q: %w", part, mkdirErr)
			}
			next, openErr = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			return fmt.Errorf("open machine parent %q: %w", part, openErr)
		}
		_ = unix.Close(fd)
		fd = next
	}
	if err := unix.Symlinkat(source, fd, parts[len(parts)-1]); err != nil {
		return fmt.Errorf("create machine link %q: %w", relative, err)
	}
	return nil
}
