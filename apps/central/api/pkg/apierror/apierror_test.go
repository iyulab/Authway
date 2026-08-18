package apierror

import (
	"errors"
	"fmt"
	"testing"
)

func TestMessage_PublicErrorSurfacesOwnText(t *testing.T) {
	err := NewPublic("tenant not found")
	if got := Message(err, "internal error"); got != "tenant not found" {
		t.Errorf("Message() = %q, want %q", got, "tenant not found")
	}
}

func TestMessage_WrappedPublicErrorSurfacesOwnText(t *testing.T) {
	err := fmt.Errorf("create invitation: %w", NewPublic("pending invitation already exists"))
	if got := Message(err, "internal error"); got != "pending invitation already exists" {
		t.Errorf("Message() = %q, want the wrapped Public's text", got)
	}
}

// TestMessage_UnreviewedErrorFallsBackToGeneric pins the fail-closed default
// this package exists for: a raw GORM/driver error (or any error nobody
// explicitly reviewed as safe) must never reach the caller as-is.
func TestMessage_UnreviewedErrorFallsBackToGeneric(t *testing.T) {
	dbErr := errors.New(`pq: duplicate key value violates unique constraint "users_tenant_id_email_key"`)
	wrapped := fmt.Errorf("failed to create invitation: %w", dbErr)

	if got := Message(wrapped, "failed to create invitation"); got != "failed to create invitation" {
		t.Errorf("Message() = %q, want the generic fallback (raw driver text must not leak)", got)
	}
}

func TestMessage_PlainStringError_FallsBackToGeneric(t *testing.T) {
	// A hand-authored fmt.Errorf("...") with no %w is still not a *Public —
	// the convention is opt-in via NewPublic, not "no wrapping = safe".
	err := errors.New("some message")
	if got := Message(err, "generic"); got != "generic" {
		t.Errorf("Message() = %q, want generic fallback for a non-Public error", got)
	}
}
