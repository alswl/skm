package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/components"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	"github.com/alswl/skm/skm/pkg/utils/pagination"
	"github.com/stretchr/testify/require"
)

func writeFileT(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func newTestModel(t *testing.T) model {
	t.Helper()
	// Stable, style-stripped rendering so View snapshots are deterministic
	// (go-tui-guides.md: strip styling in tests).
	prev := termenv.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	t.Setenv("HOME", t.TempDir()) // built-in targets now always merge in (config.mergeWithBuiltins): keep their paths out of the real home dir
	root := t.TempDir()
	writeFileT(t, root, "skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: alpha skill\n---\nbody\n")
	writeFileT(t, root, "skills/local/skill-b/SKILL.md", "---\nname: skill-b\ndescription: beta skill\n---\nbody\n")
	cfgDir := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	writeFileT(t, cfgDir, "targets.json", `[{"name":"t","path":"`+targetDir+`","builtin":false,"kind":"skill"}]`)

	cfg, err := config.Load(root, cfgDir)
	require.NoError(t, err)
	svc, err := services.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	m := initialModel(t.Context(), svc)
	m.width, m.height = 100, 30
	m.pageSize = 20
	m.help.Width = m.width
	m.loading = false
	m.applyScan(svc.Scan())
	return *m // a value copy for direct model tests; the program runs the pointer
}

// applyScan is the direct-model-test counterpart of the scanDoneMsg path:
// production derives the install columns in scanCmd, off the event
// loop (scanInstallStates), and hands them to applyEntries. Tests that drive
// the model directly have no such command, so they derive them inline.
func (m *model) applyScan(entries []*common.Entry) {
	m.applyEntries(entries, scanInstallStates(m.svc, entries))
}

// newLoadingTestModel builds a raw initialModel (loading, unscanned) using the
// same fixture repo as newTestModel, for tests that exercise the async-scan
// transition itself rather than a fully-loaded model.
func newLoadingTestModel(t *testing.T) (*model, *services.Services) {
	t.Helper()
	prev := termenv.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	t.Setenv("HOME", t.TempDir()) // built-in targets now always merge in (config.mergeWithBuiltins): keep their paths out of the real home dir
	root := t.TempDir()
	writeFileT(t, root, "skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: alpha skill\n---\nbody\n")
	cfgDir := t.TempDir()
	cfg, err := config.Load(root, cfgDir)
	require.NoError(t, err)
	svc, err := services.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	m := initialModel(t.Context(), svc)
	m.width, m.height = 100, 30
	m.help.Width = m.width
	return m, svc
}

func TestInitialModelStartsLoading(t *testing.T) {
	m, _ := newLoadingTestModel(t)
	require.True(t, m.loading)
	require.Empty(t, m.filtered)
	require.Contains(t, m.View(), "scanning")
}

func TestScanDoneMsgStopsLoading(t *testing.T) {
	m, svc := newLoadingTestModel(t)
	_, _ = m.Update(scanDoneMsg{entries: svc.Scan()})
	require.False(t, m.loading)
	require.NotEmpty(t, m.filtered)
}

// TestScanDoneMsgDoesNotDoubleArmResultListener: Init's initial Batch
// (spinner.Tick, scanCmd, waitForResult) already starts the one persistent
// listener on the job-results channel. scanDoneMsg used to also return a
// second waitForResult, leaving a permanent orphan goroutine blocked on
// m.queue.Results() after every startup (jobs_wire.go handleJobDone re-arms
// its own listener on every job completion, so this second one is never
// needed and never consumed).
func TestScanDoneMsgDoesNotDoubleArmResultListener(t *testing.T) {
	m, svc := newLoadingTestModel(t)
	_, cmd := m.Update(scanDoneMsg{entries: svc.Scan()})
	require.Nil(t, cmd, "scanDoneMsg must not arm a second result listener; Init already started one")
}

// TestScanReasonDecidesWhatApplyingAScanReportsBesidesTheEntries: the three
// occasions that trigger a scan (startup, a finished job, the manual R) share
// one command and one message and differ only by reason — startup is the only
// one that clears the loading spinner, and R is the only one that reports a
// count, so a job's rescan never steals the status line from whatever the job
// itself just reported.
func TestScanReasonDecidesWhatApplyingAScanReportsBesidesTheEntries(t *testing.T) {
	entriesOf := func(svc *services.Services) []*common.Entry { return svc.Scan() }

	t.Run("initial clears loading and says nothing", func(t *testing.T) {
		m, svc := newLoadingTestModel(t)
		_, _ = m.Update(scanDoneMsg{reason: scanInitial, entries: entriesOf(svc)})
		require.False(t, m.loading)
		require.Empty(t, m.status)
	})

	t.Run("after a job keeps the job's own status", func(t *testing.T) {
		m, svc := newLoadingTestModel(t)
		m.loading = false
		m.setStatus("installed skill-a into claude")
		_, _ = m.Update(scanDoneMsg{reason: scanAfterJob, entries: entriesOf(svc)})
		require.Equal(t, "installed skill-a into claude", m.status,
			"a post-job rescan must not overwrite what the job reported")
		require.NotEmpty(t, m.filtered)
	})

	t.Run("manual refresh reports the count", func(t *testing.T) {
		m, svc := newLoadingTestModel(t)
		m.loading = false
		_, _ = m.Update(scanDoneMsg{reason: scanManual, entries: entriesOf(svc)})
		require.Equal(t, "refreshed: 1 entries", m.status)
	})
}

// TestRefreshKeyRunsTheScanOffTheEventLoop: R must hand back a command rather
// than scanning inline, since the scan is exactly the filesystem walk that
// freezes rendering and input when it happens on the event loop.
func TestRefreshKeyRunsTheScanOffTheEventLoop(t *testing.T) {
	m := newTestModel(t)
	cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	require.NotNil(t, cmd, "R must return a scan command")
	msg, ok := cmd().(scanDoneMsg)
	require.True(t, ok, "the command must produce a scan result")
	require.Equal(t, scanManual, msg.reason)
	require.NotEmpty(t, msg.entries)
	require.NotNil(t, msg.cols, "install states are derived off the event loop with the entries")
}

// TestListAndDetailShowProviderIcon: each entry's row and its detail page are
// prefixed with the icon its provider declared (Capability().Icon) — here the
// fixture entries live under skills/local/…, so their mode_id is "local" and
// the built-in Local provider's icon (📂) should appear in both places.
func TestListAndDetailShowProviderIcon(t *testing.T) {
	m := newTestModel(t)
	require.Contains(t, m.View(), "📂", "list row shows the provider icon")

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Contains(t, m.View(), "📂 local", "detail page shows the icon next to the provider")
}

// TestProviderIconDistinguishesSelfBuildAndUnknown: an entry moved to the
// "self-build" bucket (pkg/tui/actions_normalize.go's always-offered move
// destination) shows the SelfBuild provider's icon; one with a ProviderID that
// resolves to no registered provider (including "", none recorded) falls
// back to unknownProviderIcon instead of a blank column.
func TestProviderIconDistinguishesSelfBuildAndUnknown(t *testing.T) {
	m := newTestModel(t)
	require.Equal(t, "🍺", m.providerIcon("self-build"))
	require.Equal(t, unknownProviderIcon, m.providerIcon(""))
	require.Equal(t, unknownProviderIcon, m.providerIcon("some-unregistered-provider"))
}

func TestModelRendersListAndOpensDetail(t *testing.T) {
	m := newTestModel(t)
	view := m.View()
	require.Contains(t, view, "skill-a")
	require.Contains(t, view, "skill-b")
	require.NotContains(t, view, "alpha skill", "description is shown only in the detail page")
	require.Contains(t, view, "install", "help bar renders")

	// Enter opens the full-screen detail page for the selected entry.
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.showDetail)
	require.Contains(t, m.View(), "alpha skill", "detail page shows the description")
	require.Contains(t, m.View(), "marker preview")

	// Esc returns to the nnn-style list.
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.showDetail)
	require.Contains(t, m.View(), "skill-a")
}

