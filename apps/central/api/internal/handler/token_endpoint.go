package handler

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/delegation"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// grantTypeTokenExchange is the grant of RFC 8693 (OAuth 2.0 Token Exchange).
const grantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

// TokenEndpoint is the token endpoint that discovery advertises
// (token_endpoint). It stands in front of the authorization server's own: the
// grants the authorization server knows go to it unchanged — the body, the
// client's authentication, the answer — and the grants Authway assembles
// itself are answered here.
//
// A client authenticating with a signed assertion (private_key_jwt) addresses
// it to this endpoint; the authorization server accepts that audience because
// it is the token endpoint its discovery names.
type TokenEndpoint struct {
	// engineToken is the authorization server's own token endpoint.
	engineToken string
	http        *http.Client
	logger      *zap.Logger
	exchanger   *delegation.Exchanger
	audit       audit.Service
}

// ExchangesDelegations has the endpoint answer token exchange (RFC 8693) of a
// delegation with x. Without it, token exchange is unsupported_grant_type.
func (h *TokenEndpoint) ExchangesDelegations(x *delegation.Exchanger, auditService audit.Service) {
	h.exchanger = x
	h.audit = auditService
}

// NewTokenEndpoint forwards to the token endpoint of the authorization server
// reachable at engineURL (its public address, or one inside the deployment's
// network that reaches the same server).
func NewTokenEndpoint(engineURL string, logger *zap.Logger) *TokenEndpoint {
	return &TokenEndpoint{
		engineToken: strings.TrimRight(engineURL, "/") + "/oauth2/token",
		http: &http.Client{
			Timeout: 30 * time.Second,
			// An answer is passed back as it is, a redirect included.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		logger: logger,
	}
}

// forwardedRequestHeaders are the headers of a token request the authorization
// server reads. Cookies are not among them: a token request is authenticated
// by the client, never by a browser session.
var forwardedRequestHeaders = []string{
	fiber.HeaderContentType,
	fiber.HeaderAuthorization,
	fiber.HeaderAccept,
	fiber.HeaderAcceptLanguage,
	fiber.HeaderUserAgent,
	"DPoP",
}

// returnedResponseHeaders are the headers of the authorization server's answer
// a client reads. Its CORS headers are not: whether a page may read the answer
// is decided by this API (middleware.CORS), as it is for every other route.
var returnedResponseHeaders = []string{
	fiber.HeaderContentType,
	fiber.HeaderCacheControl,
	fiber.HeaderPragma,
	fiber.HeaderWWWAuthenticate,
	"DPoP-Nonce",
}

// Token serves POST /oauth2/token.
//
// The request body is never logged: it carries client secrets, codes and
// refresh tokens.
func (h *TokenEndpoint) Token(c *fiber.Ctx) error {
	// Anything that is not a token exchange is the authorization server's to
	// judge, a malformed request included.
	switch grantType(c) {
	case grantTypeTokenExchange:
		if h.exchanger == nil {
			return tokenError(c, fiber.StatusBadRequest, "unsupported_grant_type", "Token exchange is not offered by this deployment.")
		}
		return h.exchange(c)
	case delegation.ApprovalGrant:
		if !h.exchanger.OffersApprovals() {
			return tokenError(c, fiber.StatusBadRequest, "unsupported_grant_type", "Approval requests are not offered by this deployment.")
		}
		return h.approved(c)
	}
	return h.forward(c, c.Body())
}

// exchange answers a token exchange of a delegation: the authorization server
// issues the token by its assertion grant, to the client it authenticates,
// and the answer is put in the shape of RFC 8693 section 2.2.
func (h *TokenEndpoint) exchange(c *fiber.Ctx) error {
	form, _ := url.ParseQuery(string(c.Body()))
	clientID := delegation.ClientFromRequest(c.Get(fiber.HeaderAuthorization), form)
	prepared, refused := h.exchanger.Prepare(c.UserContext(), form, clientID)
	return h.issue(c, prepared, refused, clientID, audit.ActionDelegationExchanged)
}

// approved answers a poll of an approval request (delegation.ApprovalGrant):
// a refusal while it is pending, denied or expired, and once approved the
// token bound to it.
func (h *TokenEndpoint) approved(c *fiber.Ctx) error {
	form, _ := url.ParseQuery(string(c.Body()))
	clientID := delegation.ClientFromRequest(c.Get(fiber.HeaderAuthorization), form)
	prepared, refused := h.exchanger.PrepareApproved(c.UserContext(), form, c.Get(fiber.HeaderAuthorization))
	// Waiting is the ordinary answer of a poll, not a failure worth a record.
	if refused != nil && (refused.Code == "authorization_pending" || refused.Code == "slow_down") {
		return tokenError(c, refused.Status, refused.Code, refused.Description)
	}
	return h.issue(c, prepared, refused, clientID, audit.ActionApprovalExchanged)
}

// issue sends a prepared assertion grant request to the authorization server
// and answers in the shape of RFC 8693 section 2.2, recording action.
func (h *TokenEndpoint) issue(c *fiber.Ctx, prepared *delegation.Exchanged, refused *delegation.OAuthError, clientID string, action audit.AuditAction) error {
	if refused != nil {
		// A refusal of a delegation that exists is recorded against it;
		// a handle that names none has nothing to record against.
		if prepared != nil {
			h.logExchange(c, action, prepared.Delegation, clientID, refused.Code)
		}
		return tokenError(c, refused.Status, refused.Code, refused.Description)
	}
	status, headers, body, err := h.send(c, []byte(prepared.Form.Encode()))
	if err != nil {
		return tokenError(c, fiber.StatusServiceUnavailable, "temporarily_unavailable", "The authorization server did not answer.")
	}
	if status != fiber.StatusOK {
		var engineErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &engineErr)
		// A trust that was replaced or removed behind the API's back is
		// checked again on the next exchange.
		if engineErr.Error == "invalid_grant" || engineErr.Error == "invalid_scope" {
			h.exchanger.Forget(clientID)
		}
		h.logExchange(c, action, prepared.Delegation, clientID, engineErr.Error)
		return h.relay(c, status, headers, body)
	}
	var issued map[string]any
	if err := json.Unmarshal(body, &issued); err != nil {
		return tokenError(c, fiber.StatusBadGateway, "server_error", "The authorization server's answer could not be read.")
	}
	issued["issued_token_type"] = delegation.AccessTokenType
	delete(issued, "refresh_token")
	h.logExchange(c, action, prepared.Delegation, clientID, "")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderPragma, "no-cache")
	return c.Status(fiber.StatusOK).JSON(issued)
}

