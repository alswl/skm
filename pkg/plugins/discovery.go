package plugins

import (
	"os"
	"path/filepath"
)

// ListExecutables returns the executable file paths found in each base dir's
// subdir subdirectory, in base-dir order (then directory-listing order,
// which os.ReadDir returns sorted by filename). A missing/unreadable subdir
// is skipped, not an error.
func ListExecutables(baseDirs []string, subdir string) []string {
	var out []string
	for _, base := range baseDirs {
		dir := filepath.Join(base, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // missing/unreadable dir is not an error
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if IsExecutable(path) {
				out = append(out, path)
			}
		}
	}
	return out
}

// IsExecutable reports whether the file at path has an executable bit set.
func IsExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&0o111 != 0
}
