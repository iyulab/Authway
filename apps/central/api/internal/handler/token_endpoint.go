package handler

import (
	"bytes"
	"crypto/subtle"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	if grantType(c) == grantTypeTokenExchange {
		return tokenError(c, fiber.StatusBadRequest, "unsupported_grant_type", "Token exchange is not offered by this deployment yet.")
	}
	return h.forward(c)
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

func (h *TokenEndpoint) forward(c *fiber.Ctx) error {
	req, err := http.NewRequestWithContext(c.UserContext(), http.MethodPost, h.engineToken, bytes.NewReader(c.Body()))
	if err != nil {
		h.logger.Error("Building the token request for the authorization server failed", zap.Error(err))
		return tokenError(c, fiber.StatusInternalServerError, "server_error", "The token request could not be passed on.")
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
		return tokenError(c, fiber.StatusServiceUnavailable, "temporarily_unavailable", "The authorization server did not answer.")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		h.logger.Error("Reading the authorization server's token answer failed", zap.Error(err))
		return tokenError(c, fiber.StatusBadGateway, "server_error", "The authorization server's answer could not be read.")
	}
	for _, name := range returnedResponseHeaders {
		if v := res.Header.Get(name); v != "" {
			c.Set(name, v)
		}
	}
	return c.Status(res.StatusCode).Send(body)
}

// tokenError answers in the error shape of RFC 6749 section 5.2, which every
// OAuth client library reads at the token endpoint.
func tokenError(c *fiber.Ctx, status int, code, description string) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderPragma, "no-cache")
	return c.Status(status).JSON(fiber.Map{"error": code, "error_description": description})
}

// TokenHookHeader carries the key the authorization server presents when it
// calls the token hook (its OAUTH2_TOKEN_HOOK_AUTH_CONFIG_NAME).
const TokenHookHeader = "X-Authway-Hook-Key"

// TokenHook is called by the authorization server before it issues any token
// (its OAUTH2_TOKEN_HOOK_URL). It can add claims to the token or refuse to
// issue it. It changes nothing yet: every token keeps the shape it has
// without the hook.
type TokenHook struct {
	keys [][]byte
}

// NewTokenHook accepts calls that present one of keys in TokenHookHeader.
// More than one key is accepted while the key is being changed.
func NewTokenHook(keys []string) *TokenHook {
	h := &TokenHook{}
	for _, k := range keys {
		if k != "" {
			h.keys = append(h.keys, []byte(k))
		}
	}
	return h
}

// Hook serves POST /internal/token-hook. An answer of 204 tells the
// authorization server to issue the token unchanged.
func (h *TokenHook) Hook(c *fiber.Ctx) error {
	if !h.accepts([]byte(c.Get(TokenHookHeader))) {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.SendStatus(fiber.StatusNoContent)
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
