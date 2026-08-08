package providers

import (
	"errors"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// DiscoverPlugins scans each base dir's "providers" subdirectory for
// executable files and loads each as a subprocess provider. A plugin that
// fails to launch, returns an empty id, or has a duplicate id is isolated
// (skipped, recorded as a ProviderLoadFailure) and never blocks startup
// (FR-035, FR-006, FR-007). Dirs are scanned in order; within a dir, files are
// sorted for a stable order (pkg/plugins.ListExecutables).
func DiscoverPlugins(baseDirs []string, logger *common.Logger) ([]Provider, []ProviderLoadFailure) {
	var out []Provider
	var failures []ProviderLoadFailure
	seen := map[string]bool{}
	for _, path := range plugins.ListExecutables(baseDirs, "providers") {
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
