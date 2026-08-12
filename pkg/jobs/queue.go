package jobs

import (
	"context"
	"sync"
)

// Job is a long-running operation run in the background (FR-010).
type Job struct {
	ID   int64
	Name string
	Run  func(ctx context.Context) (any, error)
}

// Result carries a completed job's output tagged with its id.
type Result struct {
	ID    int64
	Value any
	Err   error
}

// JobState is the lifecycle state of a job as reported by Snapshot (FR-039).
type JobState int

const (
	JobQueued JobState = iota
	JobRunning
	JobDone
	JobFailed
	JobCancelled
)

func (s JobState) String() string {
	switch s {
	case JobQueued:
		return "queued"
	case JobRunning:
		return "running"
	case JobDone:
		return "done"
	case JobFailed:
		return "failed"
	case JobCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// JobInfo is one job's id/name/state for the task center (FR-039).
type JobInfo struct {
	ID    int64
	Name  string
	State JobState
}

// Snapshot is a point-in-time view of the queue for the task center: the
// pending jobs (FIFO order), the running job (nil when idle), and the
// completed history (oldest first).
type Snapshot struct {
	Pending   []JobInfo
	Running   *JobInfo
	Completed []JobInfo
}

// Queue is a strictly FIFO, single-concurrency background queue (FR-010). A
// single worker executes jobs one at a time; cancel marks the running job's id
// stale so its late result is discarded and the next job proceeds (FR-011).
// The worker is a daemon; quitting the app abandons it without waiting
// (FR-012). Pending jobs sit in an unbounded slice so Submit never blocks — a
// bounded channel would freeze a batch caller mid-enqueue; a condition
// variable wakes the worker only when the backlog is empty. Alongside the
// backlog it tracks queued/running/completed metadata so the TUI task center
// can list and manage jobs (FR-039).
type Queue struct {
	mu            sync.Mutex
	cond          *sync.Cond
	nextID        int64
	jobs          []*Job // FIFO backlog, unbounded
	results       chan Result
	runningID     int64
	runningName   string
	runningCancel context.CancelFunc
	stale         map[int64]bool
	pending       []JobInfo
	completed     []JobInfo
	closed        bool
}

// New creates a queue. buffer sizes the results channel; the job backlog is
// unbounded.
func New(buffer int) *Queue {
	q := &Queue{
		results: make(chan Result, buffer),
		stale:   map[int64]bool{},
	}
	q.cond = sync.NewCond(&q.mu)
	go q.worker()
	return q
}

// Submit enqueues a job and returns its monotonic id (0 when the queue is
// closed, in which case the job is refused). It never blocks: the backlog is
// unbounded, so any number of jobs can be queued ahead of the single worker.
func (q *Queue) Submit(name string, run func(ctx context.Context) (any, error)) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return 0 // closed: the worker is gone, so a queued job would never run
	}
	q.nextID++
	id := q.nextID
	q.pending = append(q.pending, JobInfo{ID: id, Name: name, State: JobQueued})
	q.jobs = append(q.jobs, &Job{ID: id, Name: name, Run: run})
	q.cond.Signal()
	return id
}

// Results returns the channel of completed job results.
func (q *Queue) Results() <-chan Result { return q.results }

// Cancel marks a job id stale and cooperatively cancels it if it is running.
// The job is not force-killed; its late result is discarded (FR-011). A job
// still queued is dropped: it is recorded as cancelled and the worker skips it.
func (q *Queue) Cancel(id int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cancelLocked(id)
}

// CancelAll cancels the running job and drops every queued job (FR-039).
func (q *Queue) CancelAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.runningID != 0 {
		q.cancelLocked(q.runningID)
	}
	for _, p := range append([]JobInfo(nil), q.pending...) {
		q.cancelLocked(p.ID)
	}
}

// cancelLocked marks id stale; a running job is cancelled cooperatively while a
// still-pending job is moved straight to completed as cancelled. Caller holds mu.
func (q *Queue) cancelLocked(id int64) {
	q.stale[id] = true
	if q.runningID == id {
		if q.runningCancel != nil {
			q.runningCancel()
		}
		return
	}
	for i, p := range q.pending {
		if p.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			p.State = JobCancelled
			q.completed = append(q.completed, p)
			return
		}
	}
}

// ClearCompleted removes finished jobs from the task-center history (FR-039).
func (q *Queue) ClearCompleted() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = nil
}

// Snapshot returns a copy of the current queue state for the task center.
func (q *Queue) Snapshot() Snapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Snapshot{
		Pending:   append([]JobInfo(nil), q.pending...),
		Completed: append([]JobInfo(nil), q.completed...),
	}
	if q.runningID != 0 {
		s.Running = &JobInfo{ID: q.runningID, Name: q.runningName, State: JobRunning}
	}
	return s
}

// IsStale reports whether a result id should be discarded.
func (q *Queue) IsStale(id int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stale[id]
}

// RunningID returns the currently running job's id (0 when idle).
func (q *Queue) RunningID() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.runningID
}

// RunningStatus returns the running job's name and how many jobs are queued
// behind it ("", 0 when idle). It reads live fields without copying the
// completed history, so it is cheap enough for the status bar's per-frame use.
func (q *Queue) RunningStatus() (name string, queued int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.runningID == 0 {
		return "", 0
	}
	return q.runningName, len(q.pending)
}

// Close stops accepting new jobs; the worker drains the backlog and exits.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}

// worker runs jobs one at a time until the queue is closed and drained.
func (q *Queue) worker() {
	for {
		job := q.dequeue()
		if job == nil {
			return
		}
		if q.startJob(job) {
			continue // cancelled while queued: skipped, stale result already emitted
		}

		ctx, cancel := context.WithCancel(context.Background())
		q.mu.Lock()
		q.runningCancel = cancel
		q.mu.Unlock()

		value, err := job.Run(ctx)
		cancel()
		q.finishJob(job, err)
		q.results <- Result{ID: job.ID, Value: value, Err: err}
	}
}

// dequeue returns the next queued job, blocking until one is available or the
// queue is closed (nil).
func (q *Queue) dequeue() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.jobs) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.jobs) == 0 {
		return nil
	}
	job := q.jobs[0]
	q.jobs[0] = nil // release the reference so a drained batch doesn't pin job closures
	q.jobs = q.jobs[1:]
	return job
}

// startJob transitions a job from pending to running, or skips it when it was
// cancelled while still queued. It returns true (and emits a stale result) when
// the job was skipped.
func (q *Queue) startJob(job *Job) (skipped bool) {
	q.mu.Lock()
	if q.stale[job.ID] {
		q.removePending(job.ID)
		q.mu.Unlock()
		q.results <- Result{ID: job.ID}
		return true
	}
	q.removePending(job.ID)
	q.runningID = job.ID
	q.runningName = job.Name
	q.mu.Unlock()
	return false
}

// finishJob records the terminal state of a running job and clears the running
// slot. A job cancelled mid-run is recorded as cancelled, not failed.
func (q *Queue) finishJob(job *Job, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	state := JobDone
	switch {
	case q.stale[job.ID]:
		state = JobCancelled
	case err != nil:
		state = JobFailed
	}
	q.completed = append(q.completed, JobInfo{ID: job.ID, Name: job.Name, State: state})
	q.runningID = 0
	q.runningName = ""
	q.runningCancel = nil
}

// removePending drops id from the pending list. Caller holds mu.
func (q *Queue) removePending(id int64) {
	for i, p := range q.pending {
		if p.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}
