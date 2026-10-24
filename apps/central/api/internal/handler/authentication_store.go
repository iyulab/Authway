package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/ephemeral"
)

// authenticationTTL bounds how long a recorded authentication is kept. A
// browser session that outlives it simply has no known authentication, and
// anything that needs a recent one asks the user to sign in again.
const authenticationTTL = 30 * 24 * time.Hour

// Authentication is when and how the user of a browser session last proved
// who they are.
type Authentication struct {
	At time.Time
	// Methods are Authentication Method Reference values (the amr claim).
	Methods []string
}

// AuthenticationStore remembers the last authentication of a browser session,
// keyed by the authorization server's session id.
//
// A session is reused without signing in again (single sign-on), so the time
// a token was issued says nothing about when, or how, its holder last
// authenticated. Tokens carry that as auth_time and amr, and an operation
// that must not run on a long-lived or stolen session — deleting the account
// — checks the time. The authorization server keeps both but hands neither to
// the steps where Authway sets a token's claims; hence this store.
type AuthenticationStore struct {
	store  ephemeral.Store
	prefix string
}

func NewAuthenticationStore(store ephemeral.Store) *AuthenticationStore {
	// The prefix predates the methods; it stays so that sessions recorded
	// before keep their authentication time.
	return &AuthenticationStore{store: store, prefix: "auth_time:"}
}

type storedAuthentication struct {
	At      int64    `json:"at"`
	Methods []string `json:"amr,omitempty"`
}

// Record notes that the session's user has just authenticated.
func (s *AuthenticationStore) Record(sessionID string, a Authentication) {
	if s == nil || s.store == nil || sessionID == "" {
		return
	}
	raw, err := json.Marshal(storedAuthentication{At: a.At.Unix(), Methods: a.Methods})
	if err != nil {
		return
	}
	s.store.SetValue(context.Background(), s.prefix+sessionID, string(raw), authenticationTTL)
}

// Lookup returns the session's last authentication, if known.
func (s *AuthenticationStore) Lookup(sessionID string) (Authentication, bool) {
	if s == nil || s.store == nil || sessionID == "" {
		return Authentication{}, false
	}
	raw, ok, err := s.store.Value(context.Background(), s.prefix+sessionID)
	if err != nil || !ok {
		return Authentication{}, false
	}
	var stored storedAuthentication
	if err := json.Unmarshal([]byte(raw), &stored); err != nil || stored.At == 0 {
		// A record written before methods were kept is the time alone.
		unix, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Authentication{}, false
		}
		return Authentication{At: time.Unix(unix, 0)}, true
	}
	return Authentication{At: time.Unix(stored.At, 0), Methods: stored.Methods}, true
}

// acceptAuthenticatedLogin accepts a login request for a user who has just
// authenticated — by password, second factor, emailed link or social
// provider — and records when and how (body.AMR) against the browser session.
// Accepting a request because the session is being reused must not come
// through here.
func acceptAuthenticatedLogin(hc *hydra.Client, store *AuthenticationStore, challenge string, body *hydra.AcceptLoginRequest) (*hydra.LoginResponse, error) {
	// The request can no longer be read once it has been accepted.
	sessionID := ""
	if req, err := hc.GetLoginRequest(challenge); err == nil {
		sessionID = req.SessionID
	}
	resp, err := hc.AcceptLoginRequest(challenge, body)
	if err == nil {
		store.Record(sessionID, Authentication{At: time.Now(), Methods: body.AMR})
	}
	return resp, err
}

// sessionClaim names, in an access token's session data, the login session
// the token was issued in. The API reads it back by introspection to tell
// "this session" from the user's others; it is not part of the token contract.
const sessionClaim = "sid"

// withSession returns claims for an access token with what is known of the
// login session it is issued in: the session's id, and auth_time when the
// session's authentication time is known. The map it is given is left
// untouched: the same claims may also go into the ID token, whose sid and
// auth_time the authorization server sets itself.
func withSession(claims map[string]any, store *AuthenticationStore, sessionID string) map[string]any {
	out := make(map[string]any, len(claims)+2)
	for k, v := range claims {
		out[k] = v
	}
	if sessionID != "" {
		out[sessionClaim] = sessionID
	}
	if a, ok := store.Lookup(sessionID); ok {
		out["auth_time"] = a.At.Unix()
	}
	return out
}
