package repository

import (
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// DiscoveredSkill is an external unmanaged skill found in an install target.
type DiscoveredSkill struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Discover reports real, valid, non-duplicate external skills in install
// targets: real directories containing a valid SKILL.md with no same-named
// repository entry. Symlinks are ignored and never deleted (FR-027). When
// sourceDir is non-empty only that directory is scanned.
func (r *Repository) Discover(targets []common.InstallTarget, sourceDir string) []DiscoveredSkill {
	existing := r.entryNames()
	var out []DiscoveredSkill
	for _, t := range targets {
		if t.Kind != common.KindSkill {
			continue
		}
		if sourceDir != "" && t.Path != sourceDir {
			continue
		}
		out = append(out, r.discoverDir(t.Path, existing)...)
	}
	return out
}

func (r *Repository) discoverDir(dir string, existing map[string]bool) []DiscoveredSkill {
	var out []DiscoveredSkill
	children, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, c := range children {
		p := filepath.Join(dir, c.Name())
		if !c.IsDir() {
			continue
		}
		if dal.IsSymlink(p) {
			continue // managed symlink / foreign link: never reported, never deleted
		}
		if existing[c.Name()] {
			continue // duplicate of a repository entry
		}
		if !dal.PathExists(filepath.Join(p, "SKILL.md")) {
			continue // must be a real skill with a valid marker
		}
		out = append(out, DiscoveredSkill{Name: c.Name(), Path: p})
	}
	return out
}

// entryNames returns the set of entry names in the repository.
func (r *Repository) entryNames() map[string]bool {
	names := map[string]bool{}
	for _, e := range r.Scan() {
		names[e.Name] = true
	}
	return names
}
