package delegation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/tokenhash"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Token exchange (RFC 8693) of a delegation: what a client sends and is sent.
const (
	// SubjectTokenType names a delegation handle as the subject_token.
	SubjectTokenType = "urn:authway:params:oauth:token-type:delegation-grant"
	// AccessTokenType is the only token type an exchange issues.
	AccessTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

// OAuthError is a refusal at the token endpoint (RFC 6749 section 5.2).
type OAuthError struct {
	Status      int
	Code        string
	Description string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

func refuse(status int, code, description string) *OAuthError {
	return &OAuthError{Status: status, Code: code, Description: description}
}

// Standing is a delegation in force with what an exchange of it depends on.
type Standing struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UserID       uuid.UUID
	ClientID     string         // OAuth client_id
	Scopes       pq.StringArray `gorm:"type:text[]"`
	UserActive   bool
	TenantActive bool
	ClientActive bool
}

// usable reports why the delegation cannot be exchanged now, or "".
func (s *Standing) usable() string {
	switch {
	case !s.UserActive:
		return "the user is suspended"
	case !s.TenantActive:
		return "the user's tenant is turned off"
	case !s.ClientActive:
		return "the client is turned off"
	}
	return ""
}

// standing reads a delegation in force with its user's, tenant's and client's
// state.
func (s *service) standing(ctx context.Context, where string, arg any) (*Standing, error) {
	var rows []Standing
	err := s.db.WithContext(ctx).
		Table("delegation_grants AS d").
		Select("d.id, d.tenant_id, d.user_id, c.client_id, d.scopes, u.active AS user_active, t.active AS tenant_active, c.active AS client_active").
		Joins("JOIN clients c ON c.id = d.client_id AND c.deleted_at IS NULL").
		Joins("JOIN users u ON u.id = d.user_id AND u.deleted_at IS NULL").
		Joins("JOIN tenants t ON t.id = d.tenant_id AND t.deleted_at IS NULL").
		Where("d.revoked_at IS NULL AND (d.expires_at IS NULL OR d.expires_at > ?)", s.now().UTC()).
		Where(where, arg).
		Limit(1).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

// Resolve returns the delegation a handle names.
func (s *service) Resolve(ctx context.Context, handle string) (*Standing, error) {
	return s.standing(ctx, "d.handle_hash = ?", tokenhash.Hash(handle))
}

// Standing returns a delegation by id.
func (s *service) Standing(ctx context.Context, id uuid.UUID) (*Standing, error) {
	return s.standing(ctx, "d.id = ?", id)
}

// Exchanger turns a token exchange request into the request the
// authorization server issues the token by: its assertion grant (RFC 7523)
// with an assertion Authway signs, and the client's own authentication passed
// on unchanged — so the client the token is issued to is the one the
// authorization server authenticated.
type Exchanger struct {
	delegations Service
	signer      *Signer
	engine      TrustKeeper
	clients     ClientScopes
	// issuerBase is where the assertions' issuers are named:
	// {issuerBase}/{client_id}.
	issuerBase string
	// audience is what an assertion is addressed to: the token endpoint
	// discovery names, which the authorization server accepts.
	audience string

	mu      sync.Mutex
	trusted map[string]string // client_id -> what its trust was made for

	approvals  *Approvals
	verifier   ClientVerifier
	approveURL string // the approval screen; ?request=<id> is added
}

// HandlesApprovals has the exchanger take approval requests (a client asks
// the user to approve one action) and issue a token bound to an approval.
// approveURL is the approval screen of the login UI.
func (x *Exchanger) HandlesApprovals(a *Approvals, verifier ClientVerifier, approveURL string) {
	x.approvals = a
	x.verifier = verifier
	x.approveURL = approveURL
}

// OffersApprovals reports whether approval requests are taken.
func (x *Exchanger) OffersApprovals() bool { return x != nil && x.approvals != nil }

// TrustKeeper is the authorization server's trust in assertion issuers.
type TrustKeeper interface {
	TrustedIssuers(issuer string) ([]hydra.TrustedIssuer, error)
	TrustIssuer(t hydra.TrustedIssuer) error
	DistrustIssuer(id string) error
}

// ClientScopes says what a client may be granted and whether it may act for
// users.
type ClientScopes interface {
	ExchangeScopes(clientID string) (scopes []string, audience []string, mayAct bool, err error)
}

// ClientVerifier checks a confidential client's own authentication, for the
// requests Authway answers itself (an approval request and its polling):
// whatever the token endpoint accepts — a secret, or a client assertion —
// from the request's Authorization header and form. It names the client and
// its row.
type ClientVerifier interface {
	VerifyClient(ctx context.Context, authorization string, form url.Values) (clientID string, row uuid.UUID, ok bool)
}

// NewExchanger returns an Exchanger. apiURL is this API's public address.
func NewExchanger(delegations Service, signer *Signer, engine TrustKeeper, clients ClientScopes, apiURL string) *Exchanger {
	base := strings.TrimRight(apiURL, "/")
	return &Exchanger{
		delegations: delegations,
		signer:      signer,
		engine:      engine,
		clients:     clients,
		issuerBase:  base + "/delegation",
		audience:    base + "/oauth2/token",
		trusted:     map[string]string{},
	}
}

// Exchanged is what a prepared exchange is: the form to send the
// authorization server, and the delegation it is for.
type Exchanged struct {
	Form       url.Values
	Delegation *Standing
}

// Prepare checks a token exchange request and builds the assertion grant
// request for it. clientID is the client the request authenticates as — not
// yet verified: the authorization server verifies it, and the token hook
// holds the assertion to it. A refusal of a delegation that exists comes with
// it (Exchanged.Delegation, no Form), so the refusal can be recorded against
// it.
func (x *Exchanger) Prepare(ctx context.Context, form url.Values, clientID string) (*Exchanged, *OAuthError) {
	if form.Get("subject_token") == "" || form.Get("subject_token_type") != SubjectTokenType {
		return nil, refuse(400, "invalid_request", "subject_token must be a delegation handle (subject_token_type "+SubjectTokenType+").")
	}
	if t := form.Get("requested_token_type"); t != "" && t != AccessTokenType {
		return nil, refuse(400, "invalid_request", "Only an access token can be requested.")
	}
	if form.Get("actor_token") != "" || form.Get("actor_token_type") != "" {
		return nil, refuse(400, "invalid_request", "actor_token is not accepted; the client authenticates as itself.")
	}
	if clientID == "" {
		return nil, refuse(401, "invalid_client", "The client must authenticate.")
	}
	d, err := x.delegations.Resolve(ctx, form.Get("subject_token"))
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, refuse(400, "invalid_grant", "The delegation does not exist or was revoked.")
	case err != nil:
		return nil, refuse(500, "server_error", "The delegation could not be read.")
	}
	// A handle is no use to anyone but the client it was granted to.
	if d.ClientID != clientID {
		return &Exchanged{Delegation: d}, refuse(400, "invalid_grant", "The delegation does not exist or was revoked.")
	}
	known := &Exchanged{Delegation: d}
	if why := d.usable(); why != "" {
		return known, refuse(400, "invalid_grant", "The delegation cannot be used now: "+why+".")
	}
	clientScopes, clientAudience, mayAct, err := x.clients.ExchangeScopes(clientID)
	if err != nil {
		return known, refuse(500, "server_error", "The client could not be read.")
	}
	if !mayAct {
		return known, refuse(400, "unauthorized_client", "The client is not registered for token exchange.")
	}
	ceiling := intersect(d.Scopes, clientScopes)
	scopes := ceiling
	if requested := strings.Fields(form.Get("scope")); len(requested) > 0 {
		for _, s := range requested {
			if !slices.Contains(ceiling, s) {
				return known, refuse(400, "invalid_scope", fmt.Sprintf("%q is not within what the user delegated.", s))
			}
		}
		scopes = requested
	}
	audiences := form["audience"]
	for _, a := range audiences {
		if !slices.Contains(clientAudience, a) {
			return known, refuse(400, "invalid_target", fmt.Sprintf("%q is not an audience the client is registered for.", a))
		}
	}

	out, refused := x.engineForm(d, clientID, clientScopes, scopes, audiences, form, "")
	if refused != nil {
		return known, refused
	}
	return &Exchanged{Form: out, Delegation: d}, nil
}

// engineForm builds the assertion grant request for d: an assertion Authway
// signs (naming the approval, when the token is bound to one), the scopes and
// audiences, and the client's own authentication as it sent it.
func (x *Exchanger) engineForm(d *Standing, clientID string, clientScopes, scopes, audiences []string, form url.Values, approvalID string) (url.Values, *OAuthError) {
	issuer := x.issuerBase + "/" + clientID
	if err := x.ensureTrusted(issuer, clientID, clientScopes); err != nil {
		return nil, refuse(503, "temporarily_unavailable", "The authorization server could not be prepared for the exchange.")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":           issuer,
		"sub":           d.UserID.String(),
		"aud":           x.audience,
		"azp":           clientID,
		"delegation_id": d.ID.String(),
		"jti":           uuid.NewString(),
		"iat":           now.Unix(),
		"exp":           now.Add(time.Minute).Unix(),
	}
	if approvalID != "" {
		claims["approval_id"] = approvalID
	}
	assertion, err := x.signer.Sign(claims)
	if err != nil {
		return nil, refuse(500, "server_error", "The exchange could not be signed.")
	}

	out := url.Values{}
	out.Set("grant_type", hydra.JWTBearerGrant)
	out.Set("assertion", assertion)
	out.Set("scope", strings.Join(scopes, " "))
	for _, a := range audiences {
		out.Add("audience", a)
	}
	// The client's own authentication, as it sent it.
	for _, k := range []string{"client_id", "client_secret", "client_assertion", "client_assertion_type"} {
		if v := form.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out, nil
}

// ensureTrusted makes the authorization server trust the signing key for the
// client's issuer, with the client's scopes as the ceiling. What was made is
// remembered; anything else is replaced.
func (x *Exchanger) ensureTrusted(issuer, clientID string, scopes []string) error {
	sorted := slices.Clone(scopes)
	slices.Sort(sorted)
	want := x.signer.KeyID() + " " + strings.Join(sorted, " ")

	x.mu.Lock()
	known := x.trusted[clientID] == want
	x.mu.Unlock()
	if known {
		return nil
	}
	existing, err := x.engine.TrustedIssuers(issuer)
	if err != nil {
		return err
	}
	keep := false
	for _, t := range existing {
		current := slices.Clone(t.Scope)
		slices.Sort(current)
		if !keep && t.KeyID() == x.signer.KeyID() && slices.Equal(current, sorted) && t.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
			keep = true
			continue
		}
		if err := x.engine.DistrustIssuer(t.ID); err != nil {
			return err
		}
	}
	if !keep {
		err := x.engine.TrustIssuer(hydra.TrustedIssuer{
			Issuer:          issuer,
			AllowAnySubject: true,
			Scope:           sorted,
			JWK:             x.signer.PublicJWK(),
			ExpiresAt:       time.Now().UTC().AddDate(10, 0, 0),
		})
		if err != nil {
			return err
		}
	}
	x.mu.Lock()
	x.trusted[clientID] = want
	x.mu.Unlock()
	return nil
}

// Forget drops what is remembered about a client's trust, so the next
// exchange checks it again (after the authorization server refused one).
func (x *Exchanger) Forget(clientID string) {
	x.mu.Lock()
	delete(x.trusted, clientID)
	x.mu.Unlock()
}

// Distrust removes the authorization server's trust in a client's issuer —
// when the client is deleted, so no trust outlives the client it was made for.
func (x *Exchanger) Distrust(clientID string) error {
	x.Forget(clientID)
	existing, err := x.engine.TrustedIssuers(x.issuerBase + "/" + clientID)
	if err != nil {
		return err
	}
	for _, t := range existing {
		if err := x.engine.DistrustIssuer(t.ID); err != nil {
			return err
		}
	}
	return nil
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, s := range a {
		if slices.Contains(b, s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// HookAnswer is what the token hook adds to an access token, or a refusal.
type HookAnswer struct {
	// Claims go into the access token (session.access_token).
	Claims map[string]any
	// Refuse is why the token must not be issued, or "".
	Refuse string
}

// Judge decides the token hook for an assertion grant: the assertion must be
// one this deployment signed, for the client the authorization server
// authenticated, of a delegation that can still be used. The token then
// names the client as the actor (RFC 8693 section 4.1).
func (x *Exchanger) Judge(ctx context.Context, assertion, clientID string) HookAnswer {
	claims, err := x.signer.Verify(assertion)
	if err != nil {
		return HookAnswer{Refuse: "the assertion was not signed by this deployment"}
	}
	azp, _ := claims["azp"].(string)
	iss, _ := claims["iss"].(string)
	if azp == "" || azp != clientID || iss != x.issuerBase+"/"+clientID {
		return HookAnswer{Refuse: "the assertion is not for this client"}
	}
	id, err := uuid.Parse(fmt.Sprint(claims["delegation_id"]))
	if err != nil {
		return HookAnswer{Refuse: "the assertion names no delegation"}
	}
	d, err := x.delegations.Standing(ctx, id)
	if err != nil {
		return HookAnswer{Refuse: "the delegation does not exist or was revoked"}
	}
	if sub, _ := claims["sub"].(string); sub != d.UserID.String() || d.ClientID != clientID {
		return HookAnswer{Refuse: "the assertion does not match the delegation"}
	}
	if why := d.usable(); why != "" {
		return HookAnswer{Refuse: why}
	}
	out := map[string]any{
		"act":       map[string]any{"sub": clientID},
		"tenant_id": d.TenantID.String(),
	}
	// A token bound to an approval is issued once, and carries what was
	// approved.
	if raw, ok := claims["approval_id"].(string); ok {
		if x.approvals == nil {
			return HookAnswer{Refuse: "approvals are not offered"}
		}
		approvalID, err := uuid.Parse(raw)
		if err != nil {
			return HookAnswer{Refuse: "the assertion names no approval"}
		}
		approved, ok, err := x.approvals.Consume(ctx, approvalID, d.ID)
		if err != nil || !ok {
			return HookAnswer{Refuse: "the approval is not approved, or its token was issued already"}
		}
		out["approval"] = map[string]any{"id": approved.ID.String()}
		if approved.AuthorizationDetails != nil {
			var details any
			if json.Unmarshal([]byte(*approved.AuthorizationDetails), &details) == nil {
				out["authorization_details"] = details
			}
		}
	}
	return HookAnswer{Claims: out}
}

// ClientFromRequest is the client a token request says it authenticates as:
// HTTP Basic, the client_id parameter, or the issuer of a client assertion.
// It is a claim, not a proof — the authorization server checks it.
func ClientFromRequest(authorization string, form url.Values) string {
	if strings.HasPrefix(strings.ToLower(authorization), "basic ") {
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(authorization[6:])); err == nil {
			if id, _, ok := strings.Cut(string(raw), ":"); ok {
				if unescaped, err := url.QueryUnescape(id); err == nil {
					return unescaped
				}
				return id
			}
		}
	}
	if id := form.Get("client_id"); id != "" {
		return id
	}
	if a := form.Get("client_assertion"); a != "" {
		claims := jwt.MapClaims{}
		if _, _, err := jwt.NewParser().ParseUnverified(a, claims); err == nil {
			if iss, _ := claims["iss"].(string); iss != "" {
				return iss
			}
		}
	}
	return ""
}
