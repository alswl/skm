package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/services"
)

// Startup-responsiveness E2E: unlike the other program tests these render into
// a real buffer instead of io.Discard and assert on *when* bytes reach the
// terminal, because the bug they cover ("初始化装载页面不会动") is invisible at
// the model level — every frame the model produces is correct, they just stop
// being produced while the event loop is busy.

// timedBuffer is an io.Writer the Bubble Tea renderer goroutine writes to while
// the test goroutine reads it, recording when each renderer flush landed. A
// long gap between flushes is exactly what the user sees as a frozen screen.
type timedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	times []time.Time
}

func (b *timedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.times = append(b.times, time.Now())
	return b.buf.Write(p)
}

func (b *timedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// maxGapBefore returns the longest interval with no renderer flush, measured
// from start until the first flush at or after end.
func (b *timedBuffer) maxGapBefore(start, end time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	prev := start
	var worst time.Duration
	for _, ts := range b.times {
		if ts.After(end) {
			break
		}
		if d := ts.Sub(prev); d > worst {
			worst = d
		}
		prev = ts
	}
	// The silence that matters most is the trailing one — the frame the screen
	// was stuck on while the event loop was busy is the *last* one flushed.
	if d := end.Sub(prev); d > worst {
		worst = d
	}
	return worst
}

// slowStatePluginTarget writes a target plugin whose `state` action sleeps,
// and returns a targets.json entry bound to it via strategy "plugin:<id>".
// Real plugin-backed targets pay a subprocess spawn per state probe; the sleep
// just makes that cost large enough to measure deterministically.
func slowStatePluginTarget(t *testing.T, pluginBase, id, delay string) {
	t.Helper()
	dir := filepath.Join(pluginBase, "targets")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"` + id + `"}';;
  *'"action":"label"'*) echo '{"label":"Slow ` + id + `"}';;
  *'"action":"state"'*) sleep ` + delay + `; echo '{"state":"absent"}';;
  *) echo '{"error":"unknown action"}';;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, id), []byte(script), 0o755))
}

// slowInstallStateModel builds a model over a repo of `count` skills whose one
// configured target resolves install state through the slow plugin above, so
// the per-entry install-status probing that feeds the list columns costs
// roughly count*delay.
func slowInstallStateModel(t *testing.T, count int) *model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	for _, n := range skillNames(count) {
		writeFileT(t, root, "skills/local/"+n+"/SKILL.md",
			"---\nname: "+n+"\ndescription: entry "+n+"\n---\nbody\n")
	}
	pluginBase := t.TempDir()
	t.Setenv(config.EnvPluginsDir, pluginBase)
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	slowStatePluginTarget(t, pluginBase, "slowtgt", "0.05")

	cfgDir := t.TempDir()
	writeFileT(t, cfgDir, "targets.json",
		`[{"name":"slow","path":"`+targetDir+`","builtin":false,"accepts":["skill"],"strategies":{"skill":"plugin:slowtgt"}}]`)
	cfg, err := config.Load(root, cfgDir)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.PluginDirs, "plugin dir must be discoverable for the slow target to load")
	svc, err := services.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	require.Contains(t, svc.TargetPlugins, "slowtgt", "slow target plugin failed to load")
	return initialModel(context.Background(), svc)
}

func skillNames(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, "skill-"+string(rune('a'+i)))
	}
	return out
}

// TestStartupNeverFreezesTheLoadingScreen is the regression test for
// "初始化装载页面不会动". Deriving each entry's per-target install state does
// filesystem I/O and, for plugin-backed targets, one subprocess call per
// (entry, target) pair. Doing that while applying the scan result means it runs
// inside Update, on the Bubble Tea event loop: rendering and input stop for its
// full duration, and since the last painted frame is the loading screen, the
// spinner sits frozen there until it finishes.
func TestStartupNeverFreezesTheLoadingScreen(t *testing.T) {
	const entries = 10
	m := slowInstallStateModel(t, entries)
	out := &timedBuffer{}
	ch := make(chan model, 512)
	prog := tea.NewProgram(probeModel{model: m, snap: ch},
		tea.WithInput(blockingInput{}),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	)
	done := make(chan struct{})
	go func() { _, _ = prog.Run(); close(done) }()
	t.Cleanup(func() {
		prog.Quit()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("program did not stop cleanly")
		}
	})

	start := time.Now()
	prog.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Drain snapshots until the list is populated, i.e. startup is over.
	deadline := time.After(10 * time.Second)
	for done := false; !done; {
		select {
		case s := <-ch:
			if !s.loading && len(s.filtered) == entries {
				done = true
			}
		case <-deadline:
			t.Fatal("startup never completed")
		}
	}
	settled := time.Now()

	// The spinner ticks at 10fps, so while startup is in progress the renderer
	// should never go quiet for much longer than that.
	gap := out.maxGapBefore(start, settled)
	require.Less(t, gap, 300*time.Millisecond,
		"UI froze for %v during startup: the event loop was blocked, so the loading screen stayed on its last painted frame", gap)

	time.Sleep(100 * time.Millisecond) // let the renderer's framerate ticker flush the final frame
	rendered := out.String()
	require.Contains(t, rendered, "scanning skills", "loading screen was never painted")
	require.Contains(t, rendered, "skill-a", "list was never painted")

	var seen []string
	for _, f := range spinner.Line.Frames {
		if strings.Contains(rendered, f+" scanning skills") {
			seen = append(seen, f)
		}
	}
	require.GreaterOrEqual(t, len(seen), 2,
		"loading screen never animated: only spinner frames %q reached the terminal", seen)
}
