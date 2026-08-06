package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/jobs"
)

// tea.Cmd constructors live here (go-tui-guides.md). A Cmd calls a service or
// long-running primitive; its result comes back to Update as a typed Msg.

// waitForResult is a tea.Cmd that emits one job result per firing; the model
// re-arms it after each delivery so the UI keeps receiving results.
func waitForResult(q *jobs.Queue) tea.Cmd {
	return func() tea.Msg {
		return jobDoneMsg{Result: <-q.Results()}
	}
}
