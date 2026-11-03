package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/ephemeral"
)

// oauthState is what a social sign-in remembers between redirecting the user
// to the provider and receiving the provider's callback: which Hydra login
// request to complete, and for which OAuth client.
type oauthState struct {
	LoginChallenge string
	ClientID       string
	// InvitationID, set instead of LoginChallenge, says the sign-in accepts
	// that invitation rather than continuing an application's flow. It is the
	// invitation's id, not its token: the token is a bearer secret the
	// database keeps only as a hash.
	InvitationID string
	// LinkUserID, set instead of LoginChallenge, says the sign-in adds the
	// provider account to that account — an explicit link made from the
	// signed-in user's own account screen — rather than signing anyone in.
	// LinkProvider names the provider the link was started for. Once the
	// provider has answered, the same record is kept as the link's result with
	// LinkSubject and LinkEmail filled in, until the account screen completes
	// it with the user's token.
	LinkUserID   string
	LinkProvider string
	LinkSubject  string
	LinkEmail    string
	// LinkKind says which of the three records of a link this is — a ticket, the
	// state of the provider round trip, or its result — so that one can never be
	// taken for another: each step accepts only the record made for it.
	LinkKind string
}

// The three records of a link, in the order they are made.
const (
	linkTicket = "ticket"
	linkState  = "state"
	linkResult = "result"
)

// oauthStateTTL bounds how long a user may spend at the provider before the
// callback is refused and the sign-in must restart.
const oauthStateTTL = 10 * time.Minute

// OAuthStateStore keeps social sign-in state in Redis, keyed by the opaque
// `state` value sent to the provider. The callback may be served by a
// different replica than the one that started the sign-in, so the state cannot
// live in process memory. Same shape as MFAChallengeStore.
type OAuthStateStore struct {
	store  ephemeral.Store
	prefix string
}

func NewOAuthStateStore(store ephemeral.Store) *OAuthStateStore {
	return &OAuthStateStore{store: store, prefix: "oauth:state:"}
}

func (s *OAuthStateStore) key(state string) string {
	return s.prefix + state
}

// Save stores data under a fresh random state value and returns that value.
func (s *OAuthStateStore) Save(ctx context.Context, data oauthState) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(b)

	if err := s.store.Put(ctx, s.key(state), map[string]string{
		"login_challenge": data.LoginChallenge,
		"client_id":       data.ClientID,
		"invitation_id":   data.InvitationID,
		"link_user_id":    data.LinkUserID,
		"link_provider":   data.LinkProvider,
		"link_subject":    data.LinkSubject,
		"link_email":      data.LinkEmail,
		"link_kind":       data.LinkKind,
	}, oauthStateTTL); err != nil {
		return "", fmt.Errorf("store state: %w", err)
	}
	return state, nil
}

// Consume returns the data stored under state and deletes it in the same
// transaction, so a state value can complete at most one callback.
func (s *OAuthStateStore) Consume(ctx context.Context, state string) (oauthState, bool, error) {
	vals, err := s.store.Take(ctx, s.key(state))
	if err != nil {
		return oauthState{}, false, fmt.Errorf("consume state: %w", err)
	}
	if len(vals) == 0 {
		return oauthState{}, false, nil
	}
	return oauthState{
		LoginChallenge: vals["login_challenge"], ClientID: vals["client_id"], InvitationID: vals["invitation_id"],
		LinkUserID: vals["link_user_id"], LinkProvider: vals["link_provider"], LinkSubject: vals["link_subject"], LinkEmail: vals["link_email"], LinkKind: vals["link_kind"],
	}, true, nil
}