func TestModelSearchFilters(t *testing.T) {
	m := newTestModel(t)
	m.searching = true
	for _, r := range "skill-b" {
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	require.Equal(t, "skill-b", m.search)
	require.Len(t, m.filtered, 1, "filter matches name")
	require.Equal(t, "skill-b", m.filtered[0].Name)
}

func TestModelInstallActionRefreshesStatus(t *testing.T) {
	m := newTestModel(t)
	// `i` opens the installs picker; checking a target requests installation.
	_ = m.handleKey(runeKey('i'))
	require.NotNil(t, m.picker, "installs opens a target picker")
	require.Contains(t, m.picker.Title, "Installs")
	for i := range m.picker.Items {
		m.picker.Items[i].Checked = true
	}
	// Confirm the picker to submit the requested installation changes.
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.picker, "picker closes on confirm")

	// The install runs on the background queue: drain the result and apply it.
	drainJob(t, &m)
	require.Contains(t, m.status, "installed")
	// The managed link must now exist in the target.
	for _, tgt := range m.svc.Installer.Targets(m.filtered[0]) {
		require.Equal(t, common.InstallInstalled, m.svc.Installer.State(m.filtered[0], tgt))
	}
	// After the scan, the install-status column reflects the managed target.
	require.Contains(t, m.View(), "✓", "install column shows the installed icon")
}

func TestInstallStatusMessageReportsActualChanges(t *testing.T) {
	result := &services.InstallResult{Results: []common.InstallReport{
		{Target: "a", Changed: true},
		{Target: "b", Changed: false},
		{Target: "c", Changed: true},
	}}
	require.Equal(t, "uninstalled demo across 2 target(s); 1 unchanged", installStatusMessage("uninstall", "demo", result))

	result.Results = []common.InstallReport{{Target: "a", Changed: false}}
	require.Equal(t, "uninstall demo: no managed installs changed", installStatusMessage("uninstall", "demo", result))
}

func TestListFooterShowsInstallAndImportBindings(t *testing.T) {
	m := newTestModel(t)
	hint := m.listHint()
	require.Contains(t, hint, "[i] installs")
	require.Contains(t, hint, "[m] import")
	require.NotContains(t, hint, "[u] uninstall")
}

// TestActionsMenuListsOnlyAvailableEntryActions: `x` opens a single-select
// picker listing only the actions currently available for the highlighted
// entry — the same availability rules the list/detail footers already use
// (listBindings/detailBindings) — so a menu item is never offered only to
// reject it on selection.
func TestActionsMenuListsOnlyAvailableEntryActions(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker, "x opens the actions menu")
	require.Contains(t, m.picker.Title, "actions")
	require.True(t, m.picker.Single)

	var labels []string
	for _, it := range m.picker.Items {
		labels = append(labels, it.Label)
	}
	require.Contains(t, labels, "[i] installs", "install is offered for an active entry with targets")
	require.Contains(t, labels, "[a] archive", "archive is always offered")
	require.Contains(t, labels, "[d] delete", "delete is always offered")
	require.NotContains(t, labels, "[p] update", "update is not offered: the fixture entry has no origin")
	require.Contains(t, labels, "[n] normalize", "normalize is offered: an active entry can be relocated to another provider")
}

// TestActionsMenuRunsSelectedAction: confirming a choice in the actions menu
// runs the exact same handler its own key would (here, "installs" opens the
// installs picker, same as pressing `i` directly).
func TestActionsMenuRunsSelectedAction(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker)

	idx := -1
	for i, it := range m.picker.Items {
		if it.Label == "[i] installs" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0, "installs is offered")
	m.picker.Cursor = idx
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, m.picker, "choosing installs opens the installs picker, same as pressing i")
	require.Contains(t, m.picker.Title, "Installs")
}

// TestActionsMenuOmitsDetailWhenAlreadyOpen: opening the menu from the detail
// page itself must not offer "detail" again — it's the page you're already on.
func TestActionsMenuOmitsDetailWhenAlreadyOpen(t *testing.T) {
	m := newTestModel(t)
	m.openDetail()
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker)
	for _, it := range m.picker.Items {
		require.NotEqual(t, "[enter] detail", it.Label)
	}
}

// TestActionsMenuIncludesGlobalListActions: the menu also offers the
// list-scoped actions that already have their own keys (discover, import,
// batch update, targets, task queue) alongside the entry-specific ones, not
// just per-entry actions — otherwise the menu only ever shows 3-5 items and
// undersells what's actually reachable ("x 里面内容感觉有点少").
func TestActionsMenuIncludesGlobalListActions(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker)

	var labels []string
	for _, it := range m.picker.Items {
		labels = append(labels, it.Label)
	}
	require.Contains(t, labels, "[o] discover")
	require.Contains(t, labels, "[m] import")
	require.Contains(t, labels, "[P] batch update")
	require.Contains(t, labels, "[t] targets")
	require.Contains(t, labels, "[J] job queue")
}

// TestActionsMenuGlobalActionRuns: selecting "import" from the menu actually
// enters import mode, the same as pressing `m` directly.
func TestActionsMenuGlobalActionRuns(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker)

	idx := -1
	for i, it := range m.picker.Items {
		if it.Label == "[m] import" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0)
	m.picker.Cursor = idx
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.picker)
	require.True(t, m.importing, "choosing import from the menu enters import mode, same as pressing m")
}

// TestActionsMenuOmitsGlobalActionsFromDetail: global actions are offered
// only from the list, not from detail — none of these keys do anything while
// showDetail is true today (handleDetailKey has no cases for them), and
// import in particular renders its address prompt only inside listView's
// status line, so enabling it from detail would swallow keystrokes into an
// invisible prompt.
func TestActionsMenuOmitsGlobalActionsFromDetail(t *testing.T) {
	m := newTestModel(t)
	m.openDetail()
	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker)
	for _, it := range m.picker.Items {
		require.NotContains(t, []string{"[o] discover", "[m] import", "[P] batch update", "[t] targets", "[J] job queue"}, it.Label)
	}
}

// TestActionsMenuOpensWithGlobalActionsWhenListEmpty: even with zero entries
// (e.g. a search with no matches), `x` must still open with the global
// actions — it shouldn't require a selectable entry to exist.
func TestActionsMenuOpensWithGlobalActionsWhenListEmpty(t *testing.T) {
	m := newTestModel(t)
	m.search = "no-such-entry-matches-this"
	m.refreshFiltered()
	require.Empty(t, m.filtered)

	_ = m.handleKey(runeKey('x'))
	require.NotNil(t, m.picker, "the actions menu still opens with an empty list")
	require.Contains(t, m.picker.Title, "actions")
	var labels []string
	for _, it := range m.picker.Items {
		labels = append(labels, it.Label)
	}
	require.Contains(t, labels, "[o] discover")
}

func TestInstallsPickerRemovesUncheckedManagedTarget(t *testing.T) {
	m := newTestModel(t)
	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)

	_ = m.handleKey(runeKey('i'))
	require.NotNil(t, m.picker)
	for i := range m.picker.Items {
		if m.picker.Items[i].Value == "t" {
			m.picker.Items[i].Checked = false
		}
	}
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, m.confirm, "removing an installed target requires confirmation")
	require.Contains(t, m.confirm.Prompt, "Remove: t")
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)

	entry := m.svc.FindEntry("skill-a")
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	require.Equal(t, common.InstallAbsent, m.svc.Installer.State(entry, target))
}

