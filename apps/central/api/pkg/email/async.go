package email

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ErrTransient marks a send that failed in a way worth trying again — the
// mail service timed out, could not be reached, or answered 429 or 5xx.
// A sender wraps it into the errors it returns for such failures.
var ErrTransient = errors.New("transient mail failure")

// retryAfter is how long a worker waits before each further attempt at a
// transient failure. The first wait outlasts the mail service's own handling
// of the attempt that timed out, so the retry finds that attempt finished —
// its idempotency key then answers with the earlier result instead of
// sending twice.
var retryAfter = []time.Duration{45 * time.Second, 2 * time.Minute}

// ErrMailQueueFull refuses a message while the background sender is as far
// behind as it is allowed to fall.
var ErrMailQueueFull = errors.New("mail queue is full; try again shortly")

// ErrMailerClosed refuses a message handed over after Close.
var ErrMailerClosed = errors.New("mail sender is shutting down")

// Async hands each message to the wrapped service in the background and
// returns at once. A request that sends mail — an invitation, a sign-in link,
// a password reset — answers without waiting for the mail service, which can
// take tens of seconds (for one, while it starts from idle). No caller shows
// the outcome to anyone: a reset or sign-in link answers the same whether or
// not an account exists. Failures are logged.
//
// A fixed set of workers sends from a bounded queue, so a slow mail service
// costs a fixed amount of memory however many requests arrive: when the
// queue is full the message is refused with ErrMailQueueFull. Callers log the
// refusal and answer as they would have otherwise — no request that sends
// mail may reveal whether a message went out.
//
// A send that fails with ErrTransient is put back in the queue after each of
// retryAfter. The worker is free while the message waits, so a mail service
// that keeps failing costs each message one send's time per attempt, as
// before, rather than holding a sender through every wait. Waiting messages
// are bounded by the queue's size; one that finds no room, or is still
// waiting when Close is called, is dropped and logged.
type Async struct {
	inner   EmailService
	logger  *zap.Logger
	queue   chan message
	wg      sync.WaitGroup
	retries []time.Duration

	mu      sync.RWMutex
	closed  bool
	waiting map[*time.Timer]message
}

type message struct {
	kind, to string
	deliver  func() error
	attempt  int // attempts already made
}

// NewAsync wraps inner with workers senders and room for queued messages
// waiting for one.
func NewAsync(inner EmailService, logger *zap.Logger, workers, queued int) *Async {
	if workers < 1 {
		workers = 1
	}
	if queued < 0 {
		queued = 0
	}
	a := &Async{inner: inner, logger: logger, queue: make(chan message, queued), retries: retryAfter, waiting: map[*time.Timer]message{}}
	for i := 0; i < workers; i++ {
		a.wg.Add(1)
		go a.work()
	}
	return a
}

func (a *Async) work() {
	defer a.wg.Done()
	for m := range a.queue {
		a.deliver(m)
	}
}

func (a *Async) deliver(m message) {
	err := m.deliver()
	if err == nil {
		return
	}
	m.attempt++
	if !errors.Is(err, ErrTransient) || m.attempt > len(a.retries) {
		a.logger.Error("Failed to send email", zap.String("kind", m.kind), zap.String("to", m.to), zap.Int("attempts", m.attempt), zap.Error(err))
		return
	}
	a.retryLater(m, err)
}

// retryLater puts m back in the queue once its wait is over.
func (a *Async) retryLater(m message, cause error) {
	wait := a.retries[m.attempt-1]
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		a.drop(m, "shutting down before trying again", cause)
		return
	}
	if len(a.waiting) >= cap(a.queue) {
		a.drop(m, "too many messages waiting to be tried again", cause)
		return
	}
	a.logger.Warn("Sending email failed; trying again", zap.String("kind", m.kind), zap.String("to", m.to),
		zap.Int("attempt", m.attempt), zap.Duration("after", wait), zap.Error(cause))
	var t *time.Timer
	t = time.AfterFunc(wait, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if _, ok := a.waiting[t]; !ok {
			return // Close dropped it
		}
		delete(a.waiting, t)
		select {
		case a.queue <- m:
		default:
			a.drop(m, "queue full when trying again", cause)
		}
	})
	a.waiting[t] = m
}

func (a *Async) drop(m message, why string, cause error) {
	a.logger.Error("Failed to send email; "+why, zap.String("kind", m.kind), zap.String("to", m.to), zap.Int("attempts", m.attempt), zap.Error(cause))
}

func (a *Async) send(kind, to string, deliver func() error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return ErrMailerClosed
	}
	select {
	case a.queue <- message{kind: kind, to: to, deliver: deliver}:
		return nil
	default:
		a.logger.Warn("Mail queue full; refusing a message", zap.String("kind", kind), zap.String("to", to))
		return ErrMailQueueFull
	}
}

// Close stops taking messages and waits until the queued ones have been sent
// or have failed, or until ctx ends.
func (a *Async) Close(ctx context.Context) error {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		close(a.queue)
		for t, m := range a.waiting {
			t.Stop()
			a.drop(m, "shutting down before trying again", nil)
		}
		clear(a.waiting)
	}
	a.mu.Unlock()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Async) SendVerificationEmail(toEmail, token string) error {
	return a.send("verification", toEmail, func() error { return a.inner.SendVerificationEmail(toEmail, token) })
}

func (a *Async) SendPasswordResetEmail(toEmail, token string) error {
	return a.send("password_reset", toEmail, func() error { return a.inner.SendPasswordResetEmail(toEmail, token) })
}

func (a *Async) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	return a.send("invitation", toEmail, func() error {
		return a.inner.SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL)
	})
}

func (a *Async) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	return a.send("magic_link", toEmail, func() error { return a.inner.SendMagicLinkEmail(toEmail, linkURL, isNewUser) })
}
