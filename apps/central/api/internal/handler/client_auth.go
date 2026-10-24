package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/delegation"

	"authway/apps/central/api/pkg/ephemeral"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// maxAssertionLife bounds how far ahead a client assertion may expire: its
// id is remembered until then.
const maxAssertionLife = time.Hour

// clientAuthFields are the form parameters a client authenticates with.
var clientAuthFields = []string{"client_id", "client_secret", "client_assertion", "client_assertion_type"}

// ClientRows finds a client's registration by its client_id.
type ClientRows interface {
	GetByClientID(clientID string) (*client.Client, error)
}

// EngineClientAuth checks a client's authentication for the requests Authway
// answers itself (delegation.ClientVerifier) by asking the authorization
// server, so that a client authenticates there exactly as at the token
// endpoint — a secret, or a client assertion (private_key_jwt) addressed to
// the token endpoint. The server's revocation endpoint (RFC 7009)
// authenticates the client before anything else and answers 200 for a token
// that names nothing, so asking it changes nothing.
//
// That endpoint does not remember the assertions it has seen, so Authway does:
// each assertion is taken once.
type EngineClientAuth struct {
	revoke  string
	http    *http.Client
	clients ClientRows
	seen    ephemeral.Store
	logger  *zap.Logger
}

// NewEngineClientAuth returns an EngineClientAuth. engineURL is the
// authorization server's public address as this API reaches it.
func NewEngineClientAuth(engineURL string, clients ClientRows, seen ephemeral.Store, logger *zap.Logger) *EngineClientAuth {
	return &EngineClientAuth{
		revoke:  strings.TrimRight(engineURL, "/") + "/oauth2/revoke",
		http:    &http.Client{Timeout: 10 * time.Second},
		clients: clients,
		seen:    seen,
		logger:  logger,
	}
}

// VerifyClient implements delegation.ClientVerifier.
func (a *EngineClientAuth) VerifyClient(ctx context.Context, authorization string, form url.Values) (string, uuid.UUID, bool) {
	clientID := delegation.ClientFromRequest(authorization, form)
	if clientID == "" {
		return "", uuid.Nil, false
	}
	if !a.engineAccepts(ctx, authorization, form) {
		return "", uuid.Nil, false
	}
	// Remembered only once the server accepted it: an assertion nobody could
	// sign takes no id from anyone.
	if assertion := form.Get("client_assertion"); assertion != "" && !a.firstUse(ctx, clientID, assertion) {
		return "", uuid.Nil, false
	}
	cl, err := a.clients.GetByClientID(clientID)
	if err != nil || cl == nil || cl.Public {
		return "", uuid.Nil, false
	}
	return clientID, cl.ID, true
}

// engineAccepts asks the authorization server whether the request's client
// authentication holds.
func (a *EngineClientAuth) engineAccepts(ctx context.Context, authorization string, form url.Values) bool {
	nothing := make([]byte, 24)
	if _, err := rand.Read(nothing); err != nil {
		return false
	}
	body := url.Values{"token": {"unknown-" + hex.EncodeToString(nothing)}}
	for _, k := range clientAuthFields {
		if v := form.Get(k); v != "" {
			body.Set(k, v)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.revoke, strings.NewReader(body.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	res, err := a.http.Do(req)
	if err != nil {
		a.logger.Error("The authorization server did not answer a client authentication check", zap.Error(err))
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusBadRequest {
		a.logger.Warn("Unexpected answer to a client authentication check", zap.Int("status", res.StatusCode))
	}
	return res.StatusCode == http.StatusOK
}

// firstUse records an accepted client assertion's id until it expires, and
// reports whether it had not been used before. An assertion without an id,
// or one that would be remembered for longer than maxAssertionLife, is not
// taken.
func (a *EngineClientAuth) firstUse(ctx context.Context, clientID, assertion string) bool {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(assertion, claims); err != nil {
		return false
	}
	jti, _ := claims["jti"].(string)
	exp, err := claims.GetExpirationTime()
	if jti == "" || err != nil || exp == nil {
		return false
	}
	life := time.Until(exp.Time)
	if life <= 0 || life > maxAssertionLife {
		return false
	}
	first, err := a.seen.MarkOnce(ctx, "client-assertion:"+clientID+":"+jti, life)
	if err != nil {
		a.logger.Error("Recording a client assertion failed", zap.Error(err))
		return false
	}
	return first
}
