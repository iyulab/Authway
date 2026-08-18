package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// pendingMFALogin is what the password stage of Login hands off to the MFA
// verify stage: which Hydra login request to accept, as whom, and with what
// "remember me" settings — plus a bounded retry counter so a stolen or
// guessed mfa_challenge cannot be used to brute-force the 6-digit TOTP space
// (the IP-based rate limiter also covers /mfa/verify and /mfa/recovery —
// this per-challenge cap is a second, independent layer, since the two
// limits key off different identities: IP vs. challenge).
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

// recordFailureScript atomically increments the attempt counter and, once it
// reaches maxMFAAttempts, deletes the challenge — mirroring the in-memory
// implementation's single-mutex-held read-modify-write exactly. A plain
// HINCRBY would silently resurrect a missing/expired challenge as a
// one-field hash (attempts=1, no hydra_challenge/user_id, no TTL) instead of
// reporting "not found", so existence is checked first inside the script.
var recordFailureScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return -1
end
local n = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
if n >= tonumber(ARGV[1]) then
	redis.call('DEL', KEYS[1])
	return 1
end
return 0
`)

// MFAChallengeStore holds password-verified, MFA-pending logins in Redis,
// keyed by an opaque challenge handed to the client. Same shape as
// apps/branding/auth-api's StateStore (separate Go module, so not directly
// shared) — both moved off in-memory storage together: central-api's
// Container App scales to maxReplicas=5, so a challenge created on one
// replica must be readable by whichever replica serves the follow-up
// /mfa/verify request.
type MFAChallengeStore struct {
	redis  *redis.Client
	prefix string
}

func NewMFAChallengeStore(redisClient *redis.Client) *MFAChallengeStore {
	return &MFAChallengeStore{redis: redisClient, prefix: "mfa_challenge:"}
}

func (s *MFAChallengeStore) key(challenge string) string {
	return s.prefix + challenge
}

func (s *MFAChallengeStore) Set(challenge string, data *pendingMFALogin) {
	ctx := context.Background()
	key := s.key(challenge)
	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"hydra_challenge": data.HydraChallenge,
		"user_id":         data.UserID.String(),
		"remember":        data.Remember,
		"remember_for":    data.RememberFor,
		"attempts":        data.Attempts,
	})
	pipe.Expire(ctx, key, mfaChallengeTTL)
	pipe.Exec(ctx)
}

func (s *MFAChallengeStore) Get(challenge string) (*pendingMFALogin, bool) {
	ctx := context.Background()
	vals, err := s.redis.HGetAll(ctx, s.key(challenge)).Result()
	if err != nil || len(vals) == 0 {
		return nil, false
	}

	userID, err := uuid.Parse(vals["user_id"])
	if err != nil {
		return nil, false
	}
	rememberFor, _ := strconv.Atoi(vals["remember_for"])
	attempts, _ := strconv.Atoi(vals["attempts"])

	return &pendingMFALogin{
		HydraChallenge: vals["hydra_challenge"],
		UserID:         userID,
		Remember:       vals["remember"] == "1",
		RememberFor:    rememberFor,
		Attempts:       attempts,
	}, true
}

func (s *MFAChallengeStore) Delete(challenge string) {
	s.redis.Del(context.Background(), s.key(challenge))
}

// RecordFailure increments the attempt counter and reports whether the
// challenge exceeded maxMFAAttempts and was discarded as a result.
func (s *MFAChallengeStore) RecordFailure(challenge string) (locked bool) {
	result, err := recordFailureScript.Run(context.Background(), s.redis, []string{s.key(challenge)}, maxMFAAttempts).Int()
	if err != nil {
		return false
	}
	return result == 1
}
