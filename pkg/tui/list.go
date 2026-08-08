package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/tui/components"
	"github.com/alswl/skm/skm/pkg/tui/pages"
	"github.com/alswl/skm/skm/pkg/utils/pagination"
)

// handleDetailKey drives the detail page: scroll, back, and the actions also
// reachable from the list page (installs/update/archive/delete are
// shared with handleListKey — list and detail are one page's worth of
// selection state with two renderings, not two independent pages, so these
// actions stay here rather than in pkg/tui/pages).
func (m *model) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.ClearSearch), key.Matches(msg, k.Quit):
		m.showDetail = false // Esc/q back to the list (Enter does not close detail)
	case key.Matches(msg, k.MoveDown):
		lines := components.SplitLines(m.detail)
		maxOffset := maxInt(0, len(lines)-maxInt(1, maxInt(10, m.height)-4))
		m.detailOffset = clampInt(m.detailOffset+1, 0, maxOffset)
	case key.Matches(msg, k.MoveUp):
		m.detailOffset = maxInt(0, m.detailOffset-1)
	case key.Matches(msg, k.Normalize):
		m.normalizeSelected()
	case key.Matches(msg, k.Install):
		m.installSelected()
	case key.Matches(msg, k.Update):
		m.updateSelected()
	case key.Matches(msg, k.Archive):
		m.archiveSelected()
	case key.Matches(msg, k.Delete):
		m.deleteSelected()
	}
	return nil
}

// handleListKey drives the main list: navigation, search, provider tabs, and
// every action also reachable from the detail page (see handleDetailKey).
func (m *model) handleListKey(msg tea.KeyMsg) tea.Cmd {
	k := m.keys
	if m.showHelp {
		switch {
		case key.Matches(msg, k.Help), key.Matches(msg, k.ClearSearch), key.Matches(msg, k.Quit):
			m.showHelp = false
		}
		return nil
	}
	switch {
	case key.Matches(msg, k.Quit):
		return tea.Quit
	case key.Matches(msg, k.Cancel):
		m.cancelRunningTask()
		return nil
	case key.Matches(msg, k.Queue):
		m.showTasks = true
		m.tasksCursor = 0
		return nil
	case key.Matches(msg, k.MoveDown):
		m.cursor++
		m.clampView()
	case key.Matches(msg, k.MoveUp):
		m.cursor--
		m.clampView()
	case key.Matches(msg, k.PagePrev):
		m.moveByRows(-m.pageSize)
	case key.Matches(msg, k.PageNext):
		m.moveByRows(m.pageSize)
	case key.Matches(msg, k.First):
		m.cursor = 0
		m.clampView()
	case key.Matches(msg, k.Last):
		m.cursor = len(m.filtered) - 1
		m.clampView()
	case key.Matches(msg, k.Search):
		m.searching = true
	case key.Matches(msg, k.ShowArchived):
		m.showArchived = !m.showArchived
		m.refreshFiltered()
		if m.showArchived {
			m.status = "archived shown"
		} else {
			m.status = "archived hidden"
		}
	case key.Matches(msg, k.TabNext):
		m.cycleProviderTab(1)
	case key.Matches(msg, k.TabPrev):
		m.cycleProviderTab(-1)
	case isDigitKey(msg):
		m.jumpToProviderTab(int(msg.Runes[0] - '0'))
	case key.Matches(msg, k.ClearSearch):
		m.clearSearch()
	case key.Matches(msg, k.Detail):
		m.openDetail()
	case key.Matches(msg, k.Import):
		m.importing = true
		m.importAddr = ""
	case key.Matches(msg, k.Install):
		m.installSelected()
	case key.Matches(msg, k.Update):
		m.updateSelected()
	case key.Matches(msg, k.BatchUpdate):
		m.batchUpdate()
	case key.Matches(msg, k.Archive):
		m.archiveSelected()
	case key.Matches(msg, k.Delete):
		m.deleteSelected()
	case key.Matches(msg, k.Discover):
		m.discoverExternal()
	case key.Matches(msg, k.Targets):
		m.showTargets = true
		m.targetsCursor = 0
	case key.Matches(msg, k.Help):
		m.showHelp = !m.showHelp
	}
	return nil
}

// View renders the nnn-style framed layout: a full-screen single-column list
// with a bottom status/help bar, or the full-screen detail page when open.
// It is pure — the filesystem I/O behind the detail page happens in Update via
// openDetail (go-tui-guides.md).
func (m *model) View() string {
	switch {
	case m.loading:
		return m.loadingView()
	case m.picker != nil:
		return m.pickerView()
	case m.confirm != nil:
		return m.confirmView()
	case m.showTasks:
		return m.tasksView()
	case m.targetWizard != nil:
		return m.targetWizardView()
	case m.showTargets:
		return m.targetsView()
	case m.showDetail:
		return m.detailView()
	default:
		return m.listView()
	}
}