// TestListShowsInstallStatusForNonStandardEntries: scanInstallStates evaluates
// every entry (not just active ones), so a non-standard entry's install state
// is visible in the list ("安装状态无法在 list 页面看到" fix).
func TestListShowsInstallStatusForNonStandardEntries(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/flat-skill/SKILL.md", "---\nname: flat-skill\ndescription: misplaced\n---\nbody\n")
	m.applyScan(m.svc.Scan())
	// installStates is keyed by the entry's path (identity), not its name.
	flat := m.svc.FindEntry("flat-skill")
	require.NotNil(t, flat)
	flatPath := flat.Path
	// One cell per configured target: the fixture's custom "t" target plus
	// the 4 built-ins, always merged in (config.mergeWithBuiltins). Install
	// into the first (built-in, always within the rendered column width)
	// rather than "t", which the view truncates off at this test width.
	require.Len(t, m.installs[flatPath], len(m.svc.Cfg.Targets))
	tIdx := 0
	require.Equal(t, "claude-skills", m.svc.Cfg.Targets[tIdx].Name)
	require.Equal(t, common.InstallAbsent, m.installs[flatPath][tIdx].state, "absent install remains visible in model state")

	// Install flat-skill into the skill-accepting target via the installer.
	entry := m.svc.FindEntry("flat-skill")
	require.NotNil(t, entry)
	target := m.svc.Cfg.Targets[tIdx]
	tx := &dal.FileTransaction{}
	_, err := m.svc.Installer.Install(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()

	m.applyScan(m.svc.Scan())
	require.Equal(t, common.InstallInstalled, m.installs[flatPath][tIdx].state, "a non-standard entry's install state is now visible in the list")
	require.Contains(t, m.View(), "✓", "the install column renders the installed icon")
}

// TestArchivedEntryHasNoInstallStateCells: installStates is keyed by the
// entry's path (identity), not its name, and scanInstallStates skips archived
// entries (they cannot be installed; models.go). A same-named active+archived
// pair must therefore never share cells — previously the archived row rendered
// the active entry's install state, showing e.g. an archived dima as
// "installed" (an impossible state). Archived rows now render blank n/a cells.
func TestArchivedEntryHasNoInstallStateCells(t *testing.T) {
	m := newTestModel(t)
	root := m.svc.Cfg.Root
	writeFileT(t, root, "skills/local/dima/SKILL.md", "---\nname: dima\ndescription: new active\n---\nbody\n")
	writeFileT(t, root, "archived/dima/SKILL.md", "---\nname: dima\ndescription: old archived\n---\nbody\n")
	m.applyScan(m.svc.Scan())

	active := m.svc.FindEntry("skills/local/dima")
	archived := m.svc.FindEntry("archived/dima")
	require.NotNil(t, active)
	require.NotNil(t, archived)
	require.Equal(t, common.StatusArchived, archived.Status)

	// The active entry owns its own install state (path-keyed)...
	require.NotEmpty(t, m.installs[active.Path], "the active entry's install state is recorded")
	// ...and the archived copy has none: it is skipped by scanInstallStates,
	// and keyed by path it can never see the active entry's cells.
	_, ok := m.installs[archived.Path]
	require.False(t, ok, "archived entry must have no install-state cells of its own")
	for _, c := range archivedInstallCells(m.svc.Cfg.Targets) {
		require.Equal(t, components.InstallNA, c.state, "archived rows render blank n/a cells, not a real install state")
	}
}

// TestInstallsPickerLeavesConflictUntouchedUntilExplicitlyChanged protects no-op by default.
func TestInstallsPickerLeavesConflictUntouchedUntilExplicitlyChanged(t *testing.T) {
	m := newTestModel(t)
	require.Equal(t, "skill-a", m.filtered[0].Name)
	entry := m.filtered[0]
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	conflictDir := filepath.Join(target.Path, entry.Name)
	require.NoError(t, os.MkdirAll(conflictDir, 0o755))
	writeFileT(t, conflictDir, "occupied.txt", "foreign content")

	m.applyScan(m.svc.Scan())
	require.Equal(t, common.InstallConflict, m.svc.Installer.State(entry, target), "the foreign dir is a conflict")

	m.cursor = 0
	_ = m.handleKey(runeKey('i'))
	require.NotNil(t, m.picker, "installs opens a target picker")
	require.Contains(t, m.picker.Title, "Installs")
	idx := -1
	for i, item := range m.picker.Items {
		if item.Value == "t" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0)
	require.True(t, m.picker.Items[idx].Indeterminate)
	require.Contains(t, m.pickerView(), "[-]")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.picker, "picker closes on confirm")
	require.Nil(t, m.confirm)
	require.Empty(t, m.queue.Snapshot().Pending)
	require.DirExists(t, conflictDir, "an untouched conflict stays in place")

	// Only an explicit unchecked state authorizes removal.
	_ = m.handleKey(runeKey('i'))
	m.picker.Cursor = idx
	_ = m.handlePickerKey(runeKey(' '))
	require.True(t, m.picker.Items[idx].Checked)
	require.False(t, m.picker.Items[idx].Indeterminate)
	_ = m.handlePickerKey(runeKey(' '))
	require.False(t, m.picker.Items[idx].Checked)
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, m.confirm, "explicitly removing a foreign object requires confirmation")
	require.Contains(t, m.confirm.Prompt, "Remove foreign object")
	require.Contains(t, m.confirm.Prompt, "t", "the conflicted target is named")

	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.False(t, dal.PathExists(conflictDir), "the foreign object is removed")
	require.Equal(t, common.InstallAbsent, m.svc.Installer.State(entry, target))
}

// TestDetailPageShowsRepoPath: the detail page must show the entry's
// repo-relative path (its identity), not just its name — so a same-named
// active/archived pair is distinguishable and the skill's location is visible.
func TestDetailPageShowsRepoPath(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail on the first entry
	require.True(t, m.showDetail, "enter opens the detail page")
	require.Contains(t, m.View(), "skills/local/skill-a", "the detail page shows the repo-relative path")
}

// drainJob waits for one queued job result and applies it, including running
// the deferred post-job rescan command (handleJobDone no longer scans
// synchronously; see TestJobDoneRescanIsAsync).
func drainJob(t *testing.T, m *model) {
	t.Helper()
	select {
	case r := <-m.queue.Results():
		if cmd := m.handleJobDone(r); cmd != nil {
			_, _ = m.Update(cmd())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("background job did not complete")
	}
}

// TestJobDoneRescanIsAsync: applying a job result must not block Update with
// a synchronous filesystem rescan (handleJobDone used to call m.svc.Scan()
// directly), or every job completion freezes rendering and input handling for
// as long as the scan takes ("job 动作进行时候，UI 会被卡住"). The rescan must
// be deferred to a returned tea.Cmd instead.
func TestJobDoneRescanIsAsync(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/skill-c/SKILL.md", "---\nname: skill-c\ndescription: c\n---\nbody\n")
	before := len(m.entries)

	m.submitJob("noop", func(ctx context.Context) (any, error) { return "done", nil })
	select {
	case r := <-m.queue.Results():
		cmd := m.handleJobDone(r)
		require.Len(t, m.entries, before, "handleJobDone must not scan the filesystem synchronously")
		require.NotNil(t, cmd, "the rescan is deferred to a returned command")

		_, _ = m.Update(cmd())
		require.Greater(t, len(m.entries), before, "the deferred rescan applies once its command runs")
	case <-time.After(3 * time.Second):
		t.Fatal("job did not complete")
	}
}

// TestModelImportSelectsProviderAndKind: `i` collects an address, then a
// provider picker, then a kind picker, before the import runs (FR-037).
func TestModelImportSelectsProviderAndKind(t *testing.T) {
	m := newTestModel(t)
	src := t.TempDir()
	writeFileT(t, src, "SKILL.md", "---\nname: imported\ndescription: imported skill\n---\nbody\n")

	_ = m.handleKey(runeKey('m'))
	require.True(t, m.importing)
	for _, r := range src {
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // finish address -> provider picker
	require.NotNil(t, m.picker)
	require.Contains(t, m.picker.Title, "provider")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // auto provider -> kind picker
	require.NotNil(t, m.picker)
	require.Contains(t, m.picker.Title, "kind")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // auto kind -> run import
	require.Nil(t, m.picker)
	drainJob(t, &m)
	require.Contains(t, m.status, "imported imported")
	require.NotNil(t, m.svc.FindEntry("imported"))
}

func TestModelClaimAndRepairSelectedSkill(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/broken/SKILL.md", "---\nname: broken\n---\nbody\n")
	m.applyScan(m.svc.Scan())
	for i, entry := range m.filtered {
		if entry.Name == "broken" {
			m.cursor = i
			break
		}
	}
	require.Equal(t, "broken", m.filtered[m.cursor].Name)
	require.Equal(t, common.StatusError, m.filtered[m.cursor].Status)

	_ = m.handleKey(runeKey('c'))
	drainJob(t, &m)
	entry := m.svc.FindEntry("broken")
	require.NotNil(t, entry)
	require.Equal(t, common.StatusActive, entry.Status)
	require.Equal(t, filepath.Join(m.svc.Cfg.Root, "skills", "self-build", "broken"), entry.Path)
	require.Contains(t, m.status, "claimed broken")
}

// TestModelDeleteRequiresConfirmation: `d` opens a confirm modal; the entry is
// only deleted after `y` (FR-040).
func TestModelDeleteRequiresConfirmation(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm, "delete opens a confirmation")
	require.NotNil(t, m.svc.FindEntry("skill-a"), "not deleted before confirming")

	_ = m.handleConfirmKey(runeKey('y'))
	require.Nil(t, m.confirm)
	drainJob(t, &m)
	require.Nil(t, m.svc.FindEntry("skill-a"), "deleted after confirming")
}

func TestModelDeleteUsesSelectedSameNamedEntryPath(t *testing.T) {
	m := newTestModel(t)
	root := m.svc.Cfg.Root
	writeFileT(t, root, "skills/unknown/first/one/SKILL.md", "---\nname: duplicate\ndescription: first\n---\nbody\n")
	writeFileT(t, root, "skills/unknown/second/two/SKILL.md", "---\nname: duplicate\ndescription: second\n---\nbody\n")
	m.applyScan(m.svc.Scan())
	for i, entry := range m.filtered {
		if entry.Path == filepath.Join(root, "skills", "unknown", "second", "two") {
			m.cursor = i
			break
		}
	}

	m.deleteSelected()
	require.NotNil(t, m.confirm)
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)

	require.FileExists(t, filepath.Join(root, "skills", "unknown", "first", "one", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, "skills", "unknown", "second", "two"))
}

