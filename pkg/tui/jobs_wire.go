package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/tui/pages"
)

// forceRetry is a queued job's offer to retry with force enabled, kept until
// the job's result arrives so a needs-force failure can become a confirm
// prompt instead of a dead end (see submitJobForce).
type forceRetry struct {
	name string
	run  func(ctx context.Context) (any, error)
}

// submitJob enqueues a long operation onto the FIFO single-concurrency queue.
func (m *model) submitJob(name string, run func(ctx context.Context) (any, error)) {
	m.status = "queued: " + name
	m.queue.Submit(name, run)
}

// submitJobForce is like submitJob, but if the job fails with an error the
// service layer marked via common.WithNeedsForce (a same-named non-managed
// object exists at the destination), the user is offered a confirmation to
// retry the same operation with force enabled — instead of a failure the TUI
// has no way to recover from.
func (m *model) submitJobForce(name string, run, forceRun func(ctx context.Context) (any, error)) {
	m.status = "queued: " + name
	id := m.queue.Submit(name, run)
	if m.forceRetries == nil {
		m.forceRetries = map[int64]forceRetry{}
	}
	m.forceRetries[id] = forceRetry{name: name, run: forceRun}
}

// handleJobDone applies a completed job result, discarding stale results from
// cancelled jobs (FR-011).
func (m *model) handleJobDone(r jobs.Result) tea.Cmd {
	retry, hasRetry := m.forceRetries[r.ID]
	delete(m.forceRetries, r.ID)
	if m.queue.IsStale(r.ID) {
		return waitForResult(m.queue) // discard late result, keep listening
	}
	if r.Err != nil && hasRetry && common.IsNeedsForce(r.Err) {
		m.confirm = &pages.Confirm{
			Prompt: fmt.Sprintf("%s: a same-named non-managed object exists at the destination. Overwrite it?", retry.name),
			OnYes: func() {
				m.submitJob(retry.name, retry.run)
			},
		}
		return waitForResult(m.queue)
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
