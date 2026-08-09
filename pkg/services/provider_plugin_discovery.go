package services

import (
	"errors"
	"fmt"
	"sync"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// DiscoverPlugins scans each base dir's "providers" subdirectory for
// executable files and loads each as a subprocess provider. A plugin that
// fails to launch, returns an empty id, or has a duplicate id is isolated
// (skipped, recorded as a ProviderLoadFailure) and never blocks startup
// (FR-035, FR-006, FR-007).
//
// Loading itself — one subprocess round-trip per plugin — happens
// concurrently: this runs synchronously inside services.New(), before
// tui.Run() ever paints a frame, so N plugins used to cost N times one
// plugin's startup latency, paid as a blank terminal with no feedback
// ("初始化装载页面不会动"). The plugins are independent, so loading them at once
// turns that sum into a wait for the slowest one. Only that inner load races;
// every visible result — output order, and which of two plugins declaring the
// same id wins — is still decided from plugins.ListExecutables' stable order,
// exactly as it was sequentially (TestDiscoverRegistersPluginsInStableOrder,
// TestDiscoverRejectsDuplicateIDs).
func DiscoverPlugins(baseDirs []string, logger *common.Logger) ([]Provider, []ProviderLoadFailure) {
	paths := plugins.ListExecutables(baseDirs, "providers")
	loaded := make([]struct {
		p   Provider
		err error
	}, len(paths))
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			loaded[i].p, loaded[i].err = NewPluginProvider(path)
		}(i, path)
	}
	wg.Wait()

	var out []Provider
	var failures []ProviderLoadFailure
	seen := map[string]bool{}
	for i, path := range paths {
		p, err := loaded[i].p, loaded[i].err
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
