package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
)

// T045/T046: §1 audit found the TUI surface (target/provider/kind pickers,
// discover adopt/delete, task center, destructive confirms, key columns,
// background queue) already implemented in 001. These are the two genuine
// coverage gaps the audit found — not missing features, missing regression
// tests locking already-working behavior — plus one for the version/status
// row columns (FR-024).

// TestModelTaskCenterCancelAll locks the "C" (cancel all) path, which
// TestModelTaskCenterOpens didn't cover (it only exercised "x", clear-done).
func TestModelTaskCenterCancelAll(t *testing.T) {
	m := newTestModel(t)
	// Submit two jobs without draining, so both are queued/running.
	m.runInstall("install", "skill-a", []string{"t"})
	m.runInstall("install", "skill-b", []string{"t"})

	_ = m.handleKey(runeKey('J'))
	require.True(t, m.showTasks)

	_ = m.handleKey(runeKey('C')) // cancel all
	snap := m.queue.Snapshot()
	require.Empty(t, snap.Pending, "cancel-all clears the pending queue")

	// Drain whatever the running job produces (cancelled or completed) so the
	// queue goroutine doesn't leak past the test.
	drainJob(t, &m)
}

// TestModelDiscoverConfirmDelete locks the "d" (delete after confirm) path in
// the discover picker, which TestModelDiscoverAdopts didn't cover (it only
// exercised adopt/enter).
func TestModelDiscoverConfirmDelete(t *testing.T) {
	m := newTestModel(t)
	tgt := m.svc.Cfg.Targets[0].Path
	ext := filepath.Join(tgt, "ext-skill")
	writeFileT(t, ext, "SKILL.md", "---\nname: ext-skill\ndescription: external\n---\nbody\n")

	_ = m.handleKey(runeKey('o'))
	require.NotNil(t, m.picker)
	m.picker.Items[0].Checked = true
	_ = m.handlePickerKey(runeKey('d')) // request delete
	require.Nil(t, m.picker, "picker closes when delete is requested")
	require.NotNil(t, m.confirm, "delete requires an explicit confirmation (FR-040)")

	_ = m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyEnter})
	drainJob(t, &m)

	_, err := os.Lstat(ext)
	require.True(t, os.IsNotExist(err), "confirmed delete removes the external directory")
	require.Nil(t, m.svc.FindEntry("ext-skill"), "delete does not adopt into the repo")
}

// TestListRowShowsVersionAndStatusColumns locks FR-024's key columns beyond
// the source header (TestListShowsGroupHeaders) and the install column
// (TestModelInstallActionRefreshesStatus): version and status per row.
func TestListRowShowsVersionAndStatusColumns(t *testing.T) {
	mid := "github"
	ver := "1.2.3"
	m := newTestModel(t)
	m.entries = []*common.Entry{
		{Name: "versioned", Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &mid, Version: &ver},
	}
	m.refreshFiltered()

	view := m.View()
	require.Contains(t, view, "versioned")
	require.Contains(t, view, "1.2.3", "version column is shown")
	require.Contains(t, view, "active", "status column is shown")
}

// TestListRowColumnsStayAlignedWithLongContent: an over-long name or status
// must not push later columns out of alignment with adjacent rows — the
// column is truncated (hidden), not left to grow (found via manual testing
// against a real repo with long skill names).
func TestListRowColumnsStayAlignedWithLongContent(t *testing.T) {
	cells := []installCell{{name: "t", state: common.InstallInstalled}}
	short := renderEntryLine(&common.Entry{Name: "short", Kind: common.KindSkill, Status: common.StatusActive}, "", "", cells, false)
	long := renderEntryLine(&common.Entry{
		Name: "a-very-long-skill-name-that-overflows-the-column", Kind: common.KindSkill, Status: common.StatusActive,
	}, "", "", cells, false)

	require.Equal(t, len(short), len(long), "rows stay the same total width regardless of content length")
	// The kind/status/target columns must start at the same byte offset in
	// both rows — i.e. the long name was truncated, not left to overflow.
	nameFieldEnd := iconColWidth + 1 + nameColWidth + 1 // icon column, then name, +1 each for the separating space
	require.Equal(t, short[nameFieldEnd:], long[nameFieldEnd:], "columns after name stay aligned")

	nonStandard := renderEntryLine(&common.Entry{Name: "x", Kind: common.KindSkill, Status: common.StatusNonStandard}, "", "", cells, false)
	require.Contains(t, nonStandard, "non_standard", "the widened status column fits the longest status value in full")
}

