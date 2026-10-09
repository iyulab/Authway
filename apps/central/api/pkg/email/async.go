package email

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Async hands each message to the wrapped service in the background and
// returns at once. A request that sends mail — an invitation, a sign-in link,
// a password reset — answers without waiting for the mail service, which can
// take tens of seconds (for one, while it starts from idle). No caller shows
// the outcome to anyone: a reset or sign-in link answers the same whether or
// not an account exists. Failures are logged.
//
// At most maxInFlight messages are sent at a time; further ones wait their
// turn in the background. Close waits for the ones still in flight.
type Async struct {
	inner  EmailService
	logger *zap.Logger
	slots  chan struct{}
	wg     sync.WaitGroup
}

// NewAsync wraps inner.
func NewAsync(inner EmailService, logger *zap.Logger, maxInFlight int) *Async {
	if maxInFlight < 1 {
		maxInFlight = 1
	}
	return &Async{inner: inner, logger: logger, slots: make(chan struct{}, maxInFlight)}
}

func (a *Async) send(kind, to string, deliver func() error) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.slots <- struct{}{}
		defer func() { <-a.slots }()
		if err := deliver(); err != nil {
			a.logger.Error("Failed to send email", zap.String("kind", kind), zap.String("to", to), zap.Error(err))
		}
	}()
}

// Close waits until every message handed over so far has been sent or has
// failed, or until ctx ends.
func (a *Async) Close(ctx context.Context) error {
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
	a.send("verification", toEmail, func() error { return a.inner.SendVerificationEmail(toEmail, token) })
	return nil
}

func (a *Async) SendPasswordResetEmail(toEmail, token string) error {
	a.send("password_reset", toEmail, func() error { return a.inner.SendPasswordResetEmail(toEmail, token) })
	return nil
}

func (a *Async) SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error {
	a.send("invitation", toEmail, func() error {
		return a.inner.SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL)
	})
	return nil
}

func (a *Async) SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error {
	a.send("magic_link", toEmail, func() error { return a.inner.SendMagicLinkEmail(toEmail, linkURL, isNewUser) })
	return nil
}
