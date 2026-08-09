package services

import (
	"errors"
	"fmt"
	"sync"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// DiscoverTargetPlugins scans each base dir's "targets" subdirectory for
// executable files and loads each as a subprocess Target plugin, symmetric to
// DiscoverPlugins — including loading them concurrently, for the same reason;
// see DiscoverPlugins' comment. A plugin that fails to launch, returns an
// empty id, or has a duplicate id is isolated (skipped, recorded as a
// PluginLoadFailure) and never blocks startup. Output order and duplicate-id
// resolution still come from plugins.ListExecutables' stable order, exactly
// as they did sequentially.
func DiscoverTargetPlugins(baseDirs []string, logger *common.Logger) ([]*TargetPlugin, []PluginLoadFailure) {
	paths := plugins.ListExecutables(baseDirs, "targets")
	loaded := make([]struct {
		p   *TargetPlugin
		err error
	}, len(paths))
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			loaded[i].p, loaded[i].err = NewTargetPlugin(path)
		}(i, path)
	}
	wg.Wait()

	var out []*TargetPlugin
	var failures []PluginLoadFailure
	seen := map[string]bool{}
	for i, path := range paths {
		p, err := loaded[i].p, loaded[i].err
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
