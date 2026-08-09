package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
)

// The header row and the entry rows are assembled by different functions, and
// only the entry rows carry the four-cell cursor gutter ("  ▶ " / "    ").
// Comparing the two functions to each other therefore proves nothing about the
// screen — these tests read the rendered list instead.

// columnStart returns the display-cell offset of needle within line. It must
// be cells, not bytes or runes: a provider icon like 📂 is one rune but two
// cells wide, so counting runes reports the row one column to the left.
func columnStart(line, needle string) int {
	i := strings.Index(line, needle)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}

// headerAndRow renders the list and returns its header line and the line for
// the named entry.
func headerAndRow(t *testing.T, m *model, entry string) (string, string) {
	t.Helper()
	var header, row string
	for _, l := range strings.Split(m.listView(), "\n") {
		if strings.Contains(l, "kind") && strings.Contains(l, "status") && header == "" {
			header = l
		}
		if strings.Contains(l, entry) && (strings.Contains(l, "  ▶ ") || strings.HasPrefix(l, "│    ")) {
			row = l
		}
	}
	require.NotEmpty(t, header, "no header line in:\n%s", m.listView())
	require.NotEmpty(t, row, "no row for %q in:\n%s", entry, m.listView())
	return header, row
}

// TestHeaderLabelsSitOverTheirColumns is the regression test for the header
// being one gutter to the left of everything it labels.
func TestHeaderLabelsSitOverTheirColumns(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.runInstall("install", "skill-a", []string{"t"})
	drainJob(t, &m)
	m.applyScan(m.svc.Scan())

	header, row := headerAndRow(t, &m, "skill-a")
	require.Equal(t, columnStart(header, "name"), columnStart(row, "skill-a"),
		"the name label sits over the name column\nheader: %s\nrow:    %s", header, row)
	require.Equal(t, columnStart(header, "kind"), columnStart(row, "skill "),
		"the kind label sits over the kind column\nheader: %s\nrow:    %s", header, row)
	require.Equal(t, columnStart(header, "status"), columnStart(row, "active"),
		"the status label sits over the status column\nheader: %s\nrow:    %s", header, row)
}

// Every column the list draws needs a header, or the reader has to guess what
// the bare values under it mean.
func TestEveryColumnHasAHeaderLabel(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	writeFileT(t, m.svc.Repo.Root(), "skills/github/octocat/hello-world/remote/SKILL.md",
		"---\nname: remote\ndescription: d\n---\nb\n")
	m.applyScan(m.svc.Scan())

	header, _ := headerAndRow(t, &m, "remote")
	for _, label := range []string{"name", "repo", "kind", "version", "status"} {
		require.Contains(t, header, label, "the %s column is labelled", label)
	}
	for _, tg := range m.svc.Cfg.Targets {
		require.Contains(t, header, targetLabel(tg.Name), "the %s column is labelled", tg.Name)
	}
}

// Each target column is exactly as wide as its header label, so a one-cell
// status glyph padded to that width lands wherever the padding leaves it —
// hard against the left edge. Read down the list that puts ✓ under the "C" of
// "Claude" but dead centre under "Pi". The glyph belongs in the middle of its
// column, under the middle of its label.
func TestInstallIconsAreCenteredInTheirColumns(t *testing.T) {
	targets := []common.InstallTarget{
		{Name: "claude-skills"},   // "Claude"  — 6 cells
		{Name: "claude-commands"}, // "Claude*" — 7
		{Name: "codex"},           // "Codex"   — 5
		{Name: "pi"},              // "Pi"      — 2
	}
	cells := make([]installCell, len(targets))
	for i, tg := range targets {
		cells[i] = installCell{name: tg.Name, state: common.InstallInstalled}
	}
	entry := &common.Entry{Name: "skill", Kind: common.KindSkill, Status: common.StatusActive}
	row := []rune(renderEntryLine(entry, "", "", cells, false))

	off := iconColWidth + 1 + nameColWidth + 1 + kindColWidth + 1 + versionColWidth + 1 + statusColWidth
	for _, tg := range targets {
		off++ // the space separating this column from the previous one
		w := targetColWidth(tg.Name)
		require.LessOrEqual(t, off+w, len(row), "row is long enough to hold %s's column", tg.Name)
		cell := string(row[off : off+w])
		got := strings.IndexRune(cell, '✓')
		require.Equal(t, (w-1)/2, got,
			"%s: the glyph sits at cell %d of %d, not the middle: %q", tg.Name, got, w, cell)
		off += w
	}
}

// Centering must not change any column's width, or every column after the
// first would shift and the header would stop lining up with the cells.
func TestCenteringKeepsInstallColumnsTheSameWidth(t *testing.T) {
	targets := []common.InstallTarget{{Name: "claude-skills"}, {Name: "pi"}}
	entry := &common.Entry{Name: "skill", Kind: common.KindSkill, Status: common.StatusActive}

	header := len([]rune(installHeaderRow(targets, false)))
	for _, st := range []common.InstallState{
		common.InstallInstalled, common.InstallConflict,
		common.InstallDangling, common.InstallAbsent,
	} {
		cells := make([]installCell, len(targets))
		for i, tg := range targets {
			cells[i] = installCell{name: tg.Name, state: st}
		}
		require.Equal(t, header, len([]rune(renderEntryLine(entry, "", "", cells, false))),
			"a %s row is the same width as the header", st)
	}
}
