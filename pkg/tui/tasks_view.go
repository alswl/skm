package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/tui/components"
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
// selected job, cancel all, clear completed, or return to the list.
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
		if m.tasksCursor < len(tasks) {
			m.queue.Cancel(tasks[m.tasksCursor].ID)
		}
	case key.Matches(msg, m.keys.CancelAll):
		m.queue.CancelAll()
	case key.Matches(msg, m.keys.ClearDone):
		m.queue.ClearCompleted()
		m.tasksCursor = 0
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Queue), key.Matches(msg, m.keys.Quit):
		m.showTasks = false
	}
	return nil
}

// tasksView renders the task center as a full-screen framed page.
func (m model) tasksView() string {
	tasks := flattenTasks(m.queue.Snapshot())
	inner := maxInt(20, m.width) - 2
	var body strings.Builder
	if len(tasks) == 0 {
		body.WriteString(components.StyleDim.Render("no background jobs"))
	}
	for i, jb := range tasks {
		row := fmt.Sprintf("#%d  %-10s %s", jb.ID, jb.State, jb.Name)
		if i == m.tasksCursor {
			body.WriteString(components.FitCell("  ▶ "+row, inner, components.StyleCursor) + "\n")
		} else {
			body.WriteString(components.FitCell("    "+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	hint := "[c] cancel  [C] cancel all  [x] clear done  [j/k] move  [esc/J/q] back"
	return m.framedPage(" skm · tasks ", strings.TrimRight(body.String(), "\n"), hint)
}
