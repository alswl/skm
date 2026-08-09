package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/services"
)

// E2E TUI suite: drives a real Bubble Tea program loop (tea.Program + probe
// model) with scripted key messages against the committed testdata/e2e/repo
// fixture. This exercises the actual Update/command/job path the user runs —
// not just the model methods — and is what catches receiver/closure state
// bugs that direct `m.handleKey` tests miss.

// probeModel wraps the live *model so every Update hands a value snapshot to
// the test (race-free reads), while the program itself keeps operating on the
// single heap-allocated model.
type probeModel struct {
	model *model
	snap  chan model
}

func (p probeModel) Init() tea.Cmd { return p.model.Init() }

func (p probeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := p.model.Update(msg)
	live := m.(*model)
	p.snap <- *live
	return probeModel{model: live, snap: p.snap}, cmd
}

func (p probeModel) View() string { return p.model.View() }

// blockingInput never returns from Read, so the tea.Program stays alive while
// keys are delivered via Program.Send (there is no real stdin in tests).
type blockingInput struct{}

func (blockingInput) Read([]byte) (int, error) {
	<-make(chan struct{}) // block forever
	return 0, nil
}

// programProbe is a running tea.Program with scripted-key + snapshot access.
type programProbe struct {
	t  *testing.T
	p  *tea.Program
	ch chan model
}

func startProgram(t *testing.T, m *model) *programProbe {
	t.Helper()
	ch := make(chan model, 512)
	prog := tea.NewProgram(probeModel{model: m, snap: ch},
		tea.WithInput(blockingInput{}), // keys arrive via Program.Send, not stdin
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
	)
	pp := &programProbe{t: t, p: prog, ch: ch}
	done := make(chan error, 1)
	go func() {
		_, err := prog.Run()
		done <- err
		close(done)
	}()
	t.Cleanup(func() {
		prog.Quit()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Log("program did not stop cleanly")
		}
	})
	select {
	case err := <-done:
		t.Fatalf("program exited immediately: %v", err)
	case <-time.After(200 * time.Millisecond):
		// still running — good
	}
	t.Cleanup(func() {
		prog.Quit()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Log("program did not stop cleanly")
		}
	})
	// Deterministic window size (bubbletea cannot query size on io.Discard).
	pp.p.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	// height and loading are independent, persistent model fields updated by
	// unrelated messages (WindowSizeMsg vs. the async scanDoneMsg) that can
	// arrive in either order; a single combined predicate is required because
	// wait() discards non-matching snapshots as it drains, so a one-off state
	// change (loading flipping false) would be lost if it arrived during a
	// wait for the other condition.
	pp.wait(func(s model) bool { return s.height == 30 && !s.loading }, "window size 100x30 applied and initial scan completed")
	return pp
}

// send delivers a message and returns the model snapshot of that Update.
func (pp *programProbe) send(msg tea.Msg) model {
	pp.t.Helper()
	pp.p.Send(msg)
	select {
	case s := <-pp.ch:
		return s
	case <-time.After(5 * time.Second):
		pp.t.Fatal("no model snapshot after message")
		return model{}
	}
}

// wait drains snapshots until pred holds (used for async job results).
func (pp *programProbe) wait(pred func(model) bool, failMsg string) model {
	pp.t.Helper()
	deadline := time.After(5 * time.Second)
	var last model
	for {
		select {
		case s := <-pp.ch:
			last = s
			if pred(s) {
				return s
			}
		case <-deadline:
			pp.t.Fatalf("%s\nlast: showDetail=%v status=%q picker=%v confirm=%v offset=%d",
				failMsg, last.showDetail, last.status, last.picker != nil, last.confirm != nil, last.detailOffset)
			return model{}
		}
	}
}

func keyRunes(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// fixtureProgramModel copies the committed testdata/e2e/repo to a temp dir,
// builds Services with one skill target, and returns a ready initialModel.
func fixtureProgramModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // built-in targets now always merge in (config.mergeWithBuiltins): keep their paths out of the real home dir
	root := t.TempDir()
	require.NoError(t, copyE2ERepo(t, root))
	cfgDir := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	writeFileT(t, cfgDir, "targets.json",
		`[{"name":"claude-skills","path":"`+targetDir+`","builtin":false,"accepts":["skill"],"strategies":{"skill":"skill-symlink"}}]`)
	cfg, err := config.Load(root, cfgDir)
	require.NoError(t, err)
	svc, err := services.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	m := initialModel(context.Background(), svc)
	return m
}

