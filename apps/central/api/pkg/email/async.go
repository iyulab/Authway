package email

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

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
// queue is full the message is refused with ErrMailQueueFull and the request
// fails as it would have if the mail service had refused it.
type Async struct {
	inner  EmailService
	logger *zap.Logger
	queue  chan message
	wg     sync.WaitGroup

	mu     sync.RWMutex
	closed bool
}

type message struct {
	kind, to string
	deliver  func() error
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
	a := &Async{inner: inner, logger: logger, queue: make(chan message, queued)}
	for i := 0; i < workers; i++ {
		a.wg.Add(1)
		go a.work()
	}
	return a
}

func (a *Async) work() {
	defer a.wg.Done()
	for m := range a.queue {
		if err := m.deliver(); err != nil {
			a.logger.Error("Failed to send email", zap.String("kind", m.kind), zap.String("to", m.to), zap.Error(err))
		}
	}
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
