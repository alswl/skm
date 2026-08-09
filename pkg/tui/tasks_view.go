package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/tui/components"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// flattenTasks orders the queue snapshot for the task center: the running job
// first, then the FIFO-pending jobs, then completed history (oldest first).
func flattenTasks(s jobs.Snapshot) []jobs.JobInfo {
	var out []jobs.JobInfo
	if s.Running != nil {
		out = append(out, *s.Running)
	}
	out = append(out, s.Pending...)
	out = append(out, s.Completed...)
	return out
}

// handleTasksKey drives the task center (FR-039): navigate jobs, cancel the
// selected job, cancel all, clear completed, or return to the list. Each
// mutating key checks the same availability its footer binding advertises
// (tasksBindings) and sets a specific reason instead of a silent no-op when
// unavailable (FR-002).
func (m *model) handleTasksKey(msg tea.KeyMsg) tea.Cmd {
	tasks := flattenTasks(m.queue.Snapshot())
	switch {
	case key.Matches(msg, m.keys.MoveDown):
		if m.tasksCursor < len(tasks)-1 {
			m.tasksCursor++
		}
	case key.Matches(msg, m.keys.MoveUp):
		if m.tasksCursor > 0 {
			m.tasksCursor--
		}
	case key.Matches(msg, m.keys.CancelSel):
		if m.tasksCursor >= len(tasks) {
			m.setStatus("cancel: no job selected")
			return nil
		}
		t := tasks[m.tasksCursor]
		if t.State != jobs.JobQueued && t.State != jobs.JobRunning {
			m.setStatus(fmt.Sprintf("cancel: %s is already %s", t.Name, t.State))
			return nil
		}
		m.queue.Cancel(t.ID)
	case key.Matches(msg, m.keys.CancelAll):
		snap := m.queue.Snapshot()
		if snap.Running == nil && len(snap.Pending) == 0 {
			m.setStatus("cancel all: no jobs running or queued")
			return nil
		}
		m.queue.CancelAll()
	case key.Matches(msg, m.keys.ClearDone):
		if len(m.queue.Snapshot().Completed) == 0 {
			m.setStatus("clear done: no completed jobs")
			return nil
		}
		m.queue.ClearCompleted()
		m.tasksCursor = 0
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Queue), key.Matches(msg, m.keys.Quit):
		m.showTasks = false
	}
	return nil
}

// tasksBindings is the task-center footer's availability matrix, mirroring
// listBindings/detailBindings (FR-009) so all four major screens dim
// unavailable actions consistently. Snapshot() is a pure in-memory read (no
// I/O), safe here in View.
func (m model) tasksBindings() []pages.HintItem {
	snap := m.queue.Snapshot()
	tasks := flattenTasks(snap)
	cancellable := m.tasksCursor < len(tasks) &&
		(tasks[m.tasksCursor].State == jobs.JobQueued || tasks[m.tasksCursor].State == jobs.JobRunning)
	return []pages.HintItem{
		{Keys: "c", Label: "cancel", Enabled: cancellable},
		{Keys: "C", Label: "cancel all", Enabled: snap.Running != nil || len(snap.Pending) > 0},
		{Keys: "x", Label: "clear done", Enabled: len(snap.Completed) > 0},
		{Keys: "j/k", Label: "move", Enabled: true},
		{Keys: "esc/J/q", Label: "back", Enabled: true},
	}
}

// tasksHint renders the task-center footer, dimming bindings unavailable for
// the current queue state (mirrors listHint/detailHint).
func (m model) tasksHint() string {
	var parts []string
	for _, b := range m.tasksBindings() {
		parts = append(parts, pages.HintBinding(b.Keys, b.Label, b.Enabled))
	}
	return strings.Join(parts, "  ")
}

// Column widths for the task table, following the entry list's rule: the id
// is padded to a fixed width rather than printed inline, so reaching job #10
// does not shove state and name one cell right of the header and of every
// earlier row.
const (
	taskIDColWidth    = 6 // "#99999"
	taskStateColWidth = 10
)

// tasksHeaderRow labels the task table, like installHeaderRow does for the
// entry list. Its caller prefixes rowGutter.
func tasksHeaderRow() string {
	return components.StyleDim.Render(
		truncPad("id", taskIDColWidth) + " " + truncPad("state", taskStateColWidth) + " task")
}

func taskRow(id, state, name string) string {
	return truncPad(id, taskIDColWidth) + " " + truncPad(state, taskStateColWidth) + " " + name
}

// tasksView renders the task center as a full-screen framed page.
func (m model) tasksView() string {
	tasks := flattenTasks(m.queue.Snapshot())
	inner := maxInt(20, m.width) - 2
	var body strings.Builder
	body.WriteString(components.FitCell(rowGutter+tasksHeaderRow(), inner, lipgloss.NewStyle()) + "\n")
	if len(tasks) == 0 {
		body.WriteString(components.StyleDim.Render("no background jobs"))
	}
	for i, jb := range tasks {
		row := taskRow(fmt.Sprintf("#%d", jb.ID), jb.State.String(), jb.Name)
		if i == m.tasksCursor {
			body.WriteString(components.FitCell(cursorGutter+row, inner, components.StyleCursor) + "\n")
		} else {
			body.WriteString(components.FitCell(rowGutter+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	return m.framedPage(" skm · tasks ", strings.TrimRight(body.String(), "\n"), m.tasksHint())
}
