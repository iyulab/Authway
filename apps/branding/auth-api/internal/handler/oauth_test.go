package handler

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// StateStore is the CSRF-protection primitive for the OAuth flow: a state
// value is single-use and unguessable, or the callback can't be trusted to
// belong to the login attempt that started it. Backed by Redis since HD-04
// (claudedocs/HANDOFF.md) — tests run against an in-process miniredis so the
// real HSET/TTL path is exercised, not a mock.

// newTestRedisClient spins up an in-process miniredis instance shared by
// every test in this package that needs a *redis.Client (StateStore here,
// the OAuth google-flow tests too).
func newTestRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()}), mr
}

func newTestStateStore(t *testing.T) (*StateStore, *miniredis.Miniredis) {
	t.Helper()
	client, mr := newTestRedisClient(t)
	return NewStateStore(client), mr
}

func TestStateStore_SetGetDelete(t *testing.T) {
	s, _ := newTestStateStore(t)
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
	s, _ := newTestStateStore(t)
	if _, ok := s.Get("never-set"); ok {
		t.Error("expected Get() on an unknown state to report not-found")
	}
}

func TestStateStore_ExpiresAfterTTL(t *testing.T) {
	s, mr := newTestStateStore(t)
	s.Set("state1", &StateData{LoginChallenge: "lc1", ClientID: "client1"})

	if _, ok := s.Get("state1"); !ok {
		t.Fatal("expected state to exist immediately after Set")
	}

	mr.FastForward(stateTTL + time.Second)

	if _, ok := s.Get("state1"); ok {
		t.Error("expected state to be expired after TTL elapsed")
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