func TestInstallTargetHeadersStayCompactAndAligned(t *testing.T) {
	targets := []common.InstallTarget{
		{Name: "claude-skills"},
		{Name: "claude-commands"},
		{Name: "codex"},
		{Name: "pi"},
	}
	header := installHeaderRow(targets, false)
	require.Contains(t, header, "Claude Claude* Codex Pi")

	cells := []installCell{
		{name: "claude-skills", state: common.InstallInstalled},
		{name: "claude-commands", state: common.InstallAbsent},
		{name: "codex", state: common.InstallDangling},
		{name: "pi", state: common.InstallConflict},
	}
	row := renderEntryLine(&common.Entry{Name: "skill", Kind: common.KindSkill, Status: common.StatusActive}, "", "", cells, false)
	require.Equal(t, lipgloss.Width(header), lipgloss.Width(row), "target headers and target cells share a fixed compact layout")
}

// providerTabEntries builds a fixture spanning two providers plus one
// no-provider entry, for the provider-tab-filter tests below.
func providerTabEntries() []*common.Entry {
	github, local := "github", "local"
	return []*common.Entry{
		{Name: "gh-1", Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &github},
		{Name: "gh-2", Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &github},
		{Name: "loc-1", Kind: common.KindSkill, Status: common.StatusActive, ProviderID: &local},
		{Name: "no-provider", Kind: common.KindSkill, Status: common.StatusNonStandard}, // ProviderID nil
	}
}

// TestProviderTabsComputeSortedWithNoneLast: tabs are [All, github, local, none].
func TestProviderTabsComputeSortedWithNoneLast(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	require.Equal(t, []string{tabAll, "github", "local", tabNone}, m.providerTabs)
}

// TestProviderTabCyclingFiltersList: Tab steps through All -> github -> local
// -> none -> All (wrapping), and the list only shows that tab's entries.
func TestProviderTabCyclingFiltersList(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	m.refreshFiltered()
	require.Len(t, m.filtered, 4, "All shows everything")

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "github", m.activeProviderTab())
	names := entryNames(m.filtered)
	require.ElementsMatch(t, []string{"gh-1", "gh-2"}, names)

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "local", m.activeProviderTab())
	require.ElementsMatch(t, []string{"loc-1"}, entryNames(m.filtered))

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, tabNone, m.activeProviderTab())
	require.ElementsMatch(t, []string{"no-provider"}, entryNames(m.filtered))

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // wraps back to All
	require.Equal(t, tabAll, m.activeProviderTab())
	require.Len(t, m.filtered, 4)

	// Shift+Tab cycles backward: from All it wraps to the last tab (none).
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, tabNone, m.activeProviderTab())
}

// TestProviderTabListIsFlatWithoutSectionHeaders: the list is flat everywhere
// — neither the All tab nor a provider-specific tab emits per-source section
// headers; provider context comes from the tabs, the status line, and the
// detail page.
func TestProviderTabListIsFlatWithoutSectionHeaders(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	m.refreshFiltered()
	require.Equal(t, tabAll, m.activeProviderTab())

	headerCount := func() int {
		n := 0
		for _, r := range m.rows {
			if r.header != "" {
				n++
			}
		}
		return n
	}
	require.Equal(t, 0, headerCount(), "the All tab shows a flat list, no section headers")
	require.Len(t, m.rows, len(m.filtered), "every filtered entry is one flat row")

	_ = m.handleKey(runeKey('1')) // jump to the github tab (index 1)
	require.Equal(t, "github", m.activeProviderTab())
	require.Len(t, m.filtered, 2, "the github tab shows only github entries")
	require.Equal(t, 0, headerCount(), "a provider-specific tab is flat too, no section headers")
}