func copyE2ERepo(t *testing.T, dst string) error {
	t.Helper()
	return filepath.WalkDir("../../testdata/e2e/repo", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("../../testdata/e2e/repo", path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TestE2ETUIMoveFromDetailTakesEffect is the user's "move 无法生效" scenario:
// move a non-standard entry (d2) from its detail page through the real program
// loop — picker → confirm → background job — and assert the file actually
// relocates and the open detail page reflects the new state.
func TestE2ETUIMoveFromDetailTakesEffect(t *testing.T) {
	m := fixtureProgramModel(t)
	pp := startProgram(t, m)

	// d2 sorts first; Enter opens its detail page.
	s := pp.send(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, s.showDetail, "Enter opens the detail page")
	require.Equal(t, "d2", s.filtered[s.cursor].Name)

	s = pp.send(keyRunes('n'))
	require.NotNil(t, s.picker, "m opens the provider picker from the detail page")

	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter}) // choose "local" (first item)
	require.Nil(t, s.picker)
	require.NotNil(t, s.confirm, "choosing a provider previews the destination behind a confirm")

	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter}) // confirm the move
	// The job's status message and the post-job rescan land in separate
	// Update ticks now (the rescan runs off the event loop so a completing
	// job never freezes the UI — jobs_wire.go handleJobDone), so wait for the
	// rescan to actually apply rather than just the status text appearing.
	done := pp.wait(func(s model) bool {
		for _, e := range s.filtered {
			if e.Name == "d2" && e.Status == common.StatusActive {
				return true
			}
		}
		return false
	}, "move job completes and the rescan applies")

	require.Contains(t, done.status, "moved", "the status line reports the move")
	require.FileExists(t, filepath.Join(m.svc.Cfg.Root, "skills/local/d2/SKILL.md"), "d2 relocated to skills/local/d2")
	require.NoDirExists(t, filepath.Join(m.svc.Cfg.Root, "skills/d2"), "the misplaced dir is gone")

	// The moved entry is now standard (active, provider=local) in the refreshed
	// scan; the detail page stays open and reflects the post-move scan.
	require.True(t, done.showDetail, "the detail page stays open across the move")
	var d2 *common.Entry
	for _, e := range done.filtered {
		if e.Name == "d2" {
			d2 = e
		}
	}
	require.NotNil(t, d2, "d2 is still present in the refreshed list")
	require.Equal(t, common.StatusActive, d2.Status, "d2 is active after the move")
	require.Equal(t, "local", d2.ProviderIDValue(), "d2 now lives under the local provider")
}

// TestE2ETUIQAndEscOnlyGoBack: q/esc from detail and from modals return, never
// quit — the program stays alive afterwards.
func TestE2ETUIQAndEscOnlyGoBack(t *testing.T) {
	m := fixtureProgramModel(t)
	pp := startProgram(t, m)

	s := pp.send(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	require.True(t, s.showDetail)
	s = pp.send(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, s.showDetail, "esc returns to the list")

	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter}) // reopen detail
	require.True(t, s.showDetail)
	s = pp.send(keyRunes('q'))
	require.False(t, s.showDetail, "q returns to the list, does not quit")

	// Program still alive: reopen detail and drive a picker, then q closes it.
	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, s.showDetail)
	s = pp.send(keyRunes('n')) // d2 is non-standard, so move opens a provider picker
	require.NotNil(t, s.picker)
	s = pp.send(keyRunes('q'))
	require.Nil(t, s.picker, "q closes the picker, like esc")
	// Still alive: reopen detail and confirm the program responds.
	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, s.showDetail, "program is still running after q in the picker")
}

// TestE2ETUIDetailScrollsWithJK: in the running program the detail page is a
// pager — j/k scroll a window through a long entry (repo-analyzer has enough
// reference files to overflow the screen), and the offset clamps at the end.
func TestE2ETUIDetailScrollsWithJK(t *testing.T) {
	m := fixtureProgramModel(t)
	pp := startProgram(t, m)

	// The list is grouped by source, so locate repo-analyzer (the file-rich
	// entry) by index and move the cursor there with j.
	s := pp.send(keyRunes('z')) // unbound in the list view; just yields a snapshot
	idx := -1
	for i, e := range s.filtered {
		if e.Name == "repo-analyzer" {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "repo-analyzer is in the list")
	for i := 0; i < idx; i++ {
		s = pp.send(keyRunes('j'))
	}
	require.Equal(t, "repo-analyzer", s.filtered[s.cursor].Name, "j moves the list cursor")
	s = pp.send(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, s.showDetail)

	avail := map[string]bool{}
	for _, b := range s.detailBindings() {
		avail[b.Keys] = b.Enabled
	}
	require.True(t, avail["j/k"], "scroll is offered because repo-analyzer's detail overflows")

	s = pp.send(keyRunes('j'))
	require.Equal(t, 1, s.detailOffset, "j scrolls the detail window down")
	s = pp.send(keyRunes('j'))
	require.Equal(t, 2, s.detailOffset, "j scrolls further")
	s = pp.send(keyRunes('k'))
	require.Equal(t, 1, s.detailOffset, "k scrolls back up")
	s = pp.send(keyRunes('k'))
	require.Equal(t, 0, s.detailOffset, "k returns to the top")

	// Clamp at the bottom: keep pressing j until the offset stops growing.
	maxOffset := s.detailOffset
	for i := 0; i < 50; i++ {
		s = pp.send(keyRunes('j'))
		if s.detailOffset > maxOffset {
			maxOffset = s.detailOffset
		}
	}
	require.Greater(t, maxOffset, 0, "the long detail scrolls past the first window")
	require.Equal(t, maxOffset, s.detailOffset, "the offset clamps at the last window")
	require.True(t, s.showDetail, "scrolling does not leave the detail page")
}

// TestE2ETUIDetailFooterDimsUnavailableActions: in the running program the
// footer dims bindings that don't apply to the current entry (e.g. move only
// for non-standard entries; install/update only for active entries with an
// origin).
func TestE2ETUIDetailFooterDimsUnavailableActions(t *testing.T) {
	m := fixtureProgramModel(t)
	pp := startProgram(t, m)

	s := pp.send(tea.KeyMsg{Type: tea.KeyEnter}) // open d2 (non-standard)
	require.True(t, s.showDetail)
	avail := map[string]bool{}
	for _, b := range s.detailBindings() {
		avail[b.Keys] = b.Enabled
	}
	require.True(t, avail["n"], "move is offered for the non-standard entry")
	require.False(t, avail["i"], "install is disabled for a non-standard entry")
	require.False(t, avail["u"], "uninstall is disabled when nothing is installed")
	require.False(t, avail["p"], "update is disabled for an entry with no origin")
	require.True(t, avail["a"], "archive is always offered")
	require.True(t, avail["d"], "delete is always offered")
	require.True(t, avail["esc/q"], "back is always offered")
}