func TestModelArchiveUsesSelectedSameNamedEntryPath(t *testing.T) {
	m := newTestModel(t)
	root := m.svc.Cfg.Root
	first := filepath.Join(root, "skills", "unknown", "first", "one")
	second := filepath.Join(root, "skills", "unknown", "second", "two")
	writeFileT(t, first, "SKILL.md", "---\nname: duplicate\ndescription: first\n---\nbody\n")
	writeFileT(t, second, "SKILL.md", "---\nname: duplicate\ndescription: second\n---\nbody\n")
	m.applyScan(m.svc.Scan())
	for i, entry := range m.filtered {
		if entry.Path == second {
			m.cursor = i
			break
		}
	}

	m.archiveSelected()
	require.NotNil(t, m.confirm)
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)

	require.DirExists(t, first, "the other same-named entry stays active")
	require.NoDirExists(t, second)
	require.DirExists(t, filepath.Join(root, "archived", "unknown", "second", "two"))
}

// TestModelDeleteCancelKeepsEntry: `n` dismisses the confirm without deleting.
func TestModelDeleteCancelKeepsEntry(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm)
	_ = m.handleConfirmKey(runeKey('n'))
	require.Nil(t, m.confirm)
	require.NotNil(t, m.svc.FindEntry("skill-a"), "entry kept when declined")
}

// TestModelTaskCenterOpens: `J` opens the task center listing completed jobs and
// `x` clears them.
func TestModelTaskCenterOpens(t *testing.T) {
	m := newTestModel(t)
	// Run one job so there is completed history.
	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)

	_ = m.handleKey(runeKey('J'))
	require.True(t, m.showTasks)
	require.Contains(t, m.View(), "install skill-a")

	_ = m.handleKey(runeKey('x')) // clear completed
	require.Empty(t, m.queue.Snapshot().Completed)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.showTasks)
}

// TestModelDiscoverAdopts: `o` lists an external skill; confirming the picker
// adopts it into the repo and replaces the directory with a symlink (FR-038).
func TestModelDiscoverAdopts(t *testing.T) {
	m := newTestModel(t)
	// Plant a real, unmanaged external skill in the target.
	tgt := m.svc.Cfg.Targets[0].Path
	ext := filepath.Join(tgt, "ext-skill")
	writeFileT(t, ext, "SKILL.md", "---\nname: ext-skill\ndescription: external\n---\nbody\n")

	_ = m.handleKey(runeKey('o'))
	require.NotNil(t, m.picker, "discover opens a selection modal")
	m.picker.Items[0].Checked = true
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // adopt selected
	require.Nil(t, m.picker)
	drainJob(t, &m)

	require.NotNil(t, m.svc.FindEntry("ext-skill"), "external skill adopted into repo")
	fi, err := os.Lstat(ext)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "external dir replaced by a symlink")
}

func TestModelDiscoverAdoptsEachSelectionAsAnIndependentJob(t *testing.T) {
	m := newTestModel(t)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	first := filepath.Join(target.Path, "first")
	second := filepath.Join(target.Path, "second")
	writeFileT(t, first, "SKILL.md", "---\nname: duplicate\ndescription: first\n---\nbody\n")
	writeFileT(t, second, "SKILL.md", "---\nname: duplicate\ndescription: second\n---\nbody\n")

	_ = m.handleKey(runeKey('o'))
	require.NotNil(t, m.picker)
	for i := range m.picker.Items {
		m.picker.Items[i].Checked = true
	}
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})

	drainJob(t, &m)
	drainJob(t, &m)
	completed := m.queue.Snapshot().Completed
	require.Len(t, completed, 2)
	for _, job := range completed {
		require.Equal(t, jobs.JobDone, job.State)
		require.Contains(t, job.Name, "adopt ")
	}
	require.FileExists(t, filepath.Join(m.svc.Cfg.Root, "skills", "unknown", "t", "first", "SKILL.md"))
	require.FileExists(t, filepath.Join(m.svc.Cfg.Root, "skills", "unknown", "t", "second", "SKILL.md"))
}

func TestModelDiscoverContinuesAfterOneAdoptFails(t *testing.T) {
	m := newTestModel(t)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	first := filepath.Join(target.Path, "first")
	second := filepath.Join(target.Path, "second")
	writeFileT(t, first, "SKILL.md", "---\nname: first\ndescription: first external\n---\nbody\n")
	writeFileT(t, second, "SKILL.md", "---\nname: second\ndescription: second external\n---\nbody\n")
	// The occupied first destination must not block the second job.
	writeFileT(t, m.svc.Cfg.Root, "skills/unknown/t/first/SKILL.md", "---\nname: existing\ndescription: occupied\n---\nbody\n")

	_ = m.handleKey(runeKey('o'))
	require.NotNil(t, m.picker)
	for i := range m.picker.Items {
		m.picker.Items[i].Checked = true
	}
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	drainJob(t, &m)
	drainJob(t, &m)

	completed := m.queue.Snapshot().Completed
	require.Len(t, completed, 2)
	states := map[jobs.JobState]int{}
	for _, job := range completed {
		states[job.State]++
	}
	require.Equal(t, 1, states[jobs.JobFailed])
	require.Equal(t, 1, states[jobs.JobDone])
	require.FileExists(t, filepath.Join(m.svc.Cfg.Root, "skills", "unknown", "t", "second", "SKILL.md"))
	_, err := os.Lstat(second)
	require.NoError(t, err)
	require.True(t, dal.IsSymlink(second), "the later selected skill is adopted despite the earlier failure")
}

// TestHelpViewRowsFitFrameWidth: renderMainArea's showHelp branch used to
// return SplitLines(help) straight through PadLines, which only appends
// blank filler rows to reach the row *count* — it never truncates/pads each
// existing line to the frame's inner *width* the way every other row in the
// app does (list rows, detail rows, and the header/status/hint bars all go
// through components.FitCell). The long "Install status: …" legend line
// overflows past the right border, and the (shorter) key-binding table rows
// fall short of it — both misalign "│" on the help screen
// ("help 界面的窗口边框不对齐").
func TestHelpViewRowsFitFrameWidth(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.help.Width = m.width - 4
	m.showHelp = true

	inner := m.width - 2
	for i, line := range strings.Split(m.View(), "\n") {
		if line == "" {
			continue
		}
		require.Equal(t, inner+2, lipgloss.Width(line), "line %d must span the full frame width (border-to-border): %q", i, line)
	}
}