// TestProviderTabBarLabelsAreFirstLetters: the tab bar shows "*" for All and
// each provider's/none's first letter, uppercased.
func TestProviderTabBarLabelsAreFirstLetters(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	m.refreshFiltered()
	m.width, m.height = 100, 30

	bar := m.tabBarContent()
	require.Contains(t, bar, "*", "All is labeled with a wildcard, distinct from any real provider letter")
	require.Contains(t, bar, "G", "github -> G")
	require.Contains(t, bar, "L", "local -> L")
	require.Contains(t, bar, "N", "none -> N")
}

// TestProviderTabResetsToAllWhenActiveTabVanishes: if the active tab's
// entries disappear on the next scan (e.g. archived), computeProviderTabs
// must fall back to All rather than leaving a stale/out-of-range index.
func TestProviderTabResetsToAllWhenActiveTabVanishes(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	m.providerTabIdx = 2 // "local"
	require.Equal(t, "local", m.activeProviderTab())

	m.entries = providerTabEntries()[:2] // only the github entries remain
	m.computeProviderTabs()
	require.Equal(t, tabAll, m.activeProviderTab(), "the vanished tab resets to All, not an out-of-range index")
}

func entryNames(entries []*common.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

// TestProviderTabDigitJump: 0-9 jump directly to a tab index, alongside
// Tab/Shift+Tab cycling. Out-of-range digits are ignored.
func TestProviderTabDigitJump(t *testing.T) {
	m := newTestModel(t)
	m.entries = providerTabEntries()
	m.computeProviderTabs()
	m.refreshFiltered()
	require.Equal(t, []string{tabAll, "github", "local", tabNone}, m.providerTabs)

	_ = m.handleKey(runeKey('2'))
	require.Equal(t, "local", m.activeProviderTab())
	require.ElementsMatch(t, []string{"loc-1"}, entryNames(m.filtered))

	_ = m.handleKey(runeKey('0'))
	require.Equal(t, tabAll, m.activeProviderTab())

	_ = m.handleKey(runeKey('9')) // out of range: ignored, stays on All
	require.Equal(t, tabAll, m.activeProviderTab())

	require.Contains(t, m.tabBarContent(), "2L", "the tab bar shows the digit prefix for discoverability")
}

// newTestModelWithNonStandard builds a fixture with one properly-placed
// skill and one non-standard skill (skills/flat-skill/SKILL.md, missing its
// provider directory), for the detail-page "move to standard location" tests.
func newTestModelWithNonStandard(t *testing.T) model {
	t.Helper()
	m := newTestModel(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/flat-skill/SKILL.md", "---\nname: flat-skill\ndescription: misplaced\n---\nbody\n")
	m.entries = m.svc.Scan()
	m.computeProviderTabs()
	m.refreshFiltered()
	return m
}

// selectEntry moves the cursor to the entry named name.
func selectEntry(t *testing.T, m *model, name string) {
	t.Helper()
	for i, e := range m.filtered {
		if e.Name == name {
			m.cursor = i
			m.clampView()
			return
		}
	}
	t.Fatalf("entry %q not found in filtered list", name)
}

// TestNormalizeFromDetailPageAsksConfirmationThenMoves: "m" from the detail
// page opens a provider picker (choosing where to relocate the entry), then
// previews the destination via a confirm modal; confirming runs the move as
// a background job.
func TestNormalizeFromDetailPageAsksConfirmationThenMoves(t *testing.T) {
	m := newTestModelWithNonStandard(t)
	selectEntry(t, &m, "flat-skill")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	require.True(t, m.showDetail)

	_ = m.handleKey(runeKey('n'))
	require.NotNil(t, m.picker, "normalize offers a choice of provider")
	require.Contains(t, m.picker.Title, "flat-skill")
	require.Equal(t, "local", m.picker.Items[0].Value, "local is offered as the safe default")

	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // choose "local" (the highlighted default)
	require.Nil(t, m.picker)
	require.NotNil(t, m.confirm, "normalize previews the destination behind a confirmation")
	require.Contains(t, m.confirm.Prompt, "skills/local/flat-skill")

	_ = m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyEnter})
	drainJob(t, &m)
	require.Contains(t, m.status, "moved flat-skill")

	moved := m.svc.FindEntry("flat-skill")
	require.NotNil(t, moved)
	require.Equal(t, common.StatusActive, moved.Status, "the entry is now in its standard location")

	// The detail page refreshes after the job: the entry is now standard, so
	// "move" is no longer offered and install is back — the move visibly took
	// effect in the open detail page, not just on disk.
	require.True(t, m.showDetail, "the detail page stays open across the move")
	avail := map[string]bool{}
	for _, b := range m.detailBindings() {
		avail[b.Keys] = b.Enabled
	}
	require.False(t, avail["n"], "move is disabled for the now-standard entry")
}

