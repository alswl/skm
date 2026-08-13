package engines

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

// Discover keeps same-named external skills distinct and ignores symlinks.
func (r *Repository) Discover(targets []common.InstallTarget, sourceDir string) []DiscoveredSkill {
	var out []DiscoveredSkill
	for _, t := range targets {
		if !t.AcceptsKind(common.KindSkill) {
			continue
		}
		if sourceDir != "" && t.Path != sourceDir {
			continue
		}
		out = append(out, r.discoverDir(t.Path)...)
	}
	return out
}

func (r *Repository) discoverDir(dir string) []DiscoveredSkill {
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
		if !dal.PathExists(filepath.Join(p, "SKILL.md")) {
			continue // must be a real skill with a valid marker
		}
		out = append(out, DiscoveredSkill{Name: c.Name(), Path: p})
	}
	return out
}