// TestHelpLegendWordWrapsInsteadOfTruncating: at the fixture's default
// 100-column width the "Install status: …" legend line (118 cells) and the
// "Target columns: …" line both run past `inner`. FitCell used to truncate
// whatever didn't fit, cutting the text off mid-word ("blank not…") instead
// of showing all of it — border alignment was fine (TestHelpViewRowsFitFrameWidth),
// the content itself was just missing. Both legends now word-wrap onto
// additional lines instead.
func TestHelpLegendWordWrapsInsteadOfTruncating(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.help.Width = m.width - 4
	m.showHelp = true

	v := m.View()
	require.Contains(t, v, "Install status: ✓ installed")
	require.Contains(t, v, "dangling link (source missing)")
	require.Contains(t, v, "conflict (not managed)")
	// The word wrap breaks the line here, so "blank not" and what follows land
	// on separate rendered lines — checked separately rather than as one
	// contiguous substring. The regression this guards against is the text
	// being cut off entirely (truncated to "blank not…" and lost), not where
	// the wrap happens to fall.
	require.Contains(t, v, "blank not")
	require.Contains(t, v, "installed or unsupported",
		"the legend's tail must not be cut off mid-word at a frame width narrower than the line itself")
	require.Contains(t, v, "Target columns: Claude Claude* (Commands) Codex Pi")

	inner := m.width - 2
	for i, line := range strings.Split(v, "\n") {
		if line == "" {
			continue
		}
		require.Equal(t, inner+2, lipgloss.Width(line), "line %d must still span the full frame width: %q", i, line)
	}
}

// TestModelHelpToggle: `?` toggles the full help table, which shows keys that
// are absent from the compact bar (e.g. "batch update").
func TestModelHelpToggle(t *testing.T) {
	for _, closeKey := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'?'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
	} {
		t.Run(closeKey.String(), func(t *testing.T) {
			m := newTestModel(t)
			m.width = 140
			require.False(t, m.showHelp)

			_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
			require.True(t, m.showHelp)
			require.Contains(t, m.View(), "batch update")
			require.Contains(t, m.View(), "Install status: ✓ installed")
			require.Contains(t, m.View(), "dangling link (source missing)")
			require.Contains(t, m.View(), "conflict (not managed)")
			require.Contains(t, m.View(), "blank not installed or unsupported")
			require.Contains(t, m.View(), "Target columns: Claude Claude* (Commands) Codex Pi")

			cmd := m.handleKey(closeKey)
			require.Nil(t, cmd, "closing help must not quit the TUI")
			require.False(t, m.showHelp)
			require.NotContains(t, m.View(), "batch update")
		})
	}
}

// TestModelDetailOpensForSelectedEntry: Enter/v builds the detail page for the
// currently selected entry (lazily, on open).
func TestModelDetailOpensForSelectedEntry(t *testing.T) {
	m := newTestModel(t)

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // move to skill-b
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	require.True(t, m.showDetail)
	require.Contains(t, m.detail, "skill-b")
	require.NotContains(t, m.detail, "skill-a")
}

// runeKey builds a KeyMsg for a single printable key (j/k/h/l/g/G).
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// requireCursorVisible asserts the selected entry's display row falls inside the
// page window that renderMainArea actually paints — the invariant that keeps the
// cursor from vanishing across a page boundary, a page jump, or a resize.
func requireCursorVisible(t *testing.T, m model) {
	t.Helper()
	pg := pagination.Page(len(m.rows), m.pageSize, m.page)
	dr := m.entryRow[m.cursor]
	require.GreaterOrEqual(t, dr, pg.Offset, "cursor above the visible page")
	require.Less(t, dr, pg.Offset+pg.Count, "cursor below the visible page")
}

// TestPaginationCursorTracksPageAcrossMovesAndResize pins the fix for the
// pagination/resize desync: page is derived from the cursor's display row, so
// the highlighted row stays on the rendered page after every move and after a
// resize that changes pageSize.
func TestPaginationCursorTracksPageAcrossMovesAndResize(t *testing.T) {
	entries := make([]*common.Entry, 50)
	for i := range entries {
		entries[i] = &common.Entry{Name: fmt.Sprintf("e%02d", i)}
	}
	m := model{entries: entries, pageSize: 10, keys: components.DefaultKeys(), help: help.New()}
	m.refreshFiltered()

	// Walk down across page boundaries; the cursor stays on the rendered page.
	for i := 0; i < 25; i++ {
		_ = m.handleKey(runeKey('j'))
		requireCursorVisible(t, m)
	}
	require.Equal(t, 25, m.cursor)

	// A page jump advances the page and keeps the cursor visible.
	before := m.page
	_ = m.handleKey(runeKey('l'))
	require.Greater(t, m.page, before, "PageNext advances a page")
	requireCursorVisible(t, m)

	// G jumps to the last entry; it must still be on the visible page.
	_ = m.handleKey(runeKey('G'))
	require.Equal(t, 49, m.cursor)
	requireCursorVisible(t, m)

	// Shrinking the terminal shrinks pageSize; the selection stays visible.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12}) // pageSize -> 4
	m = *updated.(*model)
	require.Equal(t, 4, m.pageSize)
	require.Equal(t, 49, m.cursor, "resize keeps the selection")
	requireCursorVisible(t, m)
}

// TestListViewNeverExceedsTerminalHeight pins an off-by-one where pageSize
// (WindowSizeMsg's `m.height-7`) allowed one more row than listView's actual
// render budget (`h-8`, one more fixed line — the install-target header —
// than the pageSize comment accounted for). A fully-packed page (Count ==
// pageSize) then emitted height+1 lines, which in the alt-screen buffer
// scrolled the frame's own top border out of view — "the first row eaten",
// most visible on the "All" tab since it has the most entries and so is
// likeliest to fill a page exactly.
func TestListViewNeverExceedsTerminalHeight(t *testing.T) {
	m := newTestModel(t)
	entries := make([]*common.Entry, 40)
	for i := range entries {
		mid := "local"
		entries[i] = &common.Entry{Name: fmt.Sprintf("e%02d", i), Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &mid}
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 35})
	m = *updated.(*model)
	m.applyScan(entries)
	require.Equal(t, m.pageSize, pagination.Page(len(m.rows), m.pageSize, m.page).Count, "page is fully packed")

	lines := strings.Split(m.View(), "\n")
	require.LessOrEqual(t, len(lines), 35, "the rendered frame must never exceed the terminal height")
	require.Contains(t, lines[0], "┌─", "the frame's top border must stay the first line")
}

// TestListIsFlatWithoutSectionHeaders: the list is a single flat list — no
// per-source section headers (provider context comes from the tabs, the status
// line, and the detail page). Entries stay sorted by source.
func TestListIsFlatWithoutSectionHeaders(t *testing.T) {
	mk := func(name, mid, grp string) *common.Entry {
		e := &common.Entry{Name: name, Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &mid}
		if grp != "" {
			e.Group = &grp
		}
		return e
	}
	m := newTestModel(t)
	m.entries = []*common.Entry{
		mk("effect", "github", "mattpocock-skills"),
		mk("pdf", "github", "anthropics-skills"),
		mk("mine", "local", ""),
	}
	m.refreshFiltered()

	view := m.View()
	require.Contains(t, view, "pdf")
	require.Contains(t, view, "effect")
	require.Contains(t, view, "mine")
	require.NotContains(t, view, "▸", "no section-header rows divide the list (the status line still shows the cursor's source)")
	// Sorted by source: anthropics < mattpocock (both github) < local block.
	require.Less(t, strings.Index(view, "pdf"), strings.Index(view, "effect"))
	require.Less(t, strings.Index(view, "effect"), strings.Index(view, "mine"))
}

// TestStatusAutoHidesAfterThreeSeconds: a status message left untouched (no
// keypress at all — the case handleKey's clear-on-any-key can't reach) must
// still disappear on its own once statusAutoHide has elapsed, not sit stale
// forever. statusSetAt is backdated directly rather than sleeping 3s in the
// test; the real trigger is the recurring statusTickMsg (statusTickCmd,
// armed once in Init and self-perpetuating).
func TestStatusAutoHidesAfterThreeSeconds(t *testing.T) {
	m := newTestModel(t)
	m.setStatus("archived shown")
	require.Equal(t, "archived shown", m.status)

	// Not yet statusAutoHide: a tick must leave it alone.
	m.statusSetAt = time.Now().Add(-1 * time.Second)
	_, _ = m.Update(statusTickMsg{})
	require.Equal(t, "archived shown", m.status, "status must survive until statusAutoHide has elapsed")

	// statusAutoHide elapsed: the next tick clears it.
	m.statusSetAt = time.Now().Add(-(statusAutoHide + time.Second))
	_, _ = m.Update(statusTickMsg{})
	require.Empty(t, m.status, "status auto-hides once statusAutoHide has elapsed")
}