// listView renders the framed nnn-style list: a box that always spans the
// terminal, with the entry list filling the space between the title border and
// the status bar, and a full-width status bar plus help line at the bottom.
func (m model) listView() string {
	w := maxInt(20, m.width)
	h := maxInt(10, m.height)
	inner := w - 2         // width inside the │ frame sides
	rows := maxInt(1, h-8) // top, tabbar, install-header, [list], status-sep, status, help-sep, help, bottom
	title := " skm · " + m.svc.Repo.Root()

	var sb strings.Builder
	sb.WriteString(components.FrameTop(inner, title) + "\n")
	sb.WriteString("│" + components.FitCell(m.tabBarContent(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString("│" + components.FitCell(installHeaderRow(m.svc.Cfg.Targets), inner, lipgloss.NewStyle()) + "│\n")
	for _, r := range m.renderMainArea(inner, rows) {
		sb.WriteString("│" + r + "│\n")
	}
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(m.statusContent(), inner, components.StyleStatusBar) + "│\n")
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(m.listHint(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString(components.FrameBottom(inner))
	return sb.String()
}

// loadingView renders the same framed box as listView but with the spinner
// centered in the main area, shown while the initial background scan is
// still running (Init/scanCmd) so the terminal never sits blank.
func (m model) loadingView() string {
	w := maxInt(20, m.width)
	h := maxInt(10, m.height)
	inner := w - 2
	rows := maxInt(1, h-2) // top, [content], bottom
	title := " skm · " + m.svc.Repo.Root()
	msg := m.spinner.View() + " scanning skills…"
	mid := rows / 2

	var sb strings.Builder
	sb.WriteString(components.FrameTop(inner, title) + "\n")
	for i := 0; i < rows; i++ {
		if i == mid {
			sb.WriteString("│" + centerCell(msg, inner) + "│\n")
		} else {
			sb.WriteString("│" + components.FitCell("", inner, lipgloss.NewStyle()) + "│\n")
		}
	}
	sb.WriteString(components.FrameBottom(inner))
	return sb.String()
}

// centerCell horizontally centers content within w cells, padding with
// spaces on both sides (falls back to components.FitCell's truncate+pad if content is
// too wide to center).
func centerCell(content string, w int) string {
	cw := lipgloss.Width(content)
	if cw >= w {
		return components.FitCell(content, w, lipgloss.NewStyle())
	}
	left := (w - cw) / 2
	right := w - cw - left
	return strings.Repeat(" ", left) + content + strings.Repeat(" ", right)
}

// tabBarContent renders the provider filter tabs: "All" (as "*", to avoid
// visually colliding with any real provider whose first letter is "A") then
// each provider's first letter, uppercased, then "none"'s first letter if
// any entry has no provider. Each tab is prefixed with its digit index (for
// indices 0-9) so 0/1/2/… direct-jump is discoverable, alongside
// Tab/Shift+Tab cycling. The active tab is highlighted; multiple providers
// sharing a first letter (e.g. "github"/"gitlab") will render the
// same letter — that's an accepted display-only ambiguity (the full mode_id
// is always visible in the section header and status line); cycling and
// digit-jump are never ambiguous, since they act on the tab's index.
func (m model) tabBarContent() string {
	if len(m.providerTabs) <= 1 {
		return "" // nothing to filter by
	}
	var sb strings.Builder
	for i, t := range m.providerTabs {
		label := "*"
		if t != tabAll {
			label = strings.ToUpper(t[:1])
		}
		if i <= 9 {
			label = strconv.Itoa(i) + label
		}
		st := components.StyleDim
		if i == m.providerTabIdx {
			st = components.StyleCursor
		}
		sb.WriteString(st.Render(" " + label + " "))
	}
	return sb.String()
}

// renderMainArea returns the rows between the top and status borders: the
// paged entry list (cursor row highlighted full-width), or the full help table
// while `?` is held. Always padded to exactly `rows` lines so the frame spans
// the terminal height.
func (m model) renderMainArea(inner, rows int) []string {
	if m.showHelp {
		help := "Install status: ✓ installed  ⚠ dangling link (source missing)  ✗ conflict (not managed)  blank not installed or unsupported\n" +
			"Target columns: Claude Claude* (Commands) Codex Pi\n" +
			m.help.FullHelpView(m.keys.FullHelp())
		lines := components.SplitLines(help)
		if len(lines) > rows {
			lines = lines[:rows]
		}
		return components.PadLines(lines, inner, rows)
	}
	pageInfo := pagination.Page(len(m.rows), m.pageSize, m.page)
	var out []string
	for i := pageInfo.Offset; i < pageInfo.Offset+pageInfo.Count; i++ {
		r := m.rows[i]
		if r.header != "" {
			out = append(out, components.FitCell(components.StyleGroup.Render("▸ "+r.header), inner, lipgloss.NewStyle()))
			continue
		}
		e := m.filtered[r.entryIdx]
		hl := r.entryIdx == m.cursor
		row := renderEntryLine(e, m.providerIcon(e.ProviderIDValue()), m.installCol[e.Name], hl)
		if hl {
			out = append(out, components.FitCell("  ▶ "+row, inner, components.StyleCursor))
		} else {
			out = append(out, components.FitCell("    "+row, inner, lipgloss.NewStyle()))
		}
	}
	return components.PadLines(out, inner, rows)
}

// Column widths for renderEntryLine. statusColWidth fits the longest status
// value ("non_standard"); other columns truncate overflow instead of
// growing, so every row stays aligned regardless of content length.
const (
	iconColWidth    = 2 // fits one wide (2-cell) provider icon glyph
	nameColWidth    = 24
	kindColWidth    = 7
	versionColWidth = 8
	statusColWidth  = 12
)

// installCell is one target's install-state column for a single entry: the
// target's name (drives the header label and column width) and its state.
type installCell struct {
	name  string
	state common.InstallState
}

// targetColWidth follows the visible header label, keeping each header and its
// status cells aligned without reserving the much longer configuration name.
func targetColWidth(name string) int {
	return lipgloss.Width(targetLabel(name))
}

// targetLabel is the visible header for a target status column.
// The default targets have stable, unambiguous labels; custom targets fall
// back to their uppercased prefix.
func targetLabel(name string) string {
	switch name {
	case "claude-skills":
		return "Claude"
	case "claude-commands":
		return "Claude*"
	case "codex":
		return "Codex"
	case "pi":
		return "Pi"
	default:
		return ansi.Truncate(name, 7, "")
	}
}

// installHeaderRow labels each per-target column above the entry list
// (FR-041): a plain, blank-padded run under the fixed icon/name/kind/version/
// status columns, then each target's name over its own cell.
func installHeaderRow(targets []common.InstallTarget) string {
	var sb strings.Builder
	sb.WriteString(truncPad("", iconColWidth) + " " + truncPad("", nameColWidth) + " " + truncPad("", kindColWidth) + " " +
		truncPad("", versionColWidth) + " " + truncPad("", statusColWidth))
	for _, t := range targets {
		sb.WriteString(" " + components.StyleDim.Render(truncPad(targetLabel(t.Name), targetColWidth(t.Name))))
	}
	return sb.String()
}

// renderEntryLine is one flat list row: provider icon · name · kind · version
// · status · one column per install target (FR-041), each showing that
// target's install state as a small glyph (components.InstallIcon). icon is the
// provider's declared marker (model.providerIcon; unknownProviderIcon when
// the provider isn't recognized or declares none). Kind, status, and target cells
// are zone-colored (nnn-style) for non-highlighted rows; the highlighted
// (cursor) row is plain so the solid cursor background stays clean. Every
// column, including each target cell, is truncated to a fixed width (not
// just padded), so an over-long name/version/target-name can't misalign the
// columns that follow it.
func renderEntryLine(e *common.Entry, icon string, cells []installCell, highlighted bool) string {
	kind := truncPad(string(e.Kind), kindColWidth)
	status := truncPad(string(e.Status), statusColWidth)
	if !highlighted {
		kind = components.StyleForKind(e.Kind).Render(kind)
		status = components.StyleForStatus(e.Status).Render(status)
	}
	var sb strings.Builder
	sb.WriteString(truncPad(icon, iconColWidth) + " " +
		truncPad(e.Name, nameColWidth) + " " +
		kind + " " +
		truncPad(orDash(e.VersionValue()), versionColWidth) + " " +
		status)
	for _, c := range cells {
		icon, style := components.InstallIcon(c.state)
		cell := truncPad(icon, targetColWidth(c.name))
		if !highlighted {
			cell = style.Render(cell)
		}
		sb.WriteString(" " + cell)
	}
	return sb.String()
}

// truncPad truncates s to at most w cells (ANSI/wide-rune aware) and
// right-pads with spaces to exactly w cells, so a table column stays a fixed
// width — overflow is hidden rather than shifting later columns.
func truncPad(s string, w int) string {
	t := ansi.Truncate(s, w, "")
	return t + strings.Repeat(" ", maxInt(0, w-lipgloss.Width(t)))
}

// statusContent is the bottom status line: the import prompt, the active search
// input, the last action status, or the page + selected-entry summary.
func (m model) statusContent() string {
	switch {
	case m.importing:
		return components.StylePrompt.Render("Import address: ") + m.importAddr + "▏"
	case m.searching:
		return components.StyleTitle.Render("Search: ") + m.search + "▏"
	case m.status != "":
		return m.status
	default:
		return m.selectionSummary()
	}
}

func (m model) selectionSummary() string {
	total := len(m.filtered)
	if total == 0 {
		return "0 entries"
	}
	pages := maxInt(1, pagination.PageCount(len(m.rows), m.pageSize))
	e := m.filtered[m.cursor]
	return fmt.Sprintf("%d/%d · %s · %s · %s · %s",
		m.page+1, pages, sectionHeader(e), e.Name, e.Kind, e.Status)
}

// detailView renders the full-screen detail page for the entry opened with
// Enter/v. m.detail is built lazily in openDetail (Update), never here. The
// page is a pager: j/k scroll a window through the content (the footer stays
// pinned at the bottom), so long entries never lose information.
func (m model) detailView() string {
	w := maxInt(20, m.width)
	h := maxInt(10, m.height)
	inner := w - 2
	rows := maxInt(1, h-6) // top, [content], sep, status, sep, footer, bottom
	title := " skm · detail "
	if len(m.filtered) > 0 {
		title = " skm · " + m.filtered[m.cursor].Name + " · detail "
	}
	lines := components.SplitLines(m.detail)
	offset := clampInt(m.detailOffset, 0, maxInt(0, len(lines)-rows))
	var sb strings.Builder
	sb.WriteString(components.FrameTop(inner, title) + "\n")
	for i := 0; i < rows; i++ {
		l := ""
		if offset+i < len(lines) {
			l = lines[offset+i]
		}
		sb.WriteString("│" + components.FitCell(l, inner, lipgloss.NewStyle()) + "│\n")
	}
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(m.status, inner, components.StyleStatusBar) + "│\n")
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(m.detailHint(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString(components.FrameBottom(inner))
	return sb.String()
}

// listHint builds the list-view footer, dimming install/uninstall the same
// way detailHint does for the highlighted entry, so the bottom bar reads
// consistently whether you're on the list or the detail page instead of only
// disclosing unavailability once you press the key and land on a rejection.
func (m model) listHint() string {
	var parts []string
	for _, b := range m.listBindings() {
		parts = append(parts, pages.HintBinding(b.Keys, b.Label, b.Enabled))
	}
	return strings.Join(parts, "  ")
}

// listBindings is the list footer's availability matrix for the highlighted
// entry. Installs reuse the same conditions as installSelected /
// openInstallsPicker and detailBindings so a dimmed key never surprises with a
// status-bar rejection. Targets() is a pure in-memory lookup (no FS I/O), so
// it's safe here in View; installed/conflict state comes from m.installCol,
// cached in Update (computeInstallCols), keeping View pure.
func (m model) listBindings() []pages.HintItem {
	install := false
	if len(m.filtered) > 0 {
		e := m.filtered[m.cursor]
		install = e.Status == common.StatusActive && len(m.svc.Installer.Targets(e)) > 0
	}
	return []pages.HintItem{
		{Keys: "j/k", Label: "up/down", Enabled: true},
		{Keys: "/", Label: "search", Enabled: true},
		{Keys: "i", Label: "installs", Enabled: install},
		{Keys: "m", Label: "import", Enabled: true},
		{Keys: "q", Label: "quit", Enabled: true},
	}
}

// detailHint builds the detail-page footer, dimming the bindings that do not
// apply to the currently selected entry (e.g. install/update on an entry with
// no origin, move on an already-standard entry), so the usable commands are
// visible at a glance instead of discovered by pressing and getting a status
// rejection. FS-derived availability comes from fields cached in refreshDetail
// (Update); View stays pure. The footer is rendered plain (like the main
// list's help line) so dimmed bindings don't fight a background bar's ANSI
// reset.
func (m model) detailHint() string {
	var parts []string
	for _, b := range m.detailBindings() {
		parts = append(parts, pages.HintBinding(b.Keys, b.Label, b.Enabled))
	}
	return strings.Join(parts, "  ")
}

// detailBindings is the detail footer's availability matrix for the currently
// selected entry (tested directly; detailHint renders it). FS-derived
// availability comes from fields cached in refreshDetail (Update), so View
// stays pure.
func (m model) detailBindings() []pages.HintItem {
	if len(m.filtered) == 0 {
		return []pages.HintItem{{Keys: "esc/q", Label: "back", Enabled: true}}
	}
	e := m.filtered[m.cursor]
	rows := maxInt(1, maxInt(10, m.height)-4)
	return []pages.HintItem{
		{Keys: "j/k", Label: "scroll", Enabled: len(components.SplitLines(m.detail)) > rows},
		{Keys: "esc/q", Label: "back", Enabled: true},
		{Keys: "i", Label: "installs", Enabled: e.Status == common.StatusActive && m.detailTargets > 0},
		{Keys: "p", Label: "update", Enabled: e.Status == common.StatusActive && e.Origin != nil},
		{Keys: "a", Label: "archive", Enabled: true},
		{Keys: "d", Label: "delete", Enabled: true},
		{Keys: "n", Label: "move", Enabled: e.Status == common.StatusNonStandard},
	}
}

// buildDetail renders the detail page: name + description, metadata,
// install status, file tree and marker preview, each section divided by a
// rule so the page reads as distinct blocks rather than one long run of
// lines (tui-contract.md). It reads the filesystem, so it runs in Update
// (via openDetail), never in View.
func (m model) buildDetail() string {
	if len(m.filtered) == 0 {
		return components.StyleDim.Render("no entries")
	}
	e := m.filtered[m.cursor]
	rule := components.StyleDim.Render(strings.Repeat("─", maxInt(20, m.width)-2))

	var sb strings.Builder
	sb.WriteString(components.StyleTitle.Render(e.Name) + "\n")
	if e.Description != "" {
		sb.WriteString(e.Description + "\n")
	}
	sb.WriteString(rule + "\n")

	fmt.Fprintf(&sb, "%-10s %s\n", "kind:", e.Kind)
	fmt.Fprintf(&sb, "%-10s %s\n", "status:", e.Status)
	fmt.Fprintf(&sb, "%-10s %s\n", "provider:", providerLabel(m.providerIcon(e.ProviderIDValue()), e.ProviderIDValue()))
	fmt.Fprintf(&sb, "%-10s %s\n", "group:", orDash(e.GroupValue()))
	fmt.Fprintf(&sb, "%-10s %s\n", "version:", orDash(e.VersionValue()))
	if e.Origin != nil {
		fmt.Fprintf(&sb, "%-10s %s\n", "origin:", e.Origin.Address)
	} else {
		fmt.Fprintf(&sb, "%-10s %s\n", "origin:", "none")
	}
	if e.Error != nil {
		fmt.Fprintf(&sb, "%-10s %s\n", "error:", *e.Error)
	}

	sb.WriteString(rule + "\n")
	sb.WriteString(components.StyleDim.Render("install status") + "\n")
	targets := m.svc.Installer.Targets(e)
	if len(targets) == 0 {
		sb.WriteString("  (no matching targets)\n")
	}
	for _, t := range targets {
		st := m.svc.Installer.State(e, t)
		fmt.Fprintf(&sb, "  %-16s %s\n", t.Name, st)
	}

	sb.WriteString(rule + "\n")
	sb.WriteString(components.StyleDim.Render("files") + "\n")
	for _, f := range entryFiles(e.Path) {
		sb.WriteString("  " + f + "\n")
	}

	sb.WriteString(rule + "\n")
	sb.WriteString(components.StyleDim.Render("marker preview") + "\n")
	sb.WriteString(previewMarker(e))

	return sb.String()
}

// ---- frame helpers (box-drawing) ----
//
// components.FrameTop/components.FrameSep/components.FrameBottom/components.FitCell/components.SplitLines/components.PadLines moved to
// pkg/tui/components (003-engineering-optimization Round 2 R7) — they were
// used across list.go/modal.go/tasks_view.go/target_editor.go/tui.go with no
// model dependency, a genuinely shared primitive, not page-specific.

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// providerLabel renders a provider's mode_id for the detail page, prefixed
// with its icon (model.providerIcon; unknownProviderIcon when unrecognized).
func providerLabel(icon, modeID string) string {
	return icon + " " + orDash(modeID)
}

// entryFiles lists the relative file paths under an entry, sorted.
func entryFiles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
}

// previewMarker shows the entry's marker file content in full; the detail
// page's j/k pager (detailView) scrolls through it, so truncating here would
// hide the tail of the file from view no matter how far the user scrolls.
func previewMarker(e *common.Entry) string {
	data, err := os.ReadFile(e.MarkerPath())
	if err != nil {
		return components.StyleDim.Render("(unreadable)")
	}
	return strings.TrimSuffix(string(data), "\n")
}