// TestNormalizeProviderPickerListsExistingProviders: candidate providers
// come from the repository's actual providers, not just the hardcoded
// "local" default ("移动到更合理的 providers").
func TestNormalizeProviderPickerListsExistingProviders(t *testing.T) {
	m := newTestModelWithNonStandard(t)
	writeFileT(t, m.svc.Cfg.Root, "skills/github/gh-skill/SKILL.md", "---\nname: gh-skill\ndescription: from github\n---\nbody\n")
	m.entries = m.svc.Scan()
	m.computeProviderTabs()
	m.refreshFiltered()

	selectEntry(t, &m, "flat-skill")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	_ = m.handleKey(runeKey('n'))
	require.NotNil(t, m.picker)

	var values []string
	for _, it := range m.picker.Items {
		values = append(values, it.Value)
	}
	require.Contains(t, values, "local")
	require.Contains(t, values, "self-build", "self-build is always offered, even when absent from the repo")
	require.Contains(t, values, "github", "an existing provider in the repo is offered too, not just the hardcoded defaults")
	require.Len(t, values, 3, "local/self-build are deduped, not offered twice")
}

// TestNormalizeProviderPickerAlwaysOffersSelfBuild: self-build must be a move
// destination even in a repo that has no self-build provider directory yet —
// it is offered unconditionally, not only when a scan happens to find it.
func TestNormalizeProviderPickerAlwaysOffersSelfBuild(t *testing.T) {
	m := newTestModelWithNonStandard(t)
	m.entries = m.svc.Scan()
	m.computeProviderTabs()
	m.refreshFiltered()

	selectEntry(t, &m, "flat-skill")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	_ = m.handleKey(runeKey('n'))
	require.NotNil(t, m.picker)

	var values []string
	for _, it := range m.picker.Items {
		values = append(values, it.Value)
	}
	require.Contains(t, values, "self-build")
	require.Equal(t, "local", values[0], "local stays the first (safe default) choice")
}

// TestQBacksOutOfPickerAndConfirm: q cancels an open picker/confirm exactly
// like esc — modals are never a place where q quits the app. Only the
// top-level list's q is the app-wide quit (nnn convention).
func TestQBacksOutOfPickerAndConfirm(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('i')) // install opens a target picker
	require.NotNil(t, m.picker)
	cmd := m.handlePickerKey(runeKey('q'))
	require.Nil(t, cmd, "q does not quit from an open picker")
	require.Nil(t, m.picker, "q closes the picker, like esc")

	m2 := newTestModel(t)
	_ = m2.handleKey(runeKey('d')) // delete opens a confirm
	require.NotNil(t, m2.confirm)
	cmd2 := m2.handleConfirmKey(runeKey('q'))
	require.Nil(t, cmd2, "q does not quit from an open confirm")
	require.Nil(t, m2.confirm, "q closes the confirm, like esc")
}

