package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/ephemeral"
)

// pendingMFALogin is what the first factor of a sign-in hands off to the MFA
// verify stage: which Hydra login request to accept, as whom, how they proved
// themselves first, and with what "remember me" settings — plus a bounded
// retry counter so a stolen or
// guessed mfa_challenge cannot be used to brute-force the 6-digit TOTP space
// (the IP-based rate limiter also covers /mfa/verify and /mfa/recovery —
// this per-challenge cap is a second, independent layer, since the two
// limits key off different identities: IP vs. challenge).
type pendingMFALogin struct {
	HydraChallenge string
	UserID         uuid.UUID
	First          string // audit name of the first factor (signInMethod.audit)
	Provider       string // identity provider, when the first factor was one
	KeepSignedIn   bool   // the user's choice at the first factor (SessionPolicy)
	Attempts       int
	CreatedAt      time.Time
}

// maxMFAAttempts caps consecutive failed TOTP/recovery-code guesses against a
// single mfa_challenge before it is discarded and the user must restart login.
const maxMFAAttempts = 5

// mfaChallengeTTL bounds how long a login that passed its first factor
// stays valid — the same window OAuthStateStore gives a social sign-in.
const mfaChallengeTTL = 10 * time.Minute

// MFAChallengeStore holds logins that passed a first factor and owe a second,
// keyed by an opaque challenge handed to the client (OAuthStateStore has the
// same shape). It moved off in-memory storage because central-api's
// Container App scales to maxReplicas=5, so a challenge created on one
// replica must be readable by whichever replica serves the follow-up
// /mfa/verify request.
type MFAChallengeStore struct {
	store  ephemeral.Store
	prefix string
}

func NewMFAChallengeStore(store ephemeral.Store) *MFAChallengeStore {
	return &MFAChallengeStore{store: store, prefix: "mfa_challenge:"}
}

func (s *MFAChallengeStore) key(challenge string) string {
	return s.prefix + challenge
}

func (s *MFAChallengeStore) Set(challenge string, data *pendingMFALogin) {
	s.store.Put(context.Background(), s.key(challenge), map[string]string{
		"hydra_challenge": data.HydraChallenge,
		"user_id":         data.UserID.String(),
		"first":           data.First,
		"provider":        data.Provider,
		"keep_signed_in":  flag(data.KeepSignedIn),
		"attempts":        strconv.Itoa(data.Attempts),
	}, mfaChallengeTTL)
}

func (s *MFAChallengeStore) Get(challenge string) (*pendingMFALogin, bool) {
	vals, err := s.store.Fields(context.Background(), s.key(challenge))
	if err != nil || len(vals) == 0 {
		return nil, false
	}

	userID, err := uuid.Parse(vals["user_id"])
	if err != nil {
		return nil, false
	}
	attempts, _ := strconv.Atoi(vals["attempts"])

	return &pendingMFALogin{
		HydraChallenge: vals["hydra_challenge"],
		UserID:         userID,
		First:          vals["first"],
		Provider:       vals["provider"],
		KeepSignedIn:   vals["keep_signed_in"] == "1",
		Attempts:       attempts,
	}, true
}

func (s *MFAChallengeStore) Delete(challenge string) {
	s.store.Remove(context.Background(), s.key(challenge))
}

// RecordFailure increments the attempt counter and reports whether the
// challenge exceeded maxMFAAttempts and was discarded as a result.
func (s *MFAChallengeStore) RecordFailure(challenge string) (locked bool) {
	removed, _, err := s.store.CountFailure(context.Background(), s.key(challenge), "attempts", maxMFAAttempts)
	return err == nil && removed
}

// flag is how a stored field holds a boolean.
func flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
