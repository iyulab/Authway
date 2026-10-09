package email

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// slowSender blocks every send until release is closed.
type slowSender struct {
	release chan struct{}
	sent    atomic.Int32
	active  atomic.Int32
	peak    atomic.Int32
	err     error
}

func (s *slowSender) deliver() error {
	n := s.active.Add(1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	<-s.release
	s.active.Add(-1)
	s.sent.Add(1)
	return s.err
}

func (s *slowSender) SendVerificationEmail(string, string) error  { return s.deliver() }
func (s *slowSender) SendPasswordResetEmail(string, string) error { return s.deliver() }
func (s *slowSender) SendInvitationEmail(string, string, string, string, string) error {
	return s.deliver()
}
func (s *slowSender) SendMagicLinkEmail(string, string, bool) error { return s.deliver() }

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

func TestAsync_ReturnsBeforeTheMailIsSentAndCloseWaitsForIt(t *testing.T) {
	inner := &slowSender{release: make(chan struct{})}
	a := NewAsync(inner, zap.NewNop(), 2, 5)

	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := a.SendMagicLinkEmail("user@example.com", "https://example.com/link", false); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("handing over 5 messages took %s; it must not wait for delivery", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := a.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close before delivery = %v, want the context's deadline", err)
	}
	if err := a.SendMagicLinkEmail("late@example.com", "https://example.com/link", false); !errors.Is(err, ErrMailerClosed) {
		t.Fatalf("send after Close = %v, want ErrMailerClosed", err)
	}

	close(inner.release)
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := inner.sent.Load(); got != 5 {
		t.Fatalf("sent %d, want 5", got)
	}
	if peak := inner.peak.Load(); peak > 2 {
		t.Fatalf("%d sends ran at once, want at most 2", peak)
	}
}

// TestAsync_RefusesWhenTheQueueIsFull guards the memory bound: a slow mail
// service holds the workers, the queue fills, and further messages are
// refused instead of piling up.
func TestAsync_RefusesWhenTheQueueIsFull(t *testing.T) {
	inner := &slowSender{release: make(chan struct{})}
	a := NewAsync(inner, zap.NewNop(), 1, 1)
	defer func() { close(inner.release); _ = a.Close(context.Background()) }()

	if err := a.SendPasswordResetEmail("a@example.com", "t"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return inner.active.Load() == 1 }) // the worker holds the first
	if err := a.SendPasswordResetEmail("b@example.com", "t"); err != nil {
		t.Fatalf("second message should wait in the queue: %v", err)
	}
	if err := a.SendPasswordResetEmail("c@example.com", "t"); !errors.Is(err, ErrMailQueueFull) {
		t.Fatalf("third message = %v, want ErrMailQueueFull", err)
	}
}

func TestAsync_LogsAFailedSend(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	inner := &slowSender{release: make(chan struct{}), err: errors.New("mail service unavailable")}
	close(inner.release)
	a := NewAsync(inner, zap.New(core), 1, 1)

	if err := a.SendPasswordResetEmail("user@example.com", "token"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := logs.FilterMessage("Failed to send email").All()
	if len(entries) != 1 || entries[0].ContextMap()["kind"] != "password_reset" {
		t.Fatalf("logged %v, want one password_reset failure", entries)
	}
}