// TestQBacksOutOfTasksAndTargetsViews: the task center and target list are
// sub-views, so q there returns to the main list (same as esc) instead of
// quitting the app — q quits only from the top-level list.
func TestQBacksOutOfTasksAndTargetsViews(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('J')) // open task center
	require.True(t, m.showTasks)
	cmd := m.handleTasksKey(runeKey('q'))
	require.Nil(t, cmd, "q does not quit from the task center")
	require.False(t, m.showTasks, "q returns to the list from the task center")

	_ = m.handleKey(runeKey('t')) // open target list
	require.True(t, m.showTargets)
	cmd2 := m.handleTargetsKey(runeKey('q'))
	require.Nil(t, cmd2, "q does not quit from the target list")
	require.False(t, m.showTargets, "q returns to the list from the target list")
}

// TestDetailPageEscAndQBothGoBackEnterDoesNothing: esc and q both return to
// the list from the detail page (neither quits the whole app while detail is
// open); enter is not a close trigger there (it's reserved — install/
// uninstall/etc. use their own letters).
func TestDetailPageEscAndQBothGoBackEnterDoesNothing(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	require.True(t, m.showDetail)

	cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.showDetail, "enter does not close the detail page")
	require.Nil(t, cmd)

	cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.showDetail, "esc returns to the list")
	require.Nil(t, cmd, "esc does not quit the app")

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // reopen
	require.True(t, m.showDetail)
	cmd = m.handleKey(runeKey('q'))
	require.False(t, m.showDetail, "q also returns to the list")
	require.Nil(t, cmd, "q does not quit the app while detail is open")
}

// TestDetailPageShowsDescriptionAndSectionRules: the description is a clean,
// dedicated line right under the title (previously only visible buried
// inside the raw marker-preview YAML dump), and each section is divided by a
// rule so the page reads as distinct blocks.
func TestDetailPageShowsDescriptionAndSectionRules(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail for skill-a
	require.True(t, m.showDetail)

	lines := strings.Split(m.detail, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Equal(t, "skill-a", lines[0])
	require.Equal(t, "alpha skill", lines[1], "description is the second line, not buried in the marker preview")
	require.Contains(t, m.detail, "provider:", "mode_id is relabeled 'provider' for TUI readability")
	require.Contains(t, m.detail, "───", "sections are visually divided by a rule")
}

// TestDetailViewSpansTerminalWithFooterPinnedAtBottom: the detail page is
// fluid — short content is padded so the frame spans the full terminal height
// and the footer stays pinned at the bottom instead of floating right under a
// short body.
func TestDetailViewSpansTerminalWithFooterPinnedAtBottom(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail for skill-a
	require.True(t, m.showDetail)
	m.detail = "just one line" // simulate short content

	view := m.detailView()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, m.height, "the detail frame spans the full terminal height")
	require.True(t, strings.HasPrefix(lines[0], "┌"), "frame top opens the page")
	require.True(t, strings.HasPrefix(lines[len(lines)-1], "└"), "frame bottom is the very last line")
	require.True(t, strings.HasPrefix(lines[len(lines)-2], "│[j/k] scroll"), "footer sits directly above the bottom, at screen-bottom")
}

