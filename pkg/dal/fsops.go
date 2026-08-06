package dal

import (
	"os"
	"path/filepath"
	"sort"
)

// AdapterMarker is the marker file written inside a managed command adapter
// directory (contract/install-semantics.md). It is a dotfile so it stays
// invisible to the consuming tool's skill discovery.
const AdapterMarker = ".skm-adapter"

// IsSymlink reports whether path is a symlink (lstat-based).
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// ReadLinkTarget returns the raw target string of the symlink at path, or ""
// when path is not a symlink.
func ReadLinkTarget(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return target
}

// ResolveLink returns the absolute path a symlink resolves to, or "" when
// path is not a symlink or cannot be evaluated.
func ResolveLink(path string) string {
	target := ReadLinkTarget(path)
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target)
}

// PathExists reports whether path exists (following symlinks).
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir reports whether path is an existing directory.
func IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// IsRegularFile reports whether path is an existing regular file.
func IsRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// ListEntryFiles returns the sorted relative paths of all files/dirs under an
// entry directory (for info reports and the TUI file tree).
func ListEntryFiles(entryDir string) []string {
	var out []string
	_ = filepath.WalkDir(entryDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == entryDir {
			return nil
		}
		rel, err := filepath.Rel(entryDir, path)
		if err != nil {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
}
