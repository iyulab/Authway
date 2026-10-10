package email

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"authway/apps/central/api/pkg/workqueue"
)

// ErrMailQueueFull refuses a message while the background sender is as far
// behind as it is allowed to fall.
var ErrMailQueueFull = errors.New("mail queue is full; try again shortly")

// ErrMailerClosed refuses a message handed over after Close.
var ErrMailerClosed = errors.New("mail sender is shutting down")

// ErrTransient marks a send that failed in a way worth trying again — the
// mail service timed out, could not be reached, or answered 429 or 5xx.
// A sender wraps it into the errors it returns for such failures.
var ErrTransient = errors.New("transient mail failure")

// retryAfter is how long a message waits before each further attempt at a
// transient failure. The first wait outlasts the mail service's own handling
// of the attempt that timed out, so the retry finds that attempt finished —
// its idempotency key then answers with the earlier result instead of
// sending twice.
var retryAfter = []time.Duration{45 * time.Second, 2 * time.Minute}

// Async hands each message to the wrapped service in the background and
// returns at once. A request that sends mail — an invitation, a sign-in link,
// a password reset — answers without waiting for the mail service, which can
// take tens of seconds (for one, while it starts from idle). No caller shows
// the outcome to anyone: a reset or sign-in link answers the same whether or
// not an account exists. Failures are logged.
//
// Messages are sent from a bounded work queue: when it is full a message is
// refused with ErrMailQueueFull. Callers log the refusal and answer as they
// would have otherwise — no request that sends mail may reveal whether a
// message went out. A send that fails with ErrTransient is tried again after
// each of retryAfter.
type Async struct {
	inner   EmailService
	logger  *zap.Logger
	queue   *workqueue.Queue
	retries []time.Duration
}

// NewAsync wraps inner with workers senders and room for queued messages
// waiting for one.
func NewAsync(inner EmailService, logger *zap.Logger, workers, queued int) *Async {
	return &Async{inner: inner, logger: logger, queue: workqueue.New(workers, queued), retries: retryAfter}
}

func (a *Async) send(kind, to string, deliver func() error) error {
	fields := []zap.Field{zap.String("kind", kind), zap.String("to", to)}
	var last error
	err := a.queue.Submit(func(attempt int) time.Duration {
		last = deliver()
		if last == nil {
			return workqueue.Done
		}
		if !errors.Is(last, ErrTransient) || attempt > len(a.retries) {
			a.logger.Error("Failed to send email", append(fields, zap.Int("attempts", attempt), zap.Error(last))...)
			return workqueue.Done
		}
		wait := a.retries[attempt-1]
		a.logger.Warn("Sending email failed; trying again", append(fields, zap.Int("attempt", attempt), zap.Duration("after", wait), zap.Error(last))...)
		return wait
	}, func(reason string) {
		a.logger.Error("Failed to send email; "+reason, append(fields, zap.Error(last))...)
	})
	switch {
	case errors.Is(err, workqueue.ErrFull):
		a.logger.Warn("Mail queue full; refusing a message", fields...)
		return ErrMailQueueFull
	case errors.Is(err, workqueue.ErrClosed):
		return ErrMailerClosed
	}
	return err
}

// Close stops taking messages and waits until the queued ones have been sent
// or have failed, or until ctx ends. Messages waiting to be tried again are
// dropped and logged.
func (a *Async) Close(ctx context.Context) error {
	return a.queue.Close(ctx)
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