// TestDetailViewScrollsWithJK: the detail page is a pager — j/k scroll a
// window through the content, the footer stays pinned, and the offset clamps
// at both ends so it can never scroll past the content.
func TestDetailViewScrollsWithJK(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail for skill-a
	require.True(t, m.showDetail)

	long := make([]string, 100)
	for i := range long {
		long[i] = fmt.Sprintf("line %d", i)
	}
	m.detail = strings.Join(long, "\n")

	require.True(t, strings.HasPrefix(strings.Split(m.detailView(), "\n")[1], "│line 0"), "starts at the top")

	_ = m.handleKey(runeKey('j'))
	require.Equal(t, 1, m.detailOffset)
	require.True(t, strings.HasPrefix(strings.Split(m.detailView(), "\n")[1], "│line 1"), "j scrolls the content window down")

	_ = m.handleKey(runeKey('k'))
	require.Equal(t, 0, m.detailOffset)
	require.True(t, strings.HasPrefix(strings.Split(m.detailView(), "\n")[1], "│line 0"), "k scrolls back up")

	// Clamp at the bottom: scrolling past the last window stops there.
	maxOffset := len(long) - (m.height - 4)
	for i := 0; i < maxOffset+5; i++ {
		_ = m.handleKey(runeKey('j'))
	}
	require.Equal(t, maxOffset, m.detailOffset, "scroll clamps at the bottom of the content")

	view := m.detailView()
	lines := strings.Split(view, "\n")
	require.True(t, strings.HasPrefix(lines[len(lines)-1], "└"), "frame bottom stays pinned while scrolled")
	require.True(t, strings.HasPrefix(lines[len(lines)-2], "│[j/k] scroll"), "footer stays pinned while scrolled")
}

// TestDetailHintDisablesUnavailableActions: the footer dims bindings that
// don't apply to the current entry — install/update/uninstall for a
// non-standard entry (nothing to install/update, nothing installed), move only
// for non-standard entries, scroll only when the content overflows.
func TestDetailHintDisablesUnavailableActions(t *testing.T) {
	m := newTestModelWithNonStandard(t)
	selectEntry(t, &m, "flat-skill")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	require.True(t, m.showDetail)

	enabled := map[string]bool{}
	for _, b := range m.detailBindings() {
		enabled[b.Keys] = b.Enabled
	}
	require.True(t, enabled["n"], "move is offered for a non-standard entry")
	require.False(t, enabled["i"], "install is disabled for a non-standard entry")
	require.False(t, enabled["u"], "uninstall is disabled when nothing is installed")
	require.False(t, enabled["p"], "update is disabled for an entry with no origin")
	require.True(t, enabled["a"], "archive is always offered")
	require.True(t, enabled["d"], "delete is always offered")
	require.True(t, enabled["esc/q"], "back is always offered")
	require.False(t, enabled["j/k"], "scroll is disabled when the short content fits")

	// An active, origin-having entry flips install/update back on; move flips off.
	m2 := newTestModel(t)
	selectEntry(t, &m2, "skill-a")
	_ = m2.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail for the standard skill-a
	require.True(t, m2.showDetail)
	enabled2 := map[string]bool{}
	for _, b := range m2.detailBindings() {
		enabled2[b.Keys] = b.Enabled
	}
	require.True(t, enabled2["i"], "install is offered for an active entry with kind-matching targets")
	require.False(t, enabled2["n"], "move is disabled for an already-standard entry")
	require.False(t, enabled2["u"], "uninstall stays disabled until something is actually installed")
}

// TestDetailPageOffersInstallAndOtherActions: the detail page isn't just a
// read-only view — install/uninstall/update/archive/delete are reachable
// directly from it, matching what the main list already offers, so the user
// doesn't have to back out to act.
func TestDetailPageOffersInstallAndOtherActions(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail for skill-a
	require.True(t, m.showDetail)

	_ = m.handleKey(runeKey('i')) // install, from the detail page
	require.NotNil(t, m.picker, "install opens a target picker even from the detail page")
	require.Contains(t, m.picker.Title, "Installs")
	for i := range m.picker.Items {
		m.picker.Items[i].Checked = true
	}
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	drainJob(t, &m)
	require.Contains(t, m.status, "installed")
	require.True(t, m.showDetail, "the detail page stays open across the action")
}

