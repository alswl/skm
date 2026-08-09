package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
)

// The list has to identify *which* remote an entry came from, not just its bare
// name: a repository holding several GitHub imports otherwise shows a column of
// names with no way to tell one owner/repo from another without moving the
// cursor (req: "Github 要显示 group/repo/name").

// withGroupedEntry adds a GitHub-imported skill to the fixture repo, nested the
// way ImportStaged lays one out, so Scan recovers its owner/repo group.
func withGroupedEntry(t *testing.T, m *model) {
	t.Helper()
	writeFileT(t, m.svc.Repo.Root(), "skills/github/octocat/hello-world/remote-skill/SKILL.md",
		"---\nname: remote-skill\ndescription: from a remote\n---\nbody\n")
	m.applyScan(m.svc.Scan())
}

// rowFor returns the rendered list line for the named entry.
func rowFor(t *testing.T, m *model, name string) string {
	t.Helper()
	for _, line := range strings.Split(m.listView(), "\n") {
		// The status bar names the selected entry too; only the list rows carry
		// the leading cursor gutter.
		if strings.Contains(line, name) && (strings.Contains(line, "  ▶ ") || strings.Contains(line, "    ")) {
			return line
		}
	}
	t.Fatalf("no list row for %q in:\n%s", name, m.listView())
	return ""
}

func TestListRowShowsGroupColumn(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	withGroupedEntry(t, &m)

	require.True(t, m.showGroupColumn())
	require.Contains(t, rowFor(t, &m, "remote-skill"), "octocat/hello-world",
		"the entry's own row must name the repo it came from")
	require.Contains(t, m.listView(), "repo", "the group column needs a header label")
}

// A repository of purely local skills has no owner/repo to show, so the column
// must not appear at all — otherwise every row pays 21 columns of dashes, and
// the install-status columns would be pushed out on terminals that fit them
// today.
func TestGroupColumnAbsentWhenNoEntryHasGroup(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	require.False(t, m.showGroupColumn())
	require.NotContains(t, m.listView(), "repo", "no group column, so no header label for one")

	// Item 8's threshold is unchanged for such a list.
	m.width = 100
	require.True(t, m.showInstallColumns())
}

func TestNarrowTerminalHidesGroupColumn(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	withGroupedEntry(t, &m)
	require.True(t, m.showGroupColumn())

	m.width = 70
	require.False(t, m.showGroupColumn(), "the repo column yields before the entry's own name does")
	require.Contains(t, rowFor(t, &m, "remote-skill"), "remote-skill", "the entry name stays visible")
}

// The group column sits between name and kind, so it has to widen the header
// row by exactly as much as it widens an entry row, or every target label ends
// up offset from the cells beneath it.
func TestGroupColumnKeepsHeaderAlignedWithCells(t *testing.T) {
	targets := []common.InstallTarget{{Name: "claude-skills"}, {Name: "codex"}}
	cells := []installCell{
		{name: "claude-skills", state: common.InstallInstalled},
		{name: "codex", state: common.InstallAbsent},
	}
	entry := &common.Entry{Name: "skill", Kind: common.KindSkill, Status: common.StatusActive}

	header := installHeaderRow(targets, true)
	row := renderEntryLine(entry, "", truncPad("octocat/hello-world", groupColWidth), cells, false)
	require.Equal(t, lipgloss.Width(header), lipgloss.Width(row),
		"header and cells share one layout once the group column is in it")

	// An over-long owner/repo is truncated, not allowed to push later columns.
	wide := renderEntryLine(entry, "", truncPad("a-very-long-owner/and-an-even-longer-repo", groupColWidth), cells, false)
	require.Equal(t, lipgloss.Width(row), lipgloss.Width(wide), "the group column truncates rather than grows")
}

// With the group column present the row is wider, so the install-status columns
// need correspondingly more room before they can be shown alongside it.
func TestGroupColumnRaisesInstallColumnThreshold(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	withGroupedEntry(t, &m)

	require.True(t, m.showGroupColumn(), "100 columns is wide enough for the repo column")
	require.False(t, m.showInstallColumns(),
		"identity beats state: at 100 columns the repo wins the space over install status")

	m.width = 120
	require.True(t, m.showInstallColumns(), "both fit once there is room")
}
