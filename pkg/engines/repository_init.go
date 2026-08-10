package engines

import (
	"fmt"
	"os"
	"path/filepath"
)

// InitializeRepository creates the smallest safe skills repository. A target
// must be absent or empty; this avoids silently classifying an unrelated
// directory as a skm repository.
func InitializeRepository(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("init: resolve repository path: %w", err)
	}
	if info, err := os.Stat(abs); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("init: %q is not a directory", abs)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return "", fmt.Errorf("init: inspect %q: %w", abs, err)
		}
		if len(entries) != 0 {
			return "", fmt.Errorf("init: %q is not empty; choose an empty directory to avoid changing existing content", abs)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("init: inspect %q: %w", abs, err)
	}
	if err := os.MkdirAll(filepath.Join(abs, "skills"), 0o755); err != nil {
		return "", fmt.Errorf("init: create skills repository: %w", err)
	}
	return abs, nil
}
