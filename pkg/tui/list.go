package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/pagination"
)

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
	sb.WriteString(frameTop(inner, title) + "\n")
	sb.WriteString("│" + fitCell(m.tabBarContent(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString("│" + fitCell(installHeaderRow(m.svc.Cfg.Targets), inner, lipgloss.NewStyle()) + "│\n")
	for _, r := range m.renderMainArea(inner, rows) {
		sb.WriteString("│" + r + "│\n")
	}
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell(m.statusContent(), inner, styleStatusBar) + "│\n")
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell(m.listHint(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString(frameBottom(inner))
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
	sb.WriteString(frameTop(inner, title) + "\n")
	for i := 0; i < rows; i++ {
		if i == mid {
			sb.WriteString("│" + centerCell(msg, inner) + "│\n")
		} else {
			sb.WriteString("│" + fitCell("", inner, lipgloss.NewStyle()) + "│\n")
		}
	}
	sb.WriteString(frameBottom(inner))
	return sb.String()
}

// centerCell horizontally centers content within w cells, padding with
// spaces on both sides (falls back to fitCell's truncate+pad if content is
// too wide to center).
func centerCell(content string, w int) string {
	cw := lipgloss.Width(content)
	if cw >= w {
		return fitCell(content, w, lipgloss.NewStyle())
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
		st := styleDim
		if i == m.providerTabIdx {
			st = styleCursor
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
		return padLines(splitLines(m.help.FullHelpView(m.keys.FullHelp())), inner, rows)
	}
	pageInfo := pagination.Page(len(m.rows), m.pageSize, m.page)
	var out []string
	for i := pageInfo.Offset; i < pageInfo.Offset+pageInfo.Count; i++ {
		r := m.rows[i]
		if r.header != "" {
			out = append(out, fitCell(styleGroup.Render("▸ "+r.header), inner, lipgloss.NewStyle()))
			continue
		}
		e := m.filtered[r.entryIdx]
		hl := r.entryIdx == m.cursor
		row := renderEntryLine(e, m.providerIcon(e.ModeIDValue()), m.installCol[e.Name], hl)
		if hl {
			out = append(out, fitCell("  ▶ "+row, inner, styleCursor))
		} else {
			out = append(out, fitCell("    "+row, inner, lipgloss.NewStyle()))
		}
	}
	return padLines(out, inner, rows)
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

// installNA marks a target column that structurally cannot receive an
// entry's kind (the target's Accepts doesn't list it) — distinct from
// InstallAbsent, which means the target could receive the entry but doesn't
// have it yet (FR-041 per-target columns).
const installNA common.InstallState = "na"

// installCell is one target's install-state column for a single entry: the
// target's name (drives the header label and column width) and its state.
type installCell struct {
	name  string
	state common.InstallState
}

// targetColWidth is the shared column width for a target's header cell and
// every entry's cell under it, so the header and the rows always line up.
func targetColWidth(name string) int {
	return clampInt(len(name), 3, 16)
}

// installHeaderRow labels each per-target column above the entry list
// (FR-041): a plain, blank-padded run under the fixed icon/name/kind/version/
// status columns, then each target's name over its own cell.
func installHeaderRow(targets []common.InstallTarget) string {
	var sb strings.Builder
	sb.WriteString(truncPad("", iconColWidth) + " " + truncPad("", nameColWidth) + " " + truncPad("", kindColWidth) + " " +
		truncPad("", versionColWidth) + " " + truncPad("", statusColWidth))
	for _, t := range targets {
		sb.WriteString(" " + styleDim.Render(truncPad(t.Name, targetColWidth(t.Name))))
	}
	return sb.String()
}

// renderEntryLine is one flat list row: provider icon · name · kind · version
// · status · one column per install target (FR-041), each showing that
// target's install state as a small glyph (installIcon). icon is the
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
		kind = styleForKind(e.Kind).Render(kind)
		status = styleForStatus(e.Status).Render(status)
	}
	var sb strings.Builder
	sb.WriteString(truncPad(icon, iconColWidth) + " " +
		truncPad(e.Name, nameColWidth) + " " +
		kind + " " +
		truncPad(orDash(e.VersionValue()), versionColWidth) + " " +
		status)
	for _, c := range cells {
		icon, style := installIcon(c.state)
		cell := truncPad(icon, targetColWidth(c.name))
		if !highlighted {
			cell = style.Render(cell)
		}
		sb.WriteString(" " + cell)
	}
	return sb.String()
}

// installedSomewhere reports whether any of an entry's target cells shows a
// real install (installed, or a conflict/dangling problem worth letting the
// user clear) — used to enable/disable the list footer's uninstall key.
func installedSomewhere(cells []installCell) bool {
	for _, c := range cells {
		if c.state == common.InstallInstalled || c.state == common.InstallConflict || c.state == common.InstallDangling {
			return true
		}
	}
	return false
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
		return stylePrompt.Render("Import address: ") + m.importAddr + "▏"
	case m.searching:
		return styleTitle.Render("Search: ") + m.search + "▏"
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
	rows := maxInt(1, h-4) // top, [content], sep, footer, bottom
	title := " skm · detail "
	if len(m.filtered) > 0 {
		title = " skm · " + m.filtered[m.cursor].Name + " · detail "
	}
	lines := splitLines(m.detail)
	offset := clampInt(m.detailOffset, 0, maxInt(0, len(lines)-rows))
	var sb strings.Builder
	sb.WriteString(frameTop(inner, title) + "\n")
	for i := 0; i < rows; i++ {
		l := ""
		if offset+i < len(lines) {
			l = lines[offset+i]
		}
		sb.WriteString("│" + fitCell(l, inner, lipgloss.NewStyle()) + "│\n")
	}
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell(m.detailHint(), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString(frameBottom(inner))
	return sb.String()
}

// listHint builds the list-view footer, dimming install/uninstall the same
// way detailHint does for the highlighted entry, so the bottom bar reads
// consistently whether you're on the list or the detail page instead of only
// disclosing unavailability once you press the key and land on a rejection.
func (m model) listHint() string {
	var parts []string
	for _, b := range m.listBindings() {
		parts = append(parts, hintBinding(b.keys, b.label, b.enabled))
	}
	return strings.Join(parts, "  ")
}

// listBindings is the list footer's availability matrix for the highlighted
// entry. Install/uninstall reuse the same conditions as installSelected /
// openTargetPicker and detailBindings so a dimmed key never surprises with a
// status-bar rejection. Targets() is a pure in-memory lookup (no FS I/O), so
// it's safe here in View; installed/conflict state comes from m.installCol,
// cached in Update (computeInstallCols), keeping View pure.
func (m model) listBindings() []hintItem {
	install, uninstall := false, false
	if len(m.filtered) > 0 {
		e := m.filtered[m.cursor]
		install = e.Status == common.StatusActive && len(m.svc.Installer.Targets(e)) > 0
		uninstall = installedSomewhere(m.installCol[e.Name])
	}
	return []hintItem{
		{keys: "j/k", label: "up/down", enabled: true},
		{keys: "/", label: "search", enabled: true},
		{keys: "s", label: "install", enabled: install},
		{keys: "u", label: "uninstall", enabled: uninstall},
		{keys: "q", label: "quit", enabled: true},
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
		parts = append(parts, hintBinding(b.keys, b.label, b.enabled))
	}
	return strings.Join(parts, "  ")
}

// hintBinding renders a key binding, dimmed when the action is not available
// for the current selection.
func hintBinding(keys, label string, enabled bool) string {
	if !enabled {
		return styleDim.Render("[" + keys + "] " + label)
	}
	return "[" + keys + "] " + label
}

// hintItem is one footer binding and whether it applies to the current entry.
type hintItem struct {
	keys    string
	label   string
	enabled bool
}

// detailBindings is the detail footer's availability matrix for the currently
// selected entry (tested directly; detailHint renders it). FS-derived
// availability comes from fields cached in refreshDetail (Update), so View
// stays pure.
func (m model) detailBindings() []hintItem {
	if len(m.filtered) == 0 {
		return []hintItem{{keys: "esc/q", label: "back", enabled: true}}
	}
	e := m.filtered[m.cursor]
	rows := maxInt(1, maxInt(10, m.height)-4)
	return []hintItem{
		{keys: "j/k", label: "scroll", enabled: len(splitLines(m.detail)) > rows},
		{keys: "esc/q", label: "back", enabled: true},
		{keys: "s", label: "install", enabled: e.Status == common.StatusActive && m.detailTargets > 0},
		{keys: "u", label: "uninstall", enabled: m.detailInstalled},
		{keys: "p", label: "update", enabled: e.Status == common.StatusActive && e.Origin != nil},
		{keys: "a", label: "archive", enabled: true},
		{keys: "d", label: "delete", enabled: true},
		{keys: "m", label: "move", enabled: e.Status == common.StatusNonStandard},
	}
}

// buildDetail renders the detail page: name + description, metadata,
// install status, file tree and marker preview, each section divided by a
// rule so the page reads as distinct blocks rather than one long run of
// lines (tui-contract.md). It reads the filesystem, so it runs in Update
// (via openDetail), never in View.
func (m model) buildDetail() string {
	if len(m.filtered) == 0 {
		return styleDim.Render("no entries")
	}
	e := m.filtered[m.cursor]
	rule := styleDim.Render(strings.Repeat("─", maxInt(20, m.width)-2))

	var sb strings.Builder
	sb.WriteString(styleTitle.Render(e.Name) + "\n")
	if e.Description != "" {
		sb.WriteString(e.Description + "\n")
	}
	sb.WriteString(rule + "\n")

	fmt.Fprintf(&sb, "%-10s %s\n", "kind:", e.Kind)
	fmt.Fprintf(&sb, "%-10s %s\n", "status:", e.Status)
	fmt.Fprintf(&sb, "%-10s %s\n", "provider:", providerLabel(m.providerIcon(e.ModeIDValue()), e.ModeIDValue()))
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
	sb.WriteString(styleDim.Render("install status") + "\n")
	targets := m.svc.Installer.Targets(e)
	if len(targets) == 0 {
		sb.WriteString("  (no matching targets)\n")
	}
	for _, t := range targets {
		st := m.svc.Installer.State(e, t)
		fmt.Fprintf(&sb, "  %-16s %s\n", t.Name, st)
	}

	sb.WriteString(rule + "\n")
	sb.WriteString(styleDim.Render("files") + "\n")
	for _, f := range entryFiles(e.Path) {
		sb.WriteString("  " + f + "\n")
	}

	sb.WriteString(rule + "\n")
	sb.WriteString(styleDim.Render("marker preview") + "\n")
	sb.WriteString(previewMarker(e))

	return sb.String()
}

// ---- frame helpers (box-drawing) ----

func frameTop(inner int, title string) string {
	t := ansi.Truncate(title, maxInt(0, inner-2), "")
	pad := maxInt(1, inner-1-lipgloss.Width(t))
	return "┌─" + t + strings.Repeat("─", pad) + "┐"
}

func frameSep(inner int) string {
	return "├" + strings.Repeat("─", inner) + "┤"
}

func frameBottom(inner int) string {
	return "└" + strings.Repeat("─", inner) + "┘"
}

// fitCell truncates content to w cells (ANSI-aware) and pads it to exactly w
// cells before applying st, so a background style spans the full row.
func fitCell(content string, w int, st lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	t := ansi.Truncate(content, w, "")
	t = t + strings.Repeat(" ", maxInt(0, w-lipgloss.Width(t)))
	return st.Render(t)
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// padLines fills the slice with blank full-width rows up to n entries so the
// surrounding frame keeps its height.
func padLines(lines []string, inner, n int) []string {
	for len(lines) < n {
		lines = append(lines, fitCell("", inner, lipgloss.NewStyle()))
	}
	return lines
}

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
		return styleDim.Render("(unreadable)")
	}
	return strings.TrimSuffix(string(data), "\n")
}
