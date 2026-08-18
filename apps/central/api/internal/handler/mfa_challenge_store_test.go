package handler

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// newTestStore returns a store backed by its own in-process miniredis
// instance, plus that instance so tests can drive time forward (FastForward)
// to exercise TTL expiry.
func newTestStore(t *testing.T) (*MFAChallengeStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewMFAChallengeStore(client), mr
}

func TestMFAChallengeStore_SetGetDelete(t *testing.T) {
	store, _ := newTestStore(t)
	userID := uuid.New()
	data := &pendingMFALogin{
		HydraChallenge: "hydra-1",
		UserID:         userID,
		Remember:       true,
		RememberFor:    3600,
	}

	store.Set("chal-1", data)

	got, ok := store.Get("chal-1")
	if !ok {
		t.Fatal("Get() after Set() = not found, want found")
	}
	if got.HydraChallenge != "hydra-1" || got.UserID != userID || !got.Remember || got.RememberFor != 3600 {
		t.Errorf("Get() = %+v, want fields to round-trip", got)
	}

	store.Delete("chal-1")
	if _, ok := store.Get("chal-1"); ok {
		t.Error("Get() after Delete() = found, want not found")
	}
}

func TestMFAChallengeStore_Get_UnknownChallenge(t *testing.T) {
	store, _ := newTestStore(t)
	if _, ok := store.Get("never-set"); ok {
		t.Error("Get() on unknown challenge = found, want not found")
	}
}

func TestMFAChallengeStore_RecordFailure_LocksAtMaxAttempts(t *testing.T) {
	store, _ := newTestStore(t)
	store.Set("chal-1", &pendingMFALogin{HydraChallenge: "hydra-1", UserID: uuid.New()})

	for i := 1; i < maxMFAAttempts; i++ {
		if locked := store.RecordFailure("chal-1"); locked {
			t.Fatalf("attempt %d: locked = true, want false (cap is %d)", i, maxMFAAttempts)
		}
	}
	if locked := store.RecordFailure("chal-1"); !locked {
		t.Fatalf("attempt %d: locked = false, want true", maxMFAAttempts)
	}

	// A locked challenge is deleted, not just marked — same as the original
	// in-memory implementation's delete-on-exceed.
	if _, ok := store.Get("chal-1"); ok {
		t.Error("challenge still readable after locking, want deleted")
	}
}

// TestMFAChallengeStore_RecordFailure_UnknownChallenge pins the fix over a
// naive HINCRBY: incrementing a field on a key that doesn't exist would
// silently create a one-field hash (attempts=1, no TTL, no hydra_challenge)
// instead of reporting "nothing to fail against".
func TestMFAChallengeStore_RecordFailure_UnknownChallenge(t *testing.T) {
	store, _ := newTestStore(t)
	if locked := store.RecordFailure("never-set"); locked {
		t.Error("RecordFailure() on unknown challenge = true, want false")
	}
	if _, ok := store.Get("never-set"); ok {
		t.Error("RecordFailure() on unknown challenge resurrected it as a phantom entry")
	}
}

func TestMFAChallengeStore_ExpiresAfterTTL(t *testing.T) {
	store, mr := newTestStore(t)
	store.Set("chal-1", &pendingMFALogin{HydraChallenge: "hydra-1", UserID: uuid.New()})

	if _, ok := store.Get("chal-1"); !ok {
		t.Fatal("Get() immediately after Set() = not found")
	}

	mr.FastForward(mfaChallengeTTL + time.Second)

	if _, ok := store.Get("chal-1"); ok {
		t.Error("Get() after TTL elapsed = found, want expired")
	}
}
