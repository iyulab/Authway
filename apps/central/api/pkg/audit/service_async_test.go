package audit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TestLogAsync_BufferOverflowNonBlocking verifies that LogAsync does not block
// the caller when the async channel buffer is saturated. The login path is the
// highest-QPS audit producer; if LogAsync ever blocked, it would pin request
// threads waiting on audit writes — unacceptable for a synchronous request
// path. Contract: buffer-full = drop + warn, never block.
func TestLogAsync_BufferOverflowNonBlocking(t *testing.T) {
	// Build a service with a stalled async worker: we never drain the
	// channel, so after `cap(async)` pushes every subsequent LogAsync must
	// hit the `default` branch and return immediately.
	s := &service{
		logger: zap.NewNop(),
		async:  make(chan *AuditEntry, 8),
	}

	entry := &AuditEntry{
		TenantID: uuid.New(),
		Action:   ActionUserLogin,
		Details:  map[string]interface{}{"k": "v"},
	}

	// Fill the buffer.
	for range cap(s.async) {
		s.LogAsync(entry)
	}

	// Next call must not block. Run with a watchdog.
	done := make(chan struct{})
	go func() {
		for range 1000 {
			s.LogAsync(entry)
		}
		close(done)
	}()

	select {
	case <-done:
		// Good: 1000 extra calls returned quickly.
	case <-time.After(2 * time.Second):
		t.Fatal("LogAsync blocked when buffer was full; expected non-blocking drop")
	}
}
