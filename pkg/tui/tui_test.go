package tui

import (
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
	"github.com/alswl/skm/skm/pkg/managers"
	"github.com/alswl/skm/skm/pkg/tui/components"
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
	svc, err := managers.New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	m := initialModel(t.Context(), svc)
	m.width, m.height = 100, 30
	m.pageSize = 20
	m.help.Width = m.width
	m.loading = false
	m.applyEntries(svc.Scan())
	return *m // a value copy for direct model tests; the program runs the pointer
}

// newLoadingTestModel builds a raw initialModel (loading, unscanned) using the
// same fixture repo as newTestModel, for tests that exercise the async-scan
// transition itself rather than a fully-loaded model.
func newLoadingTestModel(t *testing.T) (*model, *managers.Services) {
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
	svc, err := managers.New(cfg, common.NewLogger(false))
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
	// `s` opens the target picker (all targets checked by default, FR-036).
	_ = m.handleKey(runeKey('s'))
	require.NotNil(t, m.picker, "install opens a target picker")
	require.Contains(t, m.picker.title, "install")
	// Confirm the picker to submit the install job.
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.picker, "picker closes on confirm")

	// The install runs on the background queue: drain the result and apply it.
	select {
	case r := <-m.queue.Results():
		m.handleJobDone(r)
	case <-time.After(3 * time.Second):
		t.Fatal("install job did not complete")
	}
	require.Contains(t, m.status, "installed")
	// The managed link must now exist in the target.
	for _, tgt := range m.svc.Installer.Targets(m.filtered[0]) {
		require.Equal(t, common.InstallInstalled, m.svc.Installer.State(m.filtered[0], tgt))
	}
	// After the scan, the install-status column reflects the managed target.
	require.Contains(t, m.View(), "✓", "install column shows the installed icon")
}

// TestListShowsInstallStatusForNonStandardEntries: computeInstallCols evaluates
// every entry (not just active ones), so a non-standard entry's install state
// is visible in the list ("安装状态无法在 list 页面看到" fix).
func TestListShowsInstallStatusForNonStandardEntries(t *testing.T) {
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/flat-skill/SKILL.md", "---\nname: flat-skill\ndescription: misplaced\n---\nbody\n")
	m.applyEntries(m.svc.Scan())
	// One cell per configured target: the fixture's custom "t" target plus
	// the 4 built-ins, always merged in (config.mergeWithBuiltins). Install
	// into the first (built-in, always within the rendered column width)
	// rather than "t", which the view truncates off at this test width.
	require.Len(t, m.installCol["flat-skill"], len(m.svc.Cfg.Targets))
	tIdx := 0
	require.Equal(t, "claude-skills", m.svc.Cfg.Targets[tIdx].Name)
	require.Equal(t, common.InstallAbsent, m.installCol["flat-skill"][tIdx].state, "absent install shows the dash, not silently skipped")

	// Install flat-skill into the skill-accepting target via the installer.
	entry := m.svc.FindEntry("flat-skill")
	require.NotNil(t, entry)
	target := m.svc.Cfg.Targets[tIdx]
	tx := &dal.FileTransaction{}
	_, err := m.svc.Installer.Install(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()

	m.applyEntries(m.svc.Scan())
	require.Equal(t, common.InstallInstalled, m.installCol["flat-skill"][tIdx].state, "a non-standard entry's install state is now visible in the list")
	require.Contains(t, m.View(), "✓", "the install column renders the installed icon")
}

// drainJob waits for one queued job result and applies it.
func drainJob(t *testing.T, m *model) {
	t.Helper()
	select {
	case r := <-m.queue.Results():
		m.handleJobDone(r)
	case <-time.After(3 * time.Second):
		t.Fatal("background job did not complete")
	}
}

// TestModelImportSelectsProviderAndKind: `i` collects an address, then a
// provider picker, then a kind picker, before the import runs (FR-037).
func TestModelImportSelectsProviderAndKind(t *testing.T) {
	m := newTestModel(t)
	src := t.TempDir()
	writeFileT(t, src, "SKILL.md", "---\nname: imported\ndescription: imported skill\n---\nbody\n")

	_ = m.handleKey(runeKey('i'))
	require.True(t, m.importing)
	for _, r := range src {
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // finish address -> provider picker
	require.NotNil(t, m.picker)
	require.Contains(t, m.picker.title, "provider")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // auto provider -> kind picker
	require.NotNil(t, m.picker)
	require.Contains(t, m.picker.title, "kind")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // auto kind -> run import
	require.Nil(t, m.picker)
	drainJob(t, &m)
	require.Contains(t, m.status, "imported imported")
	require.NotNil(t, m.svc.FindEntry("imported"))
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
	m.picker.items[0].checked = true
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // adopt selected
	require.Nil(t, m.picker)
	drainJob(t, &m)

	require.NotNil(t, m.svc.FindEntry("ext-skill"), "external skill adopted into repo")
	fi, err := os.Lstat(ext)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "external dir replaced by a symlink")
}

// TestModelHelpToggle: `?` toggles the full help table, which shows keys that
// are absent from the compact bar (e.g. "batch update").
func TestModelHelpToggle(t *testing.T) {
	m := newTestModel(t)
	require.False(t, m.showHelp)

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.True(t, m.showHelp)
	require.Contains(t, m.View(), "batch update")

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.False(t, m.showHelp)
	require.NotContains(t, m.View(), "batch update")
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
	m.applyEntries(entries)
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

// TestNonTTYGuard: a non-terminal file is not treated as interactive.
func TestNonTTYGuard(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer func() { _ = devNull.Close() }()
	require.False(t, isTerminalFile(devNull), "/dev/null is not a TTY")
}
