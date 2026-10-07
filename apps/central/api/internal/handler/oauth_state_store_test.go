package handler

import (
	"context"
	"testing"
)

func TestOAuthStateStore_RoundTripIsSingleUse(t *testing.T) {
	store := NewOAuthStateStore(newTestRedisClient(t))
	ctx := context.Background()

	state, err := store.Save(ctx, oauthState{LoginChallenge: "challenge-1", ClientID: "client-1"})
	if err != nil {
		t.Fatal(err)
	}
	if state == "" {
		t.Fatal("Save returned an empty state")
	}

	got, found, err := store.Consume(ctx, state)
	if err != nil || !found {
		t.Fatalf("Consume = %v, %v, %v; want stored data", got, found, err)
	}
	if got.LoginChallenge != "challenge-1" || got.ClientID != "client-1" {
		t.Errorf("Consume returned %+v", got)
	}

	// A provider callback replayed with the same state must not complete again.
	if _, found, _ := store.Consume(ctx, state); found {
		t.Error("state was consumable twice")
	}
}

func TestOAuthStateStore_UnknownStateIsNotFound(t *testing.T) {
	store := NewOAuthStateStore(newTestRedisClient(t))
	if _, found, err := store.Consume(context.Background(), "never-issued"); found || err != nil {
		t.Errorf("Consume(unknown) = found %v, err %v; want not found, no error", found, err)
	}
}

func TestOAuthStateStore_StatesAreDistinct(t *testing.T) {
	store := NewOAuthStateStore(newTestRedisClient(t))
	ctx := context.Background()
	a, _ := store.Save(ctx, oauthState{LoginChallenge: "a"})
	b, _ := store.Save(ctx, oauthState{LoginChallenge: "b"})
	if a == b {
		t.Fatal("two sign-ins received the same state")
	}
}
