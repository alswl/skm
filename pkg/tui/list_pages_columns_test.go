package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
)

// The entry list, the targets editor and the task centre are all column
// tables, so they should read the same way: a labelled header over every
// column, and columns that keep their width whatever is in them. Only the
// entry list had either; these tests hold the other two to the same rule.

// pageLine returns the first line of view containing needle.
func pageLine(t *testing.T, view, needle, what string) string {
	t.Helper()
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	t.Fatalf("no %s line (looking for %q) in:\n%s", what, needle, view)
	return ""
}

func TestTargetsPageHasAlignedColumnHeaders(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	m.showTargets = true

	view := m.targetsView()
	header := pageLine(t, view, "platform", "targets header")
	for _, label := range []string{"name", "platform", "accepts", "path"} {
		require.Contains(t, header, label, "the %s column is labelled", label)
	}

	row := pageLine(t, view, "claude-skills", "claude-skills row")
	require.Equal(t, columnStart(header, "name"), columnStart(row, "claude-skills"),
		"the name label sits over the name column\nheader: %s\nrow:    %s", header, row)
	require.Equal(t, columnStart(header, "platform"), columnStart(row, "claude "),
		"the platform label sits over the platform column\nheader: %s\nrow:    %s", header, row)
}

// A target whose name overflows its column must be truncated, not left to push
// platform/accepts/path out of line with the header and with every other row.
func TestTargetsPageColumnsStayAlignedWithLongValues(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	m.svc.Cfg.Targets = []common.InstallTarget{
		{Name: "short", Platform: "p", Accepts: []common.EntryKind{common.KindSkill}, Path: "/a"},
		{Name: "an-extremely-long-target-name-that-overflows", Platform: "p",
			Accepts: []common.EntryKind{common.KindSkill}, Path: "/b"},
	}
	view := m.targetsView()
	short := pageLine(t, view, "short", "short row")
	long := pageLine(t, view, "an-extremely-lon", "long row") // truncated to the column width
	require.Equal(t, columnStart(short, "/a"), columnStart(long, "/b"),
		"the path column starts at the same place in both rows\nshort: %s\nlong:  %s", short, long)
}

// blockingJob never returns, so submitted jobs stay visible in the task
// centre for the length of the test instead of completing and being cleared.
func blockingJob(ctx context.Context) (any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// waitForQueued waits until the queue reports n jobs (running plus pending).
func waitForQueued(t *testing.T, m *model, n int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if len(flattenTasks(m.queue.Snapshot())) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue never reported %d jobs", n)
}

func TestTaskCentrePageHasAlignedColumnHeaders(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	m.showTasks = true
	m.queue.Submit("install skill-a", blockingJob)
	waitForQueued(t, &m, 1)

	view := m.tasksView()
	header := pageLine(t, view, "state", "task centre header")
	for _, label := range []string{"id", "state", "task"} {
		require.Contains(t, header, label, "the %s column is labelled", label)
	}
	row := pageLine(t, view, "install skill-a", "task row")
	require.Equal(t, columnStart(header, "task"), columnStart(row, "install skill-a"),
		"the task label sits over the task column\nheader: %s\nrow:    %s", header, row)
}

// Job ids are printed inline, so a two-digit id used to shove the state column
// one cell right — the task centre came apart as soon as ten jobs had run.
func TestTaskCentreColumnsStayAlignedAcrossIDWidths(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	m.showTasks = true
	for i := 0; i < 11; i++ {
		m.queue.Submit("job", blockingJob)
	}
	waitForQueued(t, &m, 11)

	view := m.tasksView()
	var offsets []int
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "job") && strings.Contains(l, "#") {
			offsets = append(offsets, columnStart(l, "job"))
		}
	}
	require.NotEmpty(t, offsets)
	for _, o := range offsets {
		require.Equal(t, offsets[0], o,
			"every task row starts its name at the same column, whatever the id width:\n%s", view)
	}
}