// TestStatusMessageClearsOnNavigation: a transient notification like
// "archived shown"/"archived hidden" must not stick in the status bar
// forever — the next navigation should reveal the current selection summary
// again, not leave a stale message behind (list.go statusContent: "case
// m.status != \"\": return m.status" never gets cleared by cursor movement).
func TestStatusMessageClearsOnNavigation(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('.'))
	require.Equal(t, "archived shown", m.status)

	_ = m.handleKey(runeKey('j'))
	require.Empty(t, m.status, "moving the cursor clears the stale notification")
	require.Contains(t, m.statusContent(), m.filtered[m.cursor].Name, "status bar reverts to the selection summary")
}

// TestStatusMessageClearsOnAnyKeyNotJustCursorMovement: clearing only on
// j/k/h/l/g/G left the message stuck for any other next action — opening
// detail, switching a provider tab, opening the task center — which is what
// a real user does at least as often as pressing j/k right after toggling
// archived. Every keypress must drop a stale notification, not just
// navigation (handleKey now clears m.status once, up front, for every mode).
func TestStatusMessageClearsOnAnyKeyNotJustCursorMovement(t *testing.T) {
	t.Run("opening detail", func(t *testing.T) {
		m := newTestModel(t)
		_ = m.handleKey(runeKey('.'))
		require.Equal(t, "archived shown", m.status)
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		require.True(t, m.showDetail)
		require.Empty(t, m.status, "opening detail clears the stale notification")
	})

	t.Run("switching provider tab", func(t *testing.T) {
		m := newTestModel(t)
		_ = m.handleKey(runeKey('.'))
		require.Equal(t, "archived shown", m.status)
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
		require.Empty(t, m.status, "switching tabs clears the stale notification")
	})

	t.Run("opening task center", func(t *testing.T) {
		m := newTestModel(t)
		_ = m.handleKey(runeKey('.'))
		require.Equal(t, "archived shown", m.status)
		_ = m.handleKey(runeKey('J'))
		require.True(t, m.showTasks)
		require.Empty(t, m.status, "opening the task center clears the stale notification")
	})
}

// TestNarrowTerminalHidesInstallStatusColumns: on a narrow terminal the
// per-target install-status columns are hidden so name/kind/status stay
// legible instead of being squeezed; they return once there's room (req: "如
// 果窗口特别小，可以隐藏次重要的栏位（安装状态栏）").
func TestNarrowTerminalHidesInstallStatusColumns(t *testing.T) {
	m := newTestModel(t)
	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)

	m.width = 60
	require.False(t, m.showInstallColumns())
	require.NotContains(t, m.View(), "✓", "install-status columns are hidden on a narrow terminal")
	require.Contains(t, m.View(), "skill-a", "the entry name stays visible")

	m.width = 100
	require.True(t, m.showInstallColumns())
	require.Contains(t, m.View(), "✓", "install-status columns return once there's room")
}

// TestArchivedHiddenUntilToggled: archived entries are excluded from the list
// until `.` toggles them on, then back off.
func TestArchivedHiddenUntilToggled(t *testing.T) {
	mk := func(name string, st common.Status) *common.Entry {
		mid := "local"
		return &common.Entry{Name: name, Kind: common.KindSkill, Status: st, ProviderID: &mid}
	}
	m := newTestModel(t)
	m.entries = []*common.Entry{
		mk("active-one", common.StatusActive),
		mk("old-one", common.StatusArchived),
	}
	m.refreshFiltered()
	require.Len(t, m.filtered, 1, "archived hidden by default")
	require.Equal(t, "active-one", m.filtered[0].Name)
	require.NotContains(t, m.View(), "old-one")

	// `.` reveals archived entries.
	_ = m.handleKey(runeKey('.'))
	require.True(t, m.showArchived)
	require.Len(t, m.filtered, 2)
	require.Contains(t, m.View(), "old-one")

	// `.` again hides them.
	_ = m.handleKey(runeKey('.'))
	require.False(t, m.showArchived)
	require.Len(t, m.filtered, 1)
}

// bindingMap collects a []pages.HintItem into a keys->enabled map for assertions.
func bindingMap(items []pages.HintItem) map[string]bool {
	out := map[string]bool{}
	for _, b := range items {
		out[b.Keys] = b.Enabled
	}
	return out
}

// TestTasksBindingsDimUnavailableActions: the task center's footer must dim
// cancel/cancel-all/clear-done the same way list/detail already dim
// install/uninstall, instead of always showing them as available (FR-009).
func TestTasksBindingsDimUnavailableActions(t *testing.T) {
	m := newTestModel(t)
	a := bindingMap(m.tasksBindings())
	require.False(t, a["c"], "cancel disabled: no job selected")
	require.False(t, a["C"], "cancel all disabled: nothing running or queued")
	require.False(t, a["x"], "clear done disabled: no completed history")

	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)
	a = bindingMap(m.tasksBindings())
	require.True(t, a["x"], "clear done enabled once a job has completed")
	require.False(t, a["c"], "cancel still disabled: the completed job can't be cancelled")
}

// TestTasksCenterDisabledKeyGivesReason: pressing cancel/cancel-all/clear-done
// with nothing to act on must set a specific reason, never a silent no-op
// (FR-002, contract §1).
func TestTasksCenterDisabledKeyGivesReason(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('J'))
	require.True(t, m.showTasks)

	_ = m.handleKey(runeKey('c'))
	require.Contains(t, m.status, "no job", "cancel with nothing selected gives a reason")

	m.status = ""
	_ = m.handleKey(runeKey('C'))
	require.Contains(t, m.status, "no jobs", "cancel all with nothing running/queued gives a reason")

	m.status = ""
	_ = m.handleKey(runeKey('x'))
	require.Contains(t, m.status, "no completed", "clear done with no history gives a reason")
}

// TestTargetsBindingsDimWhenNoneSelected: the target editor's footer must dim
// edit-path/remove when no target is selected, the same way list/detail dim
// unavailable actions (FR-009).
func TestTargetsBindingsDimWhenNoneSelected(t *testing.T) {
	m := newTestModel(t)
	require.NotEmpty(t, m.svc.Cfg.Targets, "fixture always has at least the merged built-in targets")
	a := bindingMap(m.targetsBindings())
	require.True(t, a["enter"], "edit path enabled: a target is selected")
	require.True(t, a["d"], "remove enabled: a target is selected")

	m.targetsCursor = len(m.svc.Cfg.Targets) // push past the end: nothing selected
	a = bindingMap(m.targetsBindings())
	require.False(t, a["enter"], "edit path disabled when nothing is selected")
	require.False(t, a["d"], "remove disabled when nothing is selected")
}

// TestTargetsEditRemoveDisabledKeyGivesReason: pressing edit-path/remove with
// no target selected must set a specific reason, never a silent no-op.
func TestTargetsEditRemoveDisabledKeyGivesReason(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	require.True(t, m.showTargets)
	m.targetsCursor = len(m.svc.Cfg.Targets)

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Contains(t, m.status, "no target selected", "edit path with nothing selected gives a reason")
	require.Nil(t, m.targetWizard, "no wizard opens when nothing is selected")

	_ = m.handleKey(runeKey('d'))
	require.Contains(t, m.status, "no target selected", "remove with nothing selected gives a reason")
	require.Nil(t, m.confirm, "no confirm opens when nothing is selected")
}

// TestInstallReasonPrecedenceWhenTwoUnavailableConditionsApply: an entry that
// is both archived and has no matching targets must show the archived reason
// specifically — installSelected checks status before targets, so the two
// conditions can never race (spec.md Edge Case 5, verified by construction,
// not a new behavior).
func TestInstallReasonPrecedenceWhenTwoUnavailableConditionsApply(t *testing.T) {
	m := newTestModel(t)
	_, err := m.svc.Archive(m.ctx, "skill-a", services.LifecycleOptions{})
	require.NoError(t, err)
	m.showArchived = true // archived entries are hidden by default; make skill-a visible
	m.applyScan(m.svc.Scan())
	idx := -1
	for i, e := range m.filtered {
		if e.Name == "skill-a" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0, "archived skill-a is visible once showArchived is set")
	m.cursor = idx

	m.installSelected()
	require.Contains(t, m.status, "archived", "the archived-status reason wins regardless of target availability")
}

