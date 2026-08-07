package providers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// DiscoverPlugins scans plugin directories for executable files and loads each
// as a subprocess provider. A plugin that fails to launch, returns an empty
// id, or has a duplicate id is isolated (skipped, recorded as a
// ProviderLoadFailure) and never blocks startup (FR-035, FR-006, FR-007).
// Dirs are scanned in order; within a dir, files are sorted for a stable
// order.
func DiscoverPlugins(dirs []string, logger *common.Logger) ([]Provider, []ProviderLoadFailure) {
	var out []Provider
	var failures []ProviderLoadFailure
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
				reason := toProviderError(err)
				logger.Warn("plugin load failed (isolated)", "path", path, "err", reason.Message)
				failures = append(failures, ProviderLoadFailure{Path: path, Reason: reason})
				continue
			}
			if seen[p.ID()] {
				reason := ProviderError{Code: CodeDuplicateID, Message: fmt.Sprintf("duplicate id %q rejected in favor of the first", p.ID())}
				logger.Warn("duplicate plugin id rejected in favor of the first", "id", p.ID(), "path", path)
				failures = append(failures, ProviderLoadFailure{Path: path, ID: p.ID(), Reason: reason})
				continue
			}
			seen[p.ID()] = true
			out = append(out, p)
		}
	}
	return out, failures
}

// toProviderError unwraps a *ProviderError, or wraps any other error as a
// generic protocol_error so callers always get a typed, diagnosable reason.
func toProviderError(err error) ProviderError {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return *pe
	}
	return ProviderError{Code: CodeProtocolError, Message: err.Error()}
}

// isExecutable reports whether the file has an executable bit set.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&0o111 != 0
}