// TestNormalizeNotApplicableForStandardEntry: "m" on an already-standard
// entry reports why, without opening a confirm modal or moving anything.
func TestNormalizeNotApplicableForStandardEntry(t *testing.T) {
	m := newTestModelWithNonStandard(t)
	selectEntry(t, &m, "skill-a")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // open detail
	require.True(t, m.showDetail)

	_ = m.handleKey(runeKey('n'))
	require.Nil(t, m.confirm, "an already-standard entry has nothing to move")
	require.Contains(t, m.status, "only non-standard entries can be moved")
}

// TestDestructiveConfirmPromptsStateExactConsequence locks the exact wording
// of every destructive-action confirmation (US3/FR-006/FR-007): each must
// name the specific target and, where the action has a side effect beyond its
// literal name, state that side effect explicitly. Before this test, none of
// these four strings had a regression test asserting the exact text
// (/speckit-analyze finding).
func TestDestructiveConfirmPromptsStateExactConsequence(t *testing.T) {
	m := newTestModel(t)

	m.archiveSelected() // skill-a is active, sorts first
	require.Equal(t, `Archive "skill-a"? It will be uninstalled from all targets first.`, m.confirm.Prompt)
	m.confirm = nil

	m.deleteSelected()
	require.Equal(t, `Delete "skill-a" from the repository permanently?`, m.confirm.Prompt)
	m.confirm = nil

	_ = m.handleKey(runeKey('t')) // open target editor
	require.True(t, m.showTargets)
	targetName := m.svc.Cfg.Targets[m.targetsCursor].Name
	_ = m.handleKey(runeKey('d'))
	require.Equal(t, fmt.Sprintf("Remove target %q? Installed assets are left untouched.", targetName), m.confirm.Prompt)
	m.confirm = nil
	m.showTargets = false

	tgt := m.svc.Cfg.Targets[0].Path
	ext := filepath.Join(tgt, "ext-skill")
	writeFileT(t, ext, "SKILL.md", "---\nname: ext-skill\ndescription: external\n---\nbody\n")
	_ = m.handleKey(runeKey('o'))
	require.NotNil(t, m.picker)
	m.picker.Items[0].Checked = true
	_ = m.handlePickerKey(runeKey('d'))
	require.Equal(t, "Delete 1 external skill director(y/ies)? This removes real files.", m.confirm.Prompt)
}

// TestTargetRemoveConfirmHandlesZeroImpact: the remove-target confirm prompt
// ("Installed assets are left untouched") is a fixed string that never claims
// an install count, so removing a target used by zero installed entries is
// already handled correctly by construction — this locks it in (spec.md Edge
// Case 3), not new behavior.
func TestTargetRemoveConfirmHandlesZeroImpact(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	require.True(t, m.showTargets)
	// Built-in targets refuse removal server-side; select the fixture's custom
	// "t" target, which has never had anything installed to it (zero impact).
	idx := -1
	for i, tgt := range m.svc.Cfg.Targets {
		if tgt.Name == "t" {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0, "fixture's custom target is present")
	m.targetsCursor = idx
	name := m.svc.Cfg.Targets[m.targetsCursor].Name

	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm)
	require.Equal(t, fmt.Sprintf("Remove target %q? Installed assets are left untouched.", name), m.confirm.Prompt)

	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.Contains(t, m.status, "removed target")
	for _, tgt := range m.svc.Cfg.Targets {
		require.NotEqual(t, name, tgt.Name, "the zero-impact target is actually gone")
	}
}

// TestDeclineConfirmLeavesAllOtherStateUnchanged: FR-008 requires that
// declining a confirm only clears m.confirm — cursor, search, and the
// filtered list must be unchanged afterward.
func TestDeclineConfirmLeavesAllOtherStateUnchanged(t *testing.T) {
	m := newTestModel(t)
	beforeCursor, beforeSearch, beforeArchived := m.cursor, m.search, m.showArchived
	beforeNames := make([]string, len(m.filtered))
	for i, e := range m.filtered {
		beforeNames[i] = e.Name
	}

	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm)
	_ = m.handleConfirmKey(runeKey('n'))
	require.Nil(t, m.confirm)

	require.Equal(t, beforeCursor, m.cursor)
	require.Equal(t, beforeSearch, m.search)
	require.Equal(t, beforeArchived, m.showArchived)
	require.Len(t, m.filtered, len(beforeNames))
	for i, name := range beforeNames {
		require.Equal(t, name, m.filtered[i].Name)
	}
}

