package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/jobs"
)

// submitJob enqueues a long operation onto the FIFO single-concurrency queue.
func (m *model) submitJob(name string, run func(ctx context.Context) (any, error)) {
	m.status = "queued: " + name
	m.queue.Submit(name, run)
}

// handleJobDone applies a completed job result, discarding stale results from
// cancelled jobs (FR-011).
func (m *model) handleJobDone(r jobs.Result) tea.Cmd {
	if m.queue.IsStale(r.ID) {
		return waitForResult(m.queue) // discard late result, keep listening
	}
	if r.Err != nil {
		m.status = "task failed: " + r.Err.Error()
	} else if s, ok := r.Value.(string); ok {
		m.status = s
	} else {
		m.status = "task completed"
	}
	m.applyEntries(m.svc.Scan())
	if m.showDetail && m.cursor < len(m.filtered) {
		m.refreshDetail() // the entry may have moved/installed; show the new state
	}
	return waitForResult(m.queue)
}

// cancelRunningTask marks the running job stale and cancels it cooperatively
// (Ctrl-C, FR-011).
func (m *model) cancelRunningTask() {
	if id := m.queue.RunningID(); id != 0 {
		m.queue.Cancel(id)
		m.status = fmt.Sprintf("cancelled task #%d; continuing with the next", id)
	}
}
