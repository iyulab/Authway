package handler

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// pendingMFALogin is what the password stage of Login hands off to the MFA
// verify stage: which Hydra login request to accept, as whom, and with what
// "remember me" settings — plus a bounded local retry counter so a stolen or
// guessed mfa_challenge cannot be used to brute-force the 6-digit TOTP space
// (the IP-based rate limiter exists but is not wired to any route yet, see
// ISSUE-Authway-20260712-security-controls-not-wired.md item B).
type pendingMFALogin struct {
	HydraChallenge string
	UserID         uuid.UUID
	Remember       bool
	RememberFor    int
	Attempts       int
	CreatedAt      time.Time
}

// maxMFAAttempts caps consecutive failed TOTP/recovery-code guesses against a
// single mfa_challenge before it is discarded and the user must restart login.
const maxMFAAttempts = 5

// mfaChallengeTTL bounds how long a password-verified-but-MFA-pending login
// stays valid, mirroring the TTL apps/branding/auth-api's OAuth StateStore
// uses for the equivalent short-lived server-side state.
const mfaChallengeTTL = 10 * time.Minute

// MFAChallengeStore holds password-verified, MFA-pending logins in memory,
// keyed by an opaque challenge handed to the client. Same shape as
// apps/branding/auth-api's StateStore (separate Go module, so not directly
// shared) — in-memory, single-instance. A multi-replica deployment would need
// this moved to Redis for the same reason Phase L (ROADMAP.md) already
// tracks for OAuth state.
type MFAChallengeStore struct {
	mu      sync.RWMutex
	pending map[string]*pendingMFALogin
}

func NewMFAChallengeStore() *MFAChallengeStore {
	store := &MFAChallengeStore{
		pending: make(map[string]*pendingMFALogin),
	}
	go store.cleanupExpired()
	return store
}

func (s *MFAChallengeStore) Set(challenge string, data *pendingMFALogin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[challenge] = data
}

func (s *MFAChallengeStore) Get(challenge string) (*pendingMFALogin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, exists := s.pending[challenge]
	return data, exists
}

func (s *MFAChallengeStore) Delete(challenge string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, challenge)
}

// RecordFailure increments the attempt counter and reports whether the
// challenge exceeded maxMFAAttempts and was discarded as a result.
func (s *MFAChallengeStore) RecordFailure(challenge string) (locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, exists := s.pending[challenge]
	if !exists {
		return false
	}
	data.Attempts++
	if data.Attempts >= maxMFAAttempts {
		delete(s.pending, challenge)
		return true
	}
	return false
}

func (s *MFAChallengeStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for challenge, data := range s.pending {
			if now.Sub(data.CreatedAt) > mfaChallengeTTL {
				delete(s.pending, challenge)
			}
		}
		s.mu.Unlock()
	}
}