// logExchange records an exchange as action, failed when refused is the
// refusal's code.
func (h *TokenEndpoint) logExchange(c *fiber.Ctx, action audit.AuditAction, d *delegation.Standing, clientID, refused string) {
	if h.audit == nil || d == nil {
		return
	}
	entry := audit.EntryFromFiber(c, d.TenantID, action, "delegation", d.ID.String())
	entry.ActorType = "client"
	entry.ActorID = nil
	entry.ActorEmail = ""
	entry.Details["client_id"] = clientID
	entry.Details["user_id"] = d.UserID.String()
	if refused != "" {
		entry.Success = false
		entry.ErrorMsg = refused
	}
	h.audit.LogAsync(entry)
}

// grantType reads grant_type from a form body (RFC 6749 section 3.2), or ""
// when the body is not a form.
func grantType(c *fiber.Ctx) string {
	if !strings.HasPrefix(c.Get(fiber.HeaderContentType), fiber.MIMEApplicationForm) {
		return ""
	}
	form, err := url.ParseQuery(string(c.Body()))
	if err != nil {
		return ""
	}
	return form.Get("grant_type")
}

// forward passes body, with the request's headers the authorization server
// reads, to its token endpoint and answers what it answers.
func (h *TokenEndpoint) forward(c *fiber.Ctx, body []byte) error {
	status, headers, answer, err := h.send(c, body)
	if err != nil {
		return tokenError(c, fiber.StatusServiceUnavailable, "temporarily_unavailable", "The authorization server did not answer.")
	}
	return h.relay(c, status, headers, answer)
}

// send posts body to the authorization server's token endpoint.
func (h *TokenEndpoint) send(c *fiber.Ctx, body []byte) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(c.UserContext(), http.MethodPost, h.engineToken, bytes.NewReader(body))
	if err != nil {
		h.logger.Error("Building the token request for the authorization server failed", zap.Error(err))
		return 0, nil, nil, err
	}
	for _, name := range forwardedRequestHeaders {
		if v := c.Get(name); v != "" {
			req.Header.Set(name, v)
		}
	}
	req.Header.Set("X-Forwarded-For", c.IP())

	res, err := h.http.Do(req)
	if err != nil {
		h.logger.Error("The authorization server's token endpoint did not answer", zap.Error(err))
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		h.logger.Error("Reading the authorization server's token answer failed", zap.Error(err))
		return 0, nil, nil, err
	}
	return res.StatusCode, res.Header, answer, nil
}

// relay answers what the authorization server answered.
func (h *TokenEndpoint) relay(c *fiber.Ctx, status int, headers http.Header, body []byte) error {
	for _, name := range returnedResponseHeaders {
		if v := headers.Get(name); v != "" {
			c.Set(name, v)
		}
	}
	return c.Status(status).Send(body)
}

// tokenError answers in the error shape of RFC 6749 section 5.2, which every
// OAuth client library reads at the token endpoint.
func tokenError(c *fiber.Ctx, status int, code, description string) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderPragma, "no-cache")
	return c.Status(status).JSON(fiber.Map{"error": code, "error_description": description})
}

