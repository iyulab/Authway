package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"authway/apps/central/api/internal/hydra"
)

// authTimeTTL bounds how long a recorded authentication is kept. A browser
// session that outlives it simply has no known authentication time, and
// anything that needs a recent one asks the user to sign in again.
const authTimeTTL = 30 * 24 * time.Hour

// AuthTimeStore remembers when the user of a browser session last proved who
// they are, keyed by the authorization server's session id.
//
// A session is reused without signing in again (single sign-on), so the time
// a token was issued says nothing about when its holder last authenticated.
// Tokens carry that time as auth_time, and an operation that must not run on
// a long-lived or stolen session — deleting the account — checks it. The
// authorization server keeps the time but does not hand it to the consent
// step, which is where the token's claims are set; hence this store.
type AuthTimeStore struct {
	redis  *redis.Client
	prefix string
}

func NewAuthTimeStore(redisClient *redis.Client) *AuthTimeStore {
	return &AuthTimeStore{redis: redisClient, prefix: "auth_time:"}
}

// Record notes that the session's user authenticated at the given time.
func (s *AuthTimeStore) Record(sessionID string, at time.Time) {
	if s == nil || s.redis == nil || sessionID == "" {
		return
	}
	s.redis.Set(context.Background(), s.prefix+sessionID, at.Unix(), authTimeTTL)
}

// Lookup returns when the session's user last authenticated, if known.
func (s *AuthTimeStore) Lookup(sessionID string) (time.Time, bool) {
	if s == nil || s.redis == nil || sessionID == "" {
		return time.Time{}, false
	}
	raw, err := s.redis.Get(context.Background(), s.prefix+sessionID).Result()
	if err != nil {
		return time.Time{}, false
	}
	unix, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(unix, 0), true
}

// acceptAuthenticatedLogin accepts a login request for a user who has just
// authenticated — by password, second factor, emailed link or social
// provider — and records the time against the browser session. Accepting a
// request because the session is being reused must not come through here.
func acceptAuthenticatedLogin(hc *hydra.Client, times *AuthTimeStore, challenge string, body *hydra.AcceptLoginRequest) (*hydra.LoginResponse, error) {
	// The request can no longer be read once it has been accepted.
	sessionID := ""
	if req, err := hc.GetLoginRequest(challenge); err == nil {
		sessionID = req.SessionID
	}
	resp, err := hc.AcceptLoginRequest(challenge, body)
	if err == nil {
		times.Record(sessionID, time.Now())
	}
	return resp, err
}

// withAuthTime returns claims for an access token with auth_time added when
// the session's authentication time is known. The map it is given is left
// untouched: the same claims may also go into the ID token, whose auth_time
// the authorization server sets itself.
func withAuthTime(claims map[string]any, times *AuthTimeStore, sessionID string) map[string]any {
	out := make(map[string]any, len(claims)+1)
	for k, v := range claims {
		out[k] = v
	}
	if at, ok := times.Lookup(sessionID); ok {
		out["auth_time"] = at.Unix()
	}
	return out
}
