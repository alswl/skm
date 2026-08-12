package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/jobs"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
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
	m.setStatus("queued: " + name)
	m.queue.Submit(name, run)
}

// submitJobForce is like submitJob, but if the job fails with an error the
// service layer marked via common.WithNeedsForce (a same-named non-managed
// object exists at the destination), the user is offered a confirmation to
// retry the same operation with force enabled — instead of a failure the TUI
// has no way to recover from.
func (m *model) submitJobForce(name string, run, forceRun func(ctx context.Context) (any, error)) {
	m.setStatus("queued: " + name)
	id := m.queue.Submit(name, run)
	if m.forceRetries == nil {
		m.forceRetries = map[int64]forceRetry{}
	}
	m.forceRetries[id] = forceRetry{name: name, run: forceRun}
}

// handleJobDone applies a completed job result, discarding stale results from
// cancelled jobs (FR-011). The caller (Update's jobDoneMsg case) is
// responsible for re-arming the queue's result listener; this only ever
// returns the follow-up rescan command (or nil), so it never blocks Update
// with the synchronous filesystem walk that used to happen here directly
// ("job 动作进行时候，UI 会被卡住" — every job completion froze rendering and
// input for as long as m.svc.Scan() took).
func (m *model) handleJobDone(r jobs.Result) tea.Cmd {
	retry, hasRetry := m.forceRetries[r.ID]
	delete(m.forceRetries, r.ID)
	if m.queue.IsStale(r.ID) {
		return nil // discard late result
	}
	if preview, ok := r.Value.(fixPreview); ok && r.Err == nil {
		m.showFixConfirmation(preview)
		return nil
	}
	if preview, ok := r.Value.(orphanDanglingPreview); ok && r.Err == nil {
		m.showOrphanDanglingConfirmation(preview)
		return nil
	}
	if r.Err != nil && hasRetry && common.IsNeedsForce(r.Err) {
		m.confirm = &pages.Confirm{
			Prompt: fmt.Sprintf("%s: a same-named non-managed object exists at the destination. Overwrite it?", retry.name),
			OnYes: func() {
				m.submitJob(retry.name, retry.run)
			},
		}
		return nil
	}
	if r.Err != nil {
		m.setStatus("task failed: " + r.Err.Error())
	} else if s, ok := r.Value.(string); ok {
		m.setStatus(s)
	} else {
		m.setStatus("task completed")
	}
	return scanCmd(m.svc, scanAfterJob)
}

// cancelRunningTask marks the running job stale and cancels it cooperatively
// (Ctrl-C, FR-011).
func (m *model) cancelRunningTask() {
	if id := m.queue.RunningID(); id != 0 {
		m.queue.Cancel(id)
		m.setStatus(fmt.Sprintf("cancelled task #%d; continuing with the next", id))
	}
}

// runningJobStatus is the status-line text for the currently running job (and
// how many are still queued behind it), or "" when the queue is idle.
func (m model) runningJobStatus() string {
	name, queued := m.queue.RunningStatus()
	if name == "" {
		return ""
	}
	text := "▶ " + name
	if queued > 0 {
		text += fmt.Sprintf(" · %d queued", queued)
	}
	return text
}