// TestJobFailureVisibleRegardlessOfActiveScreen: a background job's failure
// used to only be visible on the list screen (statusContent() was the only
// caller of model.status); a user who opened the detail page, task center, or
// target editor before the job finished had no way to see it without
// navigating back. framedPage() and detailView() now render model.status
// directly, so the failure is visible on whichever screen is active when the
// job completes (FR-003, FR-004, contract §2).
func TestJobFailureVisibleRegardlessOfActiveScreen(t *testing.T) {
	screens := []struct {
		name  string
		open  rune
		check func(t *testing.T, m model)
	}{
		{"task center", 'J', func(t *testing.T, m model) { require.True(t, m.showTasks) }},
		{"target editor", 't', func(t *testing.T, m model) { require.True(t, m.showTargets) }},
	}
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			m := newTestModel(t)
			_ = m.handleKey(runeKey(sc.open))
			sc.check(t, m)
			m.submitJob("boom job", func(ctx context.Context) (any, error) { return nil, errors.New("boom") })
			drainJob(t, &m)
			require.Contains(t, m.status, "boom", "job failure sets model.status regardless of active screen")
			require.Contains(t, m.View(), "boom", "the active screen's View() renders the failure")
		})
	}

	t.Run("detail", func(t *testing.T) {
		m := newTestModel(t)
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		require.True(t, m.showDetail)
		m.submitJob("boom job", func(ctx context.Context) (any, error) { return nil, errors.New("boom") })
		drainJob(t, &m)
		require.Contains(t, m.status, "boom")
		require.Contains(t, m.View(), "boom", "the detail page also renders the failure, not just list")
	})
}

// TestBatchUpdateSurfacesFailureReason: the TUI batch-update failure status
// must name the failing entry and why, not just an aggregate count (FR-005) —
// previously services.BatchUpdateResult.Failed discarded the reason before it
// ever reached the TUI. The batch now runs as one job per entry behind a
// confirmation, and a failing per-entry job's error carries the entry name.
func TestBatchUpdateSurfacesFailureReason(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/broken/SKILL.md", "---\nname: broken\ndescription: broken\n---\nbody\n")
	writeFileT(t, m.svc.Cfg.Root, "skills/local/broken/meta.json", `{"address":"/does/not/exist/anywhere","mode_id":"local"}`)
	m.applyScan(m.svc.Scan())

	m.batchUpdate()
	require.NotNil(t, m.confirm, "batch update asks for confirmation before running")
	m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.Contains(t, m.status, "broken", "the failing entry's name is reachable, not just the count")
	require.Contains(t, m.status, "failed", "a reason is reachable alongside the name")
}

// TestBatchUpdateNoCandidatesGivesFeedback: pressing P with nothing updatable
// in the current tab must say so instead of opening a meaningless
// confirmation.
func TestBatchUpdateNoCandidatesGivesFeedback(t *testing.T) {
	m := newTestModel(t) // fixture entries have no origin
	m.batchUpdate()
	require.Nil(t, m.confirm, "no confirmation when nothing to update")
	require.Contains(t, m.status, "nothing to update")
}

// TestBatchUpdateConfirmsBeforeRunning: P opens a confirmation instead of
// running immediately; declining queues nothing.
func TestBatchUpdateConfirmsBeforeRunning(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/skill-a/meta.json", `{"address":"/fa","mode_id":"local"}`)
	m.applyScan(m.svc.Scan())

	m.batchUpdate()
	require.NotNil(t, m.confirm, "batch update asks for confirmation first")
	require.Empty(t, m.queue.Snapshot().Pending, "no jobs queued before confirmation")

	m.handleConfirmKey(runeKey('n'))
	require.Nil(t, m.confirm)
	require.Empty(t, m.queue.Snapshot().Pending, "declining queues nothing")
	require.Empty(t, m.queue.Snapshot().Completed, "declining runs nothing")
}

// TestBatchUpdateSplitsIntoPerEntryJobs: confirming a batch update queues one
// job per active-with-origin entry, so each update is observable in the task
// center and status bar instead of being hidden inside one monolithic
// batch-update job.
func TestBatchUpdateSplitsIntoPerEntryJobs(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/skill-a/meta.json", `{"address":"/fa","mode_id":"local"}`)
	writeFileT(t, m.svc.Cfg.Root, "skills/local/skill-b/meta.json", `{"address":"/fb","mode_id":"local"}`)
	m.applyScan(m.svc.Scan())

	m.batchUpdate()
	m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	drainJob(t, &m) // would time out if fewer than two jobs were queued
	done := m.queue.Snapshot().Completed
	require.Len(t, done, 2, "one job per candidate entry")
	require.Equal(t, "update skill-a", done[0].Name)
	require.Equal(t, "update skill-b", done[1].Name)
}

// TestBatchUpdateScopesToCurrentTab: batch update refreshes only the active
// entries in the current provider tab, not every entry with an origin. Entries
// live under skills/<provider>/<name>/, so the provider directory (not
// meta.json) is what defines the tab.
func TestBatchUpdateScopesToCurrentTab(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/github/gh-skill/SKILL.md", "---\nname: gh-skill\ndescription: gh\n---\nbody\n")
	writeFileT(t, m.svc.Cfg.Root, "skills/github/gh-skill/meta.json", `{"address":"/fa","mode_id":"github"}`)
	writeFileT(t, m.svc.Cfg.Root, "skills/gitlab/gl-skill/SKILL.md", "---\nname: gl-skill\ndescription: gl\n---\nbody\n")
	writeFileT(t, m.svc.Cfg.Root, "skills/gitlab/gl-skill/meta.json", `{"address":"/fb","mode_id":"gitlab"}`)
	m.applyScan(m.svc.Scan())
	githubTab := 0
	for i, tab := range m.providerTabs {
		if tab == "github" {
			githubTab = i
		}
	}
	require.NotEqual(t, 0, githubTab, "fixture has a github provider tab")
	m.jumpToProviderTab(githubTab)

	m.batchUpdate()
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "1 entry", "only the current tab's entries are candidates")
	m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	done := m.queue.Snapshot().Completed
	require.Len(t, done, 1)
	require.Equal(t, "update gh-skill", done[0].Name, "gitlab entry is not updated from the github tab")
}

// TestBatchUpdateResolvesSameNameByPath: two same-named entries in different
// providers are both updated. Per-entry jobs address by path (the entry's
// identity), not bare name — a bare name would resolve both jobs to the first
// match, silently skipping one provider's copy.
func TestBatchUpdateResolvesSameNameByPath(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/github/dup/SKILL.md", "---\nname: dup\ndescription: gh\n---\nbody\n")
	writeFileT(t, m.svc.Cfg.Root, "skills/github/dup/meta.json", `{"address":"/fa","mode_id":"github"}`)
	writeFileT(t, m.svc.Cfg.Root, "skills/gitlab/dup/SKILL.md", "---\nname: dup\ndescription: gl\n---\nbody\n")
	writeFileT(t, m.svc.Cfg.Root, "skills/gitlab/dup/meta.json", `{"address":"/fb","mode_id":"gitlab"}`)
	m.applyScan(m.svc.Scan())

	m.batchUpdate() // default tab is all, so both same-named entries are candidates
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "2 entries")
	m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	drainJob(t, &m)
	done := m.queue.Snapshot().Completed
	require.Len(t, done, 2, "one job per same-named entry, not one shadowing the other")
	require.Equal(t, "update dup", done[0].Name)
	require.Equal(t, "update dup", done[1].Name)
}

// TestStatusBarShowsRunningJob: while a job runs, the status bar names it and
// how many are queued behind it, so a multi-job batch stays visible without
// opening the task center.
func TestStatusBarShowsRunningJob(t *testing.T) {
	m := newTestModel(t)
	release := make(chan struct{})
	block := func(ctx context.Context) (any, error) { <-release; return "done", nil }
	m.submitJob("update skill-a", block)
	m.submitJob("update skill-b", block)

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(m.statusContent(), "update skill-a") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.Contains(t, m.statusContent(), "update skill-a", "status bar names the running job")
	require.Contains(t, m.statusContent(), "1 queued", "status bar counts the jobs still queued")

	close(release)
	drainJob(t, &m)
	drainJob(t, &m)
}