// TestRapidDestructiveKeyPressesNeverDoubleExecute: Bubble Tea processes one
// tea.KeyMsg per Update call, and m.confirm is set synchronously within the
// same Update as the triggering keypress — so a second, rapid press of the
// same destructive key is always routed to handleConfirmKey (where "d" isn't
// bound to yes/no, so it's a no-op), never re-triggering deleteSelected. No
// race window exists for a confirm to be bypassed or duplicated (spec.md Edge
// Case 4), verified by construction, not new behavior.
func TestRapidDestructiveKeyPressesNeverDoubleExecute(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm)
	firstPrompt := m.confirm.Prompt

	_ = m.handleKey(runeKey('d')) // rapid second press before any response
	require.NotNil(t, m.confirm, "confirm is still active, not cleared by the second press")
	require.Equal(t, firstPrompt, m.confirm.Prompt, "the second press did not open a fresh/duplicate confirm")

	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.Nil(t, m.svc.FindEntry("skill-a"), "deleted exactly once")
}

// TestHelpToggleLeavesNavigationStateUnchanged: FR-010 requires that closing
// the help overlay returns the user to the exact state/selection they had
// before opening it. showHelp (tui.go) is a pure boolean flag only checked in
// View() and only reachable from the list screen; this locks in that opening
// and closing it never touches cursor/filtered/screen-flag state (contract
// §5), verified by construction (R6), not new behavior.
func TestHelpToggleLeavesNavigationStateUnchanged(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('j')) // move cursor off zero so a reset would be detectable
	beforeCursor := m.cursor
	beforeNames := make([]string, len(m.filtered))
	for i, e := range m.filtered {
		beforeNames[i] = e.Name
	}

	_ = m.handleKey(runeKey('?'))
	require.True(t, m.showHelp)
	_ = m.handleKey(runeKey('?'))
	require.False(t, m.showHelp)

	require.Equal(t, beforeCursor, m.cursor)
	require.False(t, m.showDetail)
	require.False(t, m.showTasks)
	require.False(t, m.showTargets)
	require.Len(t, m.filtered, len(beforeNames))
	for i, name := range beforeNames {
		require.Equal(t, name, m.filtered[i].Name)
	}
}

// TestTUILabelsMatchCLIVerbNames locks FR-011: where the TUI exposes an
// action that also exists as a CLI verb, the TUI's displayed label must match
// (hyphen/space formatting aside — "batch update" vs "batch-update" are the
// same words, not a different term a user could be confused by). Confirmed no
// discrepancy exists (R6); this guards against a future regression.
func TestTUILabelsMatchCLIVerbNames(t *testing.T) {
	m := newTestModel(t)
	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "-", "")
		return strings.ReplaceAll(s, " ", "")
	}
	cases := []struct {
		tuiLabel string
		cliVerb  string
	}{
		{m.keys.Update.Help().Desc, "update"},
		{m.keys.BatchUpdate.Help().Desc, "batch-update"},
		{m.keys.Archive.Help().Desc, "archive"},
		{m.keys.Delete.Help().Desc, "delete"},
		{m.keys.Import.Help().Desc, "import"},
		{m.keys.Discover.Help().Desc, "discover"},
		{m.keys.Normalize.Help().Desc, "normalize"},
	}
	for _, c := range cases {
		require.Equal(t, normalize(c.cliVerb), normalize(c.tuiLabel),
			"TUI label %q must match CLI verb %q", c.tuiLabel, c.cliVerb)
	}

	require.Equal(t, "installs", m.keys.Install.Help().Desc)
}
