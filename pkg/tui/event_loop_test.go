package tui

import (
	"fmt"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// The Bubble Tea event loop runs Update and View on one goroutine: anything
// slow there stops rendering *and* input, and the screen keeps showing
// whatever frame was painted last. Install state is the trap — every probe is
// a filesystem read and, for a plugin-backed target, a subprocess round-trip,
// so a handler that probes "just the selected entry's targets" still costs one
// process spawn per target on every keypress. The model therefore keeps install
// state in memory (installStates, built off the loop by scanInstallStates) and
// every screen reads that. These tests are the guard on that rule.

// maxUpdateBlock is what counts as blocking the loop. In-memory work is
// microseconds; one plugin round-trip against the fixture below is ~60ms, so
// this cleanly separates them and catches even a single leaked probe.
const maxUpdateBlock = 30 * time.Millisecond

// blockProbe wraps the live *model and records the slowest single Update.
type blockProbe struct {
	model *model
	snap  chan model
	mu    *sync.Mutex
	worst *time.Duration
	msg   *string
}

func (p blockProbe) Init() tea.Cmd { return p.model.Init() }
func (p blockProbe) View() string  { return p.model.View() }

func (p blockProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	m, cmd := p.model.Update(msg)
	d := time.Since(start)
	p.mu.Lock()
	if d > *p.worst {
		*p.worst, *p.msg = d, describeMsg(msg)
	}
	p.mu.Unlock()
	live := m.(*model)
	select {
	case p.snap <- *live:
	default: // never block the loop when the test isn't draining
	}
	return blockProbe{model: live, snap: p.snap, mu: p.mu, worst: p.worst, msg: p.msg}, cmd
}

func describeMsg(msg tea.Msg) string {
	if k, ok := msg.(tea.KeyMsg); ok {
		return "key " + k.String()
	}
	return fmt.Sprintf("%T", msg)
}

// loopProbe is a running program plus the slowest-Update record.
type loopProbe struct {
	t     *testing.T
	prog  *tea.Program
	ch    chan model
	mu    sync.Mutex
	worst time.Duration
	msg   string
}

// startLoopProbe boots a real program over a repo whose single target resolves
// install state through a plugin, so every stray probe costs a subprocess.
func startLoopProbe(t *testing.T, entries int) *loopProbe {
	t.Helper()
	m := slowInstallStateModel(t, entries)
	lp := &loopProbe{t: t, ch: make(chan model, 512)}
	lp.prog = tea.NewProgram(
		blockProbe{model: m, snap: lp.ch, mu: &lp.mu, worst: &lp.worst, msg: &lp.msg},
		tea.WithInput(blockingInput{}), tea.WithOutput(&timedBuffer{}), tea.WithoutSignalHandler(),
	)
	done := make(chan struct{})
	go func() { _, _ = lp.prog.Run(); close(done) }()
	t.Cleanup(func() {
		lp.prog.Quit()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("program did not stop cleanly")
		}
	})
	lp.prog.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	lp.waitFor(func(s model) bool { return !s.loading && len(s.filtered) == entries }, "startup")
	lp.reset()
	return lp
}

func (lp *loopProbe) waitFor(pred func(model) bool, what string) model {
	lp.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case s := <-lp.ch:
			if pred(s) {
				return s
			}
		case <-deadline:
			lp.t.Fatalf("timed out waiting for %s", what)
			return model{}
		}
	}
}

// reset forgets the slowest Update so far — used to exclude startup.
func (lp *loopProbe) reset() {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	lp.worst, lp.msg = 0, ""
}

// press sends a key and lets any command it started settle.
func (lp *loopProbe) press(msg tea.Msg) {
	lp.prog.Send(msg)
	time.Sleep(250 * time.Millisecond)
}

func (lp *loopProbe) requireResponsive(what string) {
	lp.t.Helper()
	lp.mu.Lock()
	worst, msg := lp.worst, lp.msg
	lp.mu.Unlock()
	require.Less(lp.t, worst, maxUpdateBlock,
		"%s blocked the event loop for %v (slowest: %s) — rendering and input stop for that long, so the screen freezes on its last frame; read install state from the model instead of probing for it",
		what, worst, msg)
}

// TestKeypathsNeverBlockTheEventLoop sweeps every key that reaches a handler
// doing more than cursor arithmetic.
func TestKeypathsNeverBlockTheEventLoop(t *testing.T) {
	cases := []struct {
		what string
		key  tea.Msg
	}{
		{"opening the detail page", tea.KeyMsg{Type: tea.KeyEnter}},
		{"opening the installs picker", runeKey('i')},
		{"opening the actions menu", runeKey('x')},
		{"the fix action", runeKey('F')},
		{"discover", runeKey('o')},
		{"normalize", runeKey('n')},
		{"the targets editor", runeKey('t')},
		{"the task center", runeKey('J')},
		{"batch update", runeKey('P')},
		{"the import prompt", runeKey('m')},
		{"archive", runeKey('a')},
		{"search", runeKey('/')},
		{"toggling archived", runeKey('.')},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			lp := startLoopProbe(t, 3)
			lp.press(tc.key)
			lp.requireResponsive(tc.what)
		})
	}
}

// TestJobCompletionNeverBlocksTheEventLoop covers the path the user hit as
// "删除 task 会卡 UI": the rescan a finished job triggers is already off the
// loop, but applying it used to re-probe install state for the open detail
// page, freezing everything for as long as that took.
func TestJobCompletionNeverBlocksTheEventLoop(t *testing.T) {
	for _, fromDetail := range []bool{false, true} {
		what := "deleting from the list"
		if fromDetail {
			what = "deleting with the detail page open"
		}
		t.Run(what, func(t *testing.T) {
			lp := startLoopProbe(t, 3)
			if fromDetail {
				lp.prog.Send(tea.KeyMsg{Type: tea.KeyEnter})
				lp.waitFor(func(s model) bool { return s.showDetail }, "detail page")
				lp.reset()
			}
			lp.prog.Send(runeKey('d'))
			lp.prog.Send(runeKey('y'))
			lp.waitFor(func(s model) bool { return len(s.filtered) == 2 }, "the delete to land")
			lp.requireResponsive(what)
		})
	}
}
