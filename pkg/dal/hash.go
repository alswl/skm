package dal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// DirHash computes a deterministic content hash of a directory tree. Files
// are hashed in sorted path order with a NUL separator so renames reorder
// output. An optional exclude predicate skips paths (e.g. meta.json during
// update comparison, FR-023).
func DirHash(dir string, exclude func(rel string) bool) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if exclude != nil && exclude(rel) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		f, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(h, "%s\x00", rel); err != nil {
			_ = f.Close()
			return "", err
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", err
		}
		_ = f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
