package workqueue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSubmitReturnsAtOnceAndCloseWaitsForQueuedJobs(t *testing.T) {
	release := make(chan struct{})
	var done, active, peak atomic.Int32
	job := func(int) time.Duration {
		n := active.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		<-release
		active.Add(-1)
		done.Add(1)
		return Done
	}
	q := New(2, 5)

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := q.Submit(job, nil); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("handing over 5 jobs took %s; it must not wait for them", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := q.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close before the jobs finish = %v, want the context's deadline", err)
	}
	if err := q.Submit(job, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("submit after Close = %v, want ErrClosed", err)
	}

	close(release)
	if err := q.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := done.Load(); got != 5 {
		t.Fatalf("ran %d jobs, want 5", got)
	}
	if p := peak.Load(); p > 2 {
		t.Fatalf("%d jobs ran at once, want at most 2", p)
	}
}

// The memory bound: a slow remote service holds the workers, the queue fills,
// and further jobs are refused instead of piling up.
func TestSubmitRefusesWhenTheQueueIsFull(t *testing.T) {
	release := make(chan struct{})
	var active atomic.Int32
	job := func(int) time.Duration { active.Add(1); <-release; return Done }
	q := New(1, 1)
	defer func() { close(release); _ = q.Close(context.Background()) }()

	if err := q.Submit(job, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return active.Load() == 1 }) // the worker holds the first
	if err := q.Submit(job, nil); err != nil {
		t.Fatalf("second job should wait in the queue: %v", err)
	}
	if err := q.Submit(job, nil); !errors.Is(err, ErrFull) {
		t.Fatalf("third job = %v, want ErrFull", err)
	}
}

func TestAJobIsAttemptedAgainAfterItsWait(t *testing.T) {
	var attempts []int
	var calls atomic.Int32
	q := New(1, 1)
	err := q.Submit(func(attempt int) time.Duration {
		attempts = append(attempts, attempt)
		if calls.Add(1) < 3 {
			return time.Millisecond
		}
		return Done
	}, func(reason string) { t.Errorf("dropped: %s", reason) })
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return calls.Load() == 3 })
	if err := q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 3 || attempts[0] != 1 || attempts[2] != 3 {
		t.Fatalf("attempts = %v, want [1 2 3]", attempts)
	}
}

// A job waiting for another attempt must not hold a worker: otherwise a
// remote service that keeps failing lets a burst of jobs occupy every worker
// for the length of their waits and starve everything else.
func TestAWaitingJobLeavesTheWorkerFree(t *testing.T) {
	var other atomic.Int32
	q := New(1, 4)
	defer func() { _ = q.Close(context.Background()) }()

	if err := q.Submit(func(int) time.Duration { return time.Hour }, nil); err != nil {
		t.Fatal(err)
	}
	if err := q.Submit(func(int) time.Duration { other.Add(1); return Done }, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return other.Load() == 1 })
}

// Shutting down must not wait out a retry delay: the job is given up and
// reported so the server can exit within its grace period.
func TestCloseDropsJobsWaitingForAnotherAttempt(t *testing.T) {
	var calls, drops atomic.Int32
	q := New(1, 1)
	_ = q.Submit(func(int) time.Duration { calls.Add(1); return time.Hour }, func(string) { drops.Add(1) })
	waitFor(t, func() bool { return calls.Load() == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	waitFor(t, func() bool {
		q.mu.RLock()
		defer q.mu.RUnlock()
		return len(q.waiting) == 1
	})
	if err := q.Close(ctx); err != nil {
		t.Fatalf("Close waited out the retry delay: %v", err)
	}
	if drops.Load() != 1 {
		t.Fatalf("reported %d drops, want 1", drops.Load())
	}
}

// Jobs waiting for another attempt are bounded too.
func TestWaitingJobsAreBoundedByTheQueueSize(t *testing.T) {
	var drops atomic.Int32
	q := New(1, 1)
	defer func() { _ = q.Close(context.Background()) }()
	for i := 0; i < 2; i++ {
		for q.Submit(func(int) time.Duration { return time.Hour }, func(string) { drops.Add(1) }) != nil {
			time.Sleep(time.Millisecond)
		}
	}
	waitFor(t, func() bool { return drops.Load() == 1 })
}
