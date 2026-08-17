package handler

import (
	"testing"
	"time"
)

// StateStore is the CSRF-protection primitive for the OAuth flow: a state
// value is single-use and unguessable, or the callback can't be trusted to
// belong to the login attempt that started it.

func TestStateStore_SetGetDelete(t *testing.T) {
	s := NewStateStore()
	data := &StateData{LoginChallenge: "lc1", ClientID: "client1", CreatedAt: time.Now()}
	s.Set("state1", data)

	got, ok := s.Get("state1")
	if !ok {
		t.Fatal("expected state to exist after Set")
	}
	if got.LoginChallenge != "lc1" || got.ClientID != "client1" {
		t.Errorf("Get() = %+v, want LoginChallenge=lc1 ClientID=client1", got)
	}

	s.Delete("state1")
	if _, ok := s.Get("state1"); ok {
		t.Error("expected state to be gone after Delete — state must be single-use")
	}
}

func TestStateStore_UnknownStateNotFound(t *testing.T) {
	s := NewStateStore()
	if _, ok := s.Get("never-set"); ok {
		t.Error("expected Get() on an unknown state to report not-found")
	}
}

func TestGenerateState_UniqueAndWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for range 200 {
		state, err := generateState()
		if err != nil {
			t.Fatalf("generateState() error: %v", err)
		}
		if len(state) != 64 {
			t.Fatalf("generateState() = %q (len %d), want 64 hex characters (32 random bytes)", state, len(state))
		}
		if seen[state] {
			t.Fatalf("generateState() produced a duplicate value: %q", state)
		}
		seen[state] = true
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"shorter than max is unchanged", "short", 10, "short"},
		{"exactly max is unchanged", "exactlyten", 10, "exactlyten"},
		{"longer than max is truncated with ellipsis", "this is definitely too long", 7, "this is..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateString(tt.in, tt.maxLen); got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
			}
		})
	}
}
