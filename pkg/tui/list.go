package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
func (m model) View() string {
	switch {
	case m.picker != nil:
		return m.pickerView()
	case m.confirm != nil:
		return m.confirmView()
	case m.showTasks:
		return m.tasksView()
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
	rows := maxInt(1, h-6) // top, [list], status-sep, status, help-sep, help, bottom
	title := " skm · " + m.svc.Repo.Root()

	var sb strings.Builder
	sb.WriteString(frameTop(inner, title) + "\n")
	for _, r := range m.renderMainArea(inner, rows) {
		sb.WriteString("│" + r + "│\n")
	}
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell(m.statusContent(), inner, styleStatusBar) + "│\n")
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell(m.help.ShortHelpView(m.keys.ShortHelp()), inner, lipgloss.NewStyle()) + "│\n")
	sb.WriteString(frameBottom(inner))
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
		row := renderEntryLine(e, m.installCol[e.Name])
		if r.entryIdx == m.cursor {
			out = append(out, fitCell("  ▶ "+row, inner, styleCursor))
		} else {
			out = append(out, fitCell("    "+row, inner, lipgloss.NewStyle()))
		}
	}
	return padLines(out, inner, rows)
}

// renderEntryLine is one entry row under its section header. mode_id and group
// live in the header, so the row carries name · kind · version · status ·
// install summary (FR-041). install is the precomputed per-target summary
// (e.g. "claude,codex" or "—").
func renderEntryLine(e *common.Entry, install string) string {
	if install == "" {
		install = "—"
	}
	return fmt.Sprintf("%-24s %-7s %-8s %-8s %s", e.Name, e.Kind, orDash(e.VersionValue()), e.Status, install)
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
// Enter/v. m.detail is built lazily in openDetail (Update), never here.
func (m model) detailView() string {
	w := maxInt(20, m.width)
	inner := w - 2
	title := " skm · detail "
	if len(m.filtered) > 0 {
		title = " skm · " + m.filtered[m.cursor].Name + " · detail "
	}
	var sb strings.Builder
	sb.WriteString(frameTop(inner, title) + "\n")
	for _, l := range splitLines(m.detail) {
		sb.WriteString("│" + fitCell(l, inner, lipgloss.NewStyle()) + "│\n")
	}
	sb.WriteString(frameSep(inner) + "\n")
	sb.WriteString("│" + fitCell("[esc] back   [v/Enter] detail   [q] quit", inner, styleStatusBar) + "│\n")
	sb.WriteString(frameBottom(inner))
	return sb.String()
}

// buildDetail renders the detail page: metadata, source/origin, install status,
// file tree and marker preview (tui-contract.md). It reads the filesystem, so
// it runs in Update (via openDetail), never in View.
func (m model) buildDetail() string {
	if len(m.filtered) == 0 {
		return styleDim.Render("no entries")
	}
	e := m.filtered[m.cursor]

	var sb strings.Builder
	sb.WriteString(styleTitle.Render(e.Name) + "\n\n")
	fmt.Fprintf(&sb, "kind:     %s\n", e.Kind)
	fmt.Fprintf(&sb, "status:   %s\n", e.Status)
	fmt.Fprintf(&sb, "mode_id:  %s\n", e.ModeIDValue())
	fmt.Fprintf(&sb, "group:    %s\n", orDash(e.GroupValue()))
	fmt.Fprintf(&sb, "version:  %s\n", orDash(e.VersionValue()))
	if e.Origin != nil {
		fmt.Fprintf(&sb, "origin:   %s\n", e.Origin.Address)
	} else {
		sb.WriteString("origin:   none\n")
	}
	if e.Error != nil {
		fmt.Fprintf(&sb, "error:    %s\n", *e.Error)
	}

	sb.WriteString("\n" + styleDim.Render("install status") + "\n")
	for _, t := range m.svc.Installer.Targets(e) {
		st := m.svc.Installer.State(e, t)
		fmt.Fprintf(&sb, "  %-16s %s\n", t.Name, st)
	}

	sb.WriteString("\n" + styleDim.Render("files") + "\n")
	for _, f := range entryFiles(e.Path) {
		sb.WriteString("  " + f + "\n")
	}

	sb.WriteString("\n" + styleDim.Render("marker preview") + "\n")
	sb.WriteString(previewMarker(e, 8))

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

// previewMarker shows the first few lines of the entry's marker file.
func previewMarker(e *common.Entry, n int) string {
	data, err := os.ReadFile(e.MarkerPath())
	if err != nil {
		return styleDim.Render("(unreadable)")
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