// TestDiscoverSurfacesProviderLoadFailure: a provider plugin that fails to
// load during discovery used to be visible only through the CLI's `provider
// list`/`validate` (providers.Registry.LoadFailures() had zero references
// anywhere in pkg/tui). Running discover (`o`) must now show the specific
// plugin path and reason (FR-003; R3).
func TestDiscoverSurfacesProviderLoadFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeFileT(t, root, "skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: alpha\n---\nbody\n")
	cfgDir := t.TempDir()

	pluginsDir := t.TempDir()
	providersDir := filepath.Join(pluginsDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	// A plugin that replies with non-JSON fails its handshake fast (protocol
	// error), mirroring pkg/services/provider_plugins_test.go's fixture.
	brokenPath := filepath.Join(providersDir, "broken.sh")
	require.NoError(t, os.WriteFile(brokenPath, []byte("#!/bin/sh\necho 'not json'\n"), 0o755))
	t.Setenv("SKM_PLUGINS_DIR", pluginsDir)

	cfg, err := config.Load(root, cfgDir)
	require.NoError(t, err)
	svc, err := services.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	require.NotEmpty(t, svc.Registry.LoadFailures(), "fixture actually produces a load failure")

	m := initialModel(t.Context(), svc)
	m.width, m.height = 100, 30
	m.pageSize = 20
	m.help.Width = m.width
	m.loading = false
	m.applyScan(svc.Scan())

	m.discoverExternal()
	require.Contains(t, m.status, "provider plugin(s) failed to load")
	require.Contains(t, m.status, brokenPath)
}

// TestRunInstallConflictOffersForceRetry: installing into a target that
// already has a same-named non-managed object used to fail forever with no
// way to proceed from the TUI (the underlying Force flag was never wired up,
// see pkg/services/skill_install.go's "use --force" error). The job must
// now surface as a confirm-then-retry offer instead of a dead-end failure,
// and confirming must retry with force and succeed.
// cursorTo moves m.cursor to the filtered entry named name, failing the test
// if it isn't present.
func cursorTo(t *testing.T, m *model, name string) {
	t.Helper()
	for i, e := range m.filtered {
		if e.Name == name {
			m.cursor = i
			return
		}
	}
	t.Fatalf("%s not found in filtered entries", name)
}

// TestFixSelectedRepairsConflictAndDangling: `F` repairs the highlighted
// entry's per-target install problems — both a dangling link (stale, resolves
// elsewhere) and a conflict (a non-managed object already occupies the
// target) are force-overwritten with a fresh managed install — scoped to the
// current entry only. Uninstall is deliberately not used for the dangling
// case: it only ever removes a link that currently resolves to the entry's
// exact path (isManagedLink, uninstall.go), so it silently no-ops on a stale
// link left behind after the entry moved — installSkill's Force path is the
// one place that already treats conflict and dangling identically.
func TestFixSelectedRepairsConflictAndDangling(t *testing.T) {
	m := newTestModel(t)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)

	// skill-a: dangling — a stale symlink at the target resolving elsewhere.
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(target.Path, "skill-a")))
	// skill-b: conflict — a real, non-managed directory occupies the target.
	require.NoError(t, os.MkdirAll(filepath.Join(target.Path, "skill-b"), 0o755))
	m.applyScan(m.svc.Scan())

	require.Equal(t, common.InstallDangling, m.svc.Installer.State(m.svc.FindEntry("skill-a"), target))
	require.Equal(t, common.InstallConflict, m.svc.Installer.State(m.svc.FindEntry("skill-b"), target))

	cursorTo(t, &m, "skill-a")
	m.fixSelected()
	drainJob(t, &m)
	require.NotNil(t, m.confirm, "fix asks for confirmation before touching anything")
	require.Contains(t, m.confirm.Prompt, "replace with managed installs: t")
	_ = m.handleConfirmKey(runeKey('y'))
	require.Nil(t, m.confirm)
	drainJob(t, &m)
	require.Equal(t, common.InstallInstalled, m.svc.Installer.State(m.svc.FindEntry("skill-a"), target), "the dangling link is replaced with a healthy managed install")

	cursorTo(t, &m, "skill-b")
	m.fixSelected()
	drainJob(t, &m)
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "replace with managed installs: t")
	require.NotEmpty(t, m.confirm.Diff, "a real conflicting directory can be reviewed before replacement")
	_ = m.handleConfirmKey(runeKey('d'))
	require.True(t, m.confirm.ShowDiff)
	require.Contains(t, m.View(), "diff")
	_ = m.handleConfirmKey(runeKey('y'))
	require.Nil(t, m.confirm)
	drainJob(t, &m)
	require.Equal(t, common.InstallInstalled, m.svc.Installer.State(m.svc.FindEntry("skill-b"), target), "the conflict is overwritten with a managed install")

	cursorTo(t, &m, "skill-a")
	m.fixSelected()
	drainJob(t, &m)
	require.Contains(t, m.status, "no conflicts or dangling", "a healthy selected entry falls through to an orphan scan before reporting no work")
}

// TestFixSelectedCleansOrphanDanglingWithoutManagedEntry protects the target
// reconciliation path: a stale symlink can outlive the repository entry it
// once pointed at, leaving no selectable managed skill. F must still offer a
// safe repair that removes only the invalid link and leaves user files alone.
func TestFixSelectedCleansOrphanDanglingWithoutManagedEntry(t *testing.T) {
	m := newTestModel(t)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	orphan := filepath.Join(target.Path, "gone-skill")
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing-source"), orphan))
	m.applyScan(m.svc.Scan())

	// The currently selected healthy entry has nothing to repair itself, so F
	// must discover the orphan rather than returning "no managed skills".
	m.fixSelected()
	drainJob(t, &m)
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "orphan dangling")
	require.Contains(t, m.confirm.Prompt, "gone-skill")
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.False(t, dal.PathExists(orphan))
	require.False(t, dal.IsSymlink(orphan), "the dangling symlink is removed, not converted into a user file")
}

func TestRunInstallConflictOffersForceRetry(t *testing.T) {
	m := newTestModel(t)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	require.NoError(t, os.MkdirAll(filepath.Join(target.Path, "skill-a"), 0o755))

	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)

	require.NotNil(t, m.confirm, "a needs-force failure must offer a confirm-then-retry, not a dead end")
	require.Contains(t, m.confirm.Prompt, "install skill-a")
	_ = m.handleConfirmKey(runeKey('y'))
	require.Nil(t, m.confirm, "confirm closes on yes")
	drainJob(t, &m)

	require.Contains(t, m.status, "installed", "confirming the retry must succeed with force")
	entry := m.svc.FindEntry("skill-a")
	require.NotNil(t, entry)
	require.Equal(t, common.InstallInstalled, m.svc.Installer.State(entry, target))
}

// TestRunImportConflictOffersForceRetry: importing a name that already exists
// in the repo used to fail forever with no way to proceed from the TUI (the
// underlying Force flag was never wired up, see
// pkg/services/repository_import.go's "use --force to overwrite" error). The
// job must now surface as a confirm-then-retry offer, and confirming must
// retry with force and succeed.
func TestRunImportConflictOffersForceRetry(t *testing.T) {
	m := newTestModel(t)
	src := t.TempDir()
	writeFileT(t, src, "SKILL.md", "---\nname: skill-a\ndescription: a colliding import\n---\nnew body\n")

	m.runImport(src, "", "auto")
	drainJob(t, &m)

	require.NotNil(t, m.confirm, "a needs-force failure must offer a confirm-then-retry, not a dead end")
	require.Contains(t, m.confirm.Prompt, "import "+src)
	_ = m.handleConfirmKey(runeKey('y'))
	require.Nil(t, m.confirm, "confirm closes on yes")
	drainJob(t, &m)

	require.Contains(t, m.status, "imported", "confirming the retry must succeed with force")
}

// TestNonTTYGuard: a non-terminal file is not treated as interactive.
func TestNonTTYGuard(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = devNull.Close() }()
	require.False(t, isTerminalFile(devNull), "/dev/null is not a TTY")
}