// ApprovalRequests serves POST /oauth2/approval-requests: a client acting for
// a user asks the user to approve one action (delegation.RequestApproval).
// It answers the polling secret and the address of the approval screen,
// which the client brings to the user by its own means.
func (h *TokenEndpoint) ApprovalRequests(c *fiber.Ctx) error {
	form, _ := url.ParseQuery(string(c.Body()))
	clientID := delegation.ClientFromRequest(c.Get(fiber.HeaderAuthorization), form)
	answer, d, refused := h.exchanger.RequestApproval(c.UserContext(), form, c.Get(fiber.HeaderAuthorization))
	if refused != nil {
		if refused.Code == "invalid_client" {
			c.Set(fiber.HeaderWWWAuthenticate, `Basic realm="approval-requests"`)
		}
		if d != nil {
			h.logExchange(c, audit.ActionApprovalRequested, d, clientID, refused.Code)
		}
		return tokenError(c, refused.Status, refused.Code, refused.Description)
	}
	if h.audit != nil {
		entry := audit.EntryFromFiber(c, d.TenantID, audit.ActionApprovalRequested, "approval_request", answer.Request.ID.String())
		entry.ActorType = "client"
		entry.ActorID = nil
		entry.ActorEmail = ""
		entry.Details["client_id"] = clientID
		entry.Details["user_id"] = d.UserID.String()
		entry.Details["delegation_id"] = d.ID.String()
		h.audit.LogAsync(entry)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderPragma, "no-cache")
	return c.JSON(answer)
}

// ClientExchangeScopes reads from the client registrations what a client may
// be granted by an exchange (delegation.ClientScopes).
func ClientExchangeScopes(clients client.Service) delegation.ClientScopes {
	return clientExchangeScopes{clients}
}

type clientExchangeScopes struct{ clients client.Service }

func (s clientExchangeScopes) ExchangeScopes(clientID string) ([]string, []string, bool, error) {
	cl, err := s.clients.GetByClientID(clientID)
	if err != nil {
		return nil, nil, false, err
	}
	return []string(cl.Scopes), []string(cl.Audience), cl.MayActForUsers(), nil
}

// TokenHookHeader carries the key the authorization server presents when it
// calls the token hook (its OAUTH2_TOKEN_HOOK_AUTH_CONFIG_NAME).
const TokenHookHeader = "X-Authway-Hook-Key"

// TokenHook is called by the authorization server before it issues any token
// (its OAUTH2_TOKEN_HOOK_URL). For the assertion grant — the token of a
// delegation exchange — it decides whether the token may be issued and names
// the client as the actor; every other token it leaves as it is.
type TokenHook struct {
	keys     [][]byte
	exchange *delegation.Exchanger
	logger   *zap.Logger
}

// NewTokenHook accepts calls that present one of keys in TokenHookHeader.
// More than one key is accepted while the key is being changed. exchange may
// be nil: then an assertion grant is refused, since only Authway's own
// assertions are meant to reach the authorization server.
func NewTokenHook(keys []string, exchange *delegation.Exchanger, logger *zap.Logger) *TokenHook {
	h := &TokenHook{exchange: exchange, logger: logger}
	for _, k := range keys {
		if k != "" {
			h.keys = append(h.keys, []byte(k))
		}
	}
	return h
}

// tokenHookRequest is the part of the authorization server's hook call read
// here.
type tokenHookRequest struct {
	Request struct {
		ClientID   string                     `json:"client_id"`
		GrantTypes []string                   `json:"grant_types"`
		Payload    map[string]json.RawMessage `json:"payload"`
	} `json:"request"`
}

// Hook serves POST /internal/token-hook. 204 tells the authorization server
// to issue the token unchanged; 200 adds claims to it; 403 refuses it.
func (h *TokenHook) Hook(c *fiber.Ctx) error {
	if !h.accepts([]byte(c.Get(TokenHookHeader))) {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var call tokenHookRequest
	if err := json.Unmarshal(c.Body(), &call); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if !slices.Contains(call.Request.GrantTypes, hydra.JWTBearerGrant) {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if h.exchange == nil {
		return c.SendStatus(fiber.StatusForbidden)
	}
	answer := h.exchange.Judge(c.UserContext(), payloadValue(call.Request.Payload["assertion"]), call.Request.ClientID)
	if answer.Refuse != "" {
		h.logger.Warn("Token hook refused an assertion grant", zap.String("client_id", call.Request.ClientID), zap.String("reason", answer.Refuse))
		return c.SendStatus(fiber.StatusForbidden)
	}
	return c.JSON(fiber.Map{"session": fiber.Map{"access_token": answer.Claims}})
}

// payloadValue reads a request parameter the authorization server passes on
// as a string or as a list of strings.
func payloadValue(raw json.RawMessage) string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return many[0]
	}
	return ""
}

func (h *TokenHook) accepts(presented []byte) bool {
	ok := false
	for _, k := range h.keys {
		if subtle.ConstantTimeCompare(presented, k) == 1 {
			ok = true
		}
	}
	return ok
}
