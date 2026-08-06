package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// waitResult reads one result from the queue with a timeout.
func waitResult(t *testing.T, q *Queue) Result {
	t.Helper()
	select {
	case r := <-q.Results():
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for job result")
		return Result{}
	}
}

func TestQueueFIFOSingleConcurrency(t *testing.T) {
	q := New(8)
	defer q.Close()

	var mu sync.Mutex
	var running int
	var order []int64
	overlap := false

	mkJob := func(id int64) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			mu.Lock()
			running++
			if running > 1 {
				overlap = true
			}
			order = append(order, id)
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			running--
			mu.Unlock()
			return id, nil
		}
	}

	q.Submit("a", mkJob(1))
	q.Submit("b", mkJob(2))
	q.Submit("c", mkJob(3))

	got := []int64{waitResult(t, q).Value.(int64), waitResult(t, q).Value.(int64), waitResult(t, q).Value.(int64)}
	require.Equal(t, []int64{1, 2, 3}, got, "strictly FIFO order")
	require.False(t, overlap, "single concurrency: no two jobs overlap")
	require.Equal(t, []int64{1, 2, 3}, order)
}

func TestQueueMonotonicIDs(t *testing.T) {
	q := New(8)
	ids := []int64{
		q.Submit("a", func(ctx context.Context) (any, error) { return nil, nil }),
		q.Submit("b", func(ctx context.Context) (any, error) { return nil, nil }),
		q.Submit("c", func(ctx context.Context) (any, error) { return nil, nil }),
	}
	require.Equal(t, []int64{1, 2, 3}, ids, "monotonic ids")
}

func TestQueueCancelDropsStaleResultAndContinues(t *testing.T) {
	q := New(8)
	defer q.Close()

	// First job blocks until its context is cancelled.
	firstStarted := make(chan struct{})
	firstID := q.Submit("blocking", func(ctx context.Context) (any, error) {
		close(firstStarted)
		<-ctx.Done() // cooperative cancel
		return "late", nil
	})

	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first job did not start")
	}

	// Second job should proceed only after the first returns.
	secondID := q.Submit("second", func(ctx context.Context) (any, error) {
		return "second-done", nil
	})

	require.Equal(t, firstID, q.RunningID(), "first job is running")
	q.Cancel(firstID)
	require.True(t, q.IsStale(firstID), "cancelled id marked stale")

	// The late result of the cancelled job is discarded.
	firstResult := waitResult(t, q)
	require.Equal(t, firstID, firstResult.ID)
	require.True(t, q.IsStale(firstResult.ID), "late result must be stale")

	// The next job proceeds and its result is not stale.
	secondResult := waitResult(t, q)
	require.Equal(t, secondID, secondResult.ID)
	require.False(t, q.IsStale(secondResult.ID))
	require.Equal(t, "second-done", secondResult.Value)
}

func TestQueueSnapshotAndCompleted(t *testing.T) {
	q := New(8)
	defer q.Close()

	id := q.Submit("done", func(ctx context.Context) (any, error) { return "ok", nil })
	require.Equal(t, id, waitResult(t, q).ID)

	// After draining, the job is in completed and nothing is pending/running.
	snap := q.Snapshot()
	require.Nil(t, snap.Running)
	require.Empty(t, snap.Pending)
	require.Len(t, snap.Completed, 1)
	require.Equal(t, JobDone, snap.Completed[0].State)

	q.ClearCompleted()
	require.Empty(t, q.Snapshot().Completed)
}

func TestQueueCancelPendingSkipsExecution(t *testing.T) {
	q := New(8)
	defer q.Close()

	// A blocking first job holds the single worker so the rest stay queued.
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	q.Submit("block", func(ctx context.Context) (any, error) {
		close(firstStarted)
		<-release
		return "first", nil
	})
	<-firstStarted

	var ran bool
	pendingID := q.Submit("victim", func(ctx context.Context) (any, error) {
		ran = true
		return "should-not-run", nil
	})
	tailID := q.Submit("tail", func(ctx context.Context) (any, error) { return "tail", nil })

	// Cancel while still queued: recorded as cancelled immediately.
	q.Cancel(pendingID)
	require.True(t, q.IsStale(pendingID))
	found := false
	for _, c := range q.Snapshot().Completed {
		if c.ID == pendingID {
			require.Equal(t, JobCancelled, c.State)
			found = true
		}
	}
	require.True(t, found, "cancelled pending job recorded in completed")

	close(release)

	// Drain: first (real), victim (skipped/stale), tail (real).
	require.Equal(t, "first", waitResult(t, q).Value)
	victimRes := waitResult(t, q)
	require.Equal(t, pendingID, victimRes.ID)
	require.True(t, q.IsStale(victimRes.ID), "skipped job yields a stale result")
	require.Equal(t, tailID, waitResult(t, q).ID)
	require.False(t, ran, "cancelled pending job never executes")
}

func TestQueueCancelAll(t *testing.T) {
	q := New(8)
	defer q.Close()

	firstStarted := make(chan struct{})
	release := make(chan struct{})
	q.Submit("block", func(ctx context.Context) (any, error) {
		close(firstStarted)
		<-release
		return "first", nil
	})
	<-firstStarted
	q.Submit("a", func(ctx context.Context) (any, error) { return "a", nil })
	q.Submit("b", func(ctx context.Context) (any, error) { return "b", nil })

	q.CancelAll()
	require.Empty(t, q.Snapshot().Pending, "all queued jobs dropped")

	close(release)
	// Drain all three results so the worker returns to idle.
	for i := 0; i < 3; i++ {
		waitResult(t, q)
	}
	require.Equal(t, int64(0), q.RunningID())
}
