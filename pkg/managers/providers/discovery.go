package providers

import (
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// DiscoverPlugins scans plugin directories for executable files and loads each
// as a subprocess provider. A plugin that fails to launch, returns an empty
// id, or has a duplicate id is isolated (logged, skipped) and never blocks
// startup (FR-035). Dirs are scanned in order; within a dir, files are sorted
// for a stable order.
func DiscoverPlugins(dirs []string, logger *common.Logger) []Provider {
	var out []Provider
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // missing/unreadable dir is not an error
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if !isExecutable(path) {
				continue
			}
			p, err := NewPluginProvider(path)
			if err != nil {
				logger.Warn("plugin load failed (isolated)", "path", path, "err", err.Error())
				continue
			}
			if seen[p.ID()] {
				logger.Warn("duplicate plugin id rejected in favor of the first", "id", p.ID(), "path", path)
				continue
			}
			seen[p.ID()] = true
			out = append(out, p)
		}
	}
	return out
}

// isExecutable reports whether the file has an executable bit set.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&0o111 != 0
}
