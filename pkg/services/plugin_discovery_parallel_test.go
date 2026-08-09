package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
	"github.com/stretchr/testify/require"
)

// Every plugin found by discovery pays a subprocess spawn to answer "id", and
// discovery blocked on them one at a time — with N slow plugins that's N times
// one plugin's startup cost, paid synchronously before tui.Run ever paints a
// frame ("初始化装载页面不会动" turned out to have a second cause besides the
// event-loop freeze already fixed: services.New(), which both discoveries run
// under, executes entirely before the alt-screen is even entered). The
// plugins are independent, so loading them concurrently turns that sum into
// something closer to a single wait for the slowest one.
//
// The tests below compare DiscoverPlugins/DiscoverTargetPlugins against a
// forced-sequential load of the exact same plugins, measured moments later in
// the same run, rather than against an absolute wall-clock bound: this
// sandbox's process-spawn cost varies enough on its own (450ms-1s+ per
// subprocess, observed directly, for reasons outside this code — spawning
// several processes at once here does not get anywhere near the speedup a
// bare-metal machine would show) that a fixed threshold derived from the
// configured delay is either too tight to pass reliably or too loose to catch
// a real regression. A same-machine, same-moment ratio is stable either way:
// if this ever reverts to loading one plugin at a time, the two loops
// measured below are doing identical work in identical order and their
// timings converge to within a few percent of each other (ratio ~1.0); actual
// concurrency stays meaningfully below that even when contended (observed
// ~0.7-0.8 here), which the threshold has ample margin to tell apart from a
// reversion.

func writeDelayedPlugin(t *testing.T, dir, name, id string, delay time.Duration) {
	t.Helper()
	// `sleep 200ms` (Go's Duration.String()) is a GNU coreutils-ism; BSD/macOS
	// sleep rejects it outright ("invalid time interval") and the plugin
	// responds instantly, which silently defeats the whole point of this test.
	// Plain fractional seconds work on both.
	script := fmt.Sprintf("#!/bin/sh\nsleep %g\nIFS= read -r line\ncase \"$line\" in\n"+
		"  *'\"action\":\"id\"'*) echo '{\"id\":\"%s\"}';;\n"+
		"  *) echo '{\"error\":\"unknown action\"}';;\nesac\n", delay.Seconds(), id)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
}

// requireMeaningfullyFasterThanSequential asserts concurrent — measured
// moments earlier over the exact plugins in dir/subdir — beats loading those
// same plugins one at a time, load, by a margin only real concurrency
// produces (see the package comment above for why a ratio rather than an
// absolute bound).
func requireMeaningfullyFasterThanSequential(t *testing.T, concurrent time.Duration, dir, subdir string, load func(path string) error) {
	t.Helper()
	start := time.Now()
	for _, path := range plugins.ListExecutables([]string{dir}, subdir) {
		require.NoError(t, load(path))
	}
	sequential := time.Since(start)
	require.Less(t, concurrent, sequential*9/10,
		"concurrent discovery (%v) is not meaningfully faster than loading the same plugins one at a time (%v) — looks sequential, not concurrent",
		concurrent, sequential)
}

func TestDiscoverPluginsRunsConcurrently(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	const n, delay = 6, 300 * time.Millisecond
	for i := 0; i < n; i++ {
		writeDelayedPlugin(t, providersDir, fmt.Sprintf("p%d.sh", i), fmt.Sprintf("p%d", i), delay)
	}

	start := time.Now()
	loaded, failures := DiscoverPlugins([]string{dir}, common.NewLogger(false))
	elapsed := time.Since(start)
	require.Empty(t, failures)
	require.Len(t, loaded, n)

	requireMeaningfullyFasterThanSequential(t, elapsed, dir, "providers", func(path string) error {
		_, err := NewPluginProvider(path)
		return err
	})
}

func TestDiscoverTargetPluginsRunsConcurrently(t *testing.T) {
	dir := t.TempDir()
	targetsDir := filepath.Join(dir, "targets")
	require.NoError(t, os.MkdirAll(targetsDir, 0o755))
	const n, delay = 6, 300 * time.Millisecond
	for i := 0; i < n; i++ {
		writeDelayedPlugin(t, targetsDir, fmt.Sprintf("p%d.sh", i), fmt.Sprintf("p%d", i), delay)
	}

	start := time.Now()
	loaded, failures := DiscoverTargetPlugins([]string{dir}, common.NewLogger(false))
	elapsed := time.Since(start)
	require.Empty(t, failures)
	require.Len(t, loaded, n)

	requireMeaningfullyFasterThanSequential(t, elapsed, dir, "targets", func(path string) error {
		_, err := NewTargetPlugin(path)
		return err
	})
}
