package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

// DiscoverRoot resolves the repository root. When rootFlag is non-empty it is
// normalized (accepting either the repo root or a skills/ directory); when
// empty, the tool walks upward from cwd for an ancestor containing skills/ or
// commands/ (FR-002).
func DiscoverRoot(rootFlag string) (string, error) {
	if rootFlag != "" {
		return NormalizeRoot(rootFlag)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", common.Errf("cannot determine working directory: %w", err)
	}
	dir := cwd
	for {
		if hasRepoMarkers(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("no repository found: no skills/ or commands/ directory upward from the current directory; initialize one with `skm init [DIRECTORY]` or use --root")
}

// NormalizeRoot converts a --root value into the repository root: a path
// pointing directly at a skills/ directory is normalized to its parent,
// unless the path itself already contains a skills/ or commands/ tree (e.g.
// a repo whose root happens to be named "skills"), in which case it is
// returned as-is.
func NormalizeRoot(p string) (string, error) {
	home, herr := os.UserHomeDir()
	switch {
	case p == "~" && herr == nil:
		p = home
	case strings.HasPrefix(p, "~/") && herr == nil:
		p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", common.Errf("cannot resolve --root %q: %w", p, err)
	}
	if hasRepoMarkers(abs) {
		return abs, nil
	}
	if base := filepath.Base(abs); base == "skills" || base == "commands" {
		return filepath.Dir(abs), nil
	}
	return abs, nil
}

// hasRepoMarkers reports whether dir contains a skills/ or commands/ tree.
func hasRepoMarkers(dir string) bool {
	for _, sub := range []string{"skills", "commands"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}
