package installer

import (
	"errors"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// DiscoverTargetPlugins scans each base dir's "targets" subdirectory for
// executable files and loads each as a subprocess Target plugin, symmetric to
// providers.DiscoverPlugins. A plugin that fails to launch, returns an empty
// id, or has a duplicate id is isolated (skipped, recorded as a
// PluginLoadFailure) and never blocks startup. Dirs are scanned in order;
// within a dir, files are sorted for a stable order (pkg/plugins.ListExecutables).
func DiscoverTargetPlugins(baseDirs []string, logger *common.Logger) ([]*TargetPlugin, []PluginLoadFailure) {
	var out []*TargetPlugin
	var failures []PluginLoadFailure
	seen := map[string]bool{}
	for _, path := range plugins.ListExecutables(baseDirs, "targets") {
		p, err := NewTargetPlugin(path)
		if err != nil {
			reason := toTargetPluginError(err)
			logger.Warn("target plugin load failed (isolated)", "path", path, "err", reason.Message)
			failures = append(failures, PluginLoadFailure{Path: path, Reason: reason})
			continue
		}
		if seen[p.ID()] {
			reason := TargetPluginError{Code: CodeTargetDuplicateID, Message: fmt.Sprintf("duplicate id %q rejected in favor of the first", p.ID())}
			logger.Warn("duplicate target plugin id rejected in favor of the first", "id", p.ID(), "path", path)
			failures = append(failures, PluginLoadFailure{Path: path, ID: p.ID(), Reason: reason})
			continue
		}
		seen[p.ID()] = true
		out = append(out, p)
	}
	return out, failures
}

// toTargetPluginError unwraps a *TargetPluginError, or wraps any other error
// as a generic protocol_error so callers always get a typed, diagnosable
// reason.
func toTargetPluginError(err error) TargetPluginError {
	var pe *TargetPluginError
	if errors.As(err, &pe) {
		return *pe
	}
	return TargetPluginError{Code: CodeTargetProtocolError, Message: err.Error()}
}
