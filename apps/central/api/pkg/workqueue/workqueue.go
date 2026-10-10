// Package workqueue runs background jobs — sending a mail, delivering a
// webhook — on a fixed set of workers fed from a bounded queue, so work that
// waits on a slow or failing remote service costs a fixed amount of memory and
// a fixed number of goroutines however many requests ask for it.
package workqueue

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrFull refuses a job while the queue is as far behind as it is allowed to
// fall.
var ErrFull = errors.New("work queue is full")

// ErrClosed refuses a job handed over after Close.
var ErrClosed = errors.New("work queue is closed")

// Job is one attempt at a piece of work; attempt counts from 1. It returns
// how long to wait before the next attempt, or Done when there is to be none
// — because it succeeded or because trying again would not help.
type Job func(attempt int) (retryAfter time.Duration)

// Done is what a Job returns when it is not to be attempted again.
const Done time.Duration = -1

// Dropped is told about a job that was due another attempt and will not get
// one, with the reason.
type Dropped func(reason string)

// Queue feeds jobs to its workers.
//
// A job due another attempt goes back into the queue when its wait is over.
// The worker is free meanwhile, so a remote service that keeps failing costs
// each job one attempt's time per attempt rather than holding a worker through
// every wait. Jobs waiting for another attempt are bounded by the queue's
// size; one that finds no room, or is still waiting when Close is called, is
// dropped.
type Queue struct {
	queue chan item
	wg    sync.WaitGroup

	mu      sync.RWMutex
	closed  bool
	waiting map[*time.Timer]item
}

type item struct {
	job     Job
	dropped Dropped
	attempt int // attempts already made
}

// New starts workers workers with room for queued jobs waiting for one.
func New(workers, queued int) *Queue {
	if workers < 1 {
		workers = 1
	}
	if queued < 0 {
		queued = 0
	}
	q := &Queue{queue: make(chan item, queued), waiting: map[*time.Timer]item{}}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.work()
	}
	return q
}

// Submit hands job over and returns at once. dropped may be nil.
func (q *Queue) Submit(job Job, dropped Dropped) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrClosed
	}
	select {
	case q.queue <- item{job: job, dropped: dropped}:
		return nil
	default:
		return ErrFull
	}
}

func (q *Queue) work() {
	defer q.wg.Done()
	for it := range q.queue {
		it.attempt++
		if wait := it.job(it.attempt); wait >= 0 {
			q.retryLater(it, wait)
		}
	}
}

// retryLater puts it back in the queue once its wait is over.
func (q *Queue) retryLater(it item, wait time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		it.drop("shutting down before trying again")
		return
	}
	if len(q.waiting) >= cap(q.queue) {
		it.drop("too many jobs waiting to be tried again")
		return
	}
	var t *time.Timer
	t = time.AfterFunc(wait, func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if _, ok := q.waiting[t]; !ok {
			return // Close dropped it
		}
		delete(q.waiting, t)
		select {
		case q.queue <- it:
		default:
			it.drop("queue full when trying again")
		}
	})
	q.waiting[t] = it
}

func (it item) drop(reason string) {
	if it.dropped != nil {
		it.dropped(reason)
	}
}

// Close stops taking jobs, drops those waiting for another attempt, and waits
// until the queued ones have had their attempt or until ctx ends.
func (q *Queue) Close(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.queue)
		for t, it := range q.waiting {
			t.Stop()
			it.drop("shutting down before trying again")
		}
		clear(q.waiting)
	}
	q.mu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
