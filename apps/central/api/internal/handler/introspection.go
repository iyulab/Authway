package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/middleware"
)

// IntrospectionHandler answers whether a token is still good (RFC 7662), for
// a resource server that must know at once when a user has signed out. A
// resource server that can live with a token until it expires validates the
// JWT itself and never calls this.
type IntrospectionHandler struct {
	clients client.Service
	hydra   *hydra.Client
	logger  *zap.Logger
	callers *knownCallers
}

func NewIntrospectionHandler(clients client.Service, hydraClient *hydra.Client, logger *zap.Logger) *IntrospectionHandler {
	return &IntrospectionHandler{clients: clients, hydra: hydraClient, logger: logger, callers: newKnownCallers(time.Minute)}
}

// contractClaims are the claims of the token contract; they are answered at
// the top level, as they are in a JWT access token.
var contractClaims = []string{"tenant_id", "email", "name", "auth_time"}

// Introspect serves POST /api/v1/oauth2/introspect.
//
// The caller authenticates as a confidential client, with its id and secret
// in the Authorization header (Basic) or in the form. It learns about tokens
// issued to the clients of its own tenant only: any other token — unknown,
// expired, revoked, or another tenant's — is answered {"active": false},
// which does not tell them apart.
func (h *IntrospectionHandler) Introspect(c *fiber.Ctx) error {
	// Only the form body is read (RFC 7662 section 2.1): a token in the query
	// string would sit in every log the request passes.
	var form IntrospectionRequest
	_ = c.BodyParser(&form)
	id, secret, ok := clientCredentials(c, form)
	if !ok {
		return refuseCaller(c)
	}
	tenantID, ok := h.callerTenant(id, secret)
	if !ok {
		// Counts against the caller's address, as a failed sign-in does.
		middleware.IncrementRateLimitOnFailure(c)
		return refuseCaller(c)
	}

	token := form.Token
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "token is required (form-encoded)",
		})
	}

	found, err := h.hydra.IntrospectToken(token)
	if err != nil {
		h.logger.Error("Token introspection failed at the authorization server", zap.Error(err))
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "temporarily_unavailable",
			"error_description": "the token could not be checked; try again",
		})
	}
	inactive := fiber.Map{"active": false}
	if !found.Active || found.ClientID == "" {
		return c.JSON(inactive)
	}
	// A token of a client turned off is not answered for, whatever the
	// engine still holds.
	issuedTo, err := h.clients.GetByClientID(found.ClientID)
	if err != nil || issuedTo.TenantID != tenantID || !h.clients.InForce(issuedTo) {
		return c.JSON(inactive)
	}

	answer := fiber.Map{"active": true, "client_id": found.ClientID}
	put := func(name string, value any, present bool) {
		if present {
			answer[name] = value
		}
	}
	put("sub", found.Subject, found.Subject != "")
	put("scope", found.Scope, found.Scope != "")
	put("token_type", found.TokenType, found.TokenType != "")
	put("token_use", found.TokenUse, found.TokenUse != "")
	put("iss", found.Issuer, found.Issuer != "")
	put("aud", found.Audience, len(found.Audience) > 0)
	put("exp", found.Exp, found.Exp != 0)
	put("iat", found.Iat, found.Iat != 0)
	put("nbf", found.Nbf, found.Nbf != 0)
	rest := map[string]any{}
	for name, value := range found.Ext {
		rest[name] = value
	}
	for _, name := range contractClaims {
		if value, has := rest[name]; has {
			answer[name] = value
			delete(rest, name)
		}
	}
	put("ext", rest, len(rest) > 0)
	return c.JSON(answer)
}

// callerTenant returns the tenant of the confidential client the credentials
// belong to. Checking a secret is deliberately slow, and a resource server
// asks about every request it serves, so a pair that was just checked is
// remembered briefly.
func (h *IntrospectionHandler) callerTenant(id, secret string) (uuid.UUID, bool) {
	if tenantID, ok := h.callers.get(id, secret); ok {
		return tenantID, true
	}
	caller, err := h.clients.ValidateClient(id, secret)
	// A public client has no secret to prove who it is.
	if err != nil || caller == nil || caller.Public {
		return uuid.Nil, false
	}
	h.callers.put(id, secret, caller.TenantID)
	return caller.TenantID, true
}

func refuseCaller(c *fiber.Ctx) error {
	c.Set(fiber.HeaderWWWAuthenticate, `Basic realm="introspection"`)
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error":             "invalid_client",
		"error_description": "authenticate as a confidential client: its id and secret, as HTTP Basic or in the form",
	})
}

// IntrospectionRequest is the form an introspection request carries. The
// caller's id and secret are in it when it does not send them as HTTP Basic;
// token_type_hint is accepted and not relied on.
type IntrospectionRequest struct {
	Token         string `form:"token"`
	TokenTypeHint string `form:"token_type_hint"`
	ClientID      string `form:"client_id"`
	ClientSecret  string `form:"client_secret"`
}

// clientCredentials reads a client's id and secret from the Authorization
// header (client_secret_basic — both parts form-encoded, RFC 6749 §2.3.1) or
// from the form (client_secret_post).
func clientCredentials(c *fiber.Ctx, form IntrospectionRequest) (id, secret string, ok bool) {
	if header := c.Get(fiber.HeaderAuthorization); strings.HasPrefix(header, "Basic ") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
		if err != nil {
			return "", "", false
		}
		idPart, secretPart, found := strings.Cut(string(raw), ":")
		if !found {
			return "", "", false
		}
		if id, err = url.QueryUnescape(idPart); err != nil {
			return "", "", false
		}
		if secret, err = url.QueryUnescape(secretPart); err != nil {
			return "", "", false
		}
	} else {
		id, secret = form.ClientID, form.ClientSecret
	}
	return id, secret, id != "" && secret != ""
}

// knownCallers remembers, for a short time, credentials that were checked and
// found right. Only a digest of the pair is kept. A secret that has just been
// replaced therefore keeps answering here for up to ttl.
type knownCallers struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[[sha256.Size]byte]knownCaller
}

type knownCaller struct {
	tenantID uuid.UUID
	until    time.Time
}

func newKnownCallers(ttl time.Duration) *knownCallers {
	return &knownCallers{ttl: ttl, entries: map[[sha256.Size]byte]knownCaller{}}
}

func callerKey(id, secret string) [sha256.Size]byte {
	return sha256.Sum256([]byte(id + "\x00" + secret))
}

func (k *knownCallers) get(id, secret string) (uuid.UUID, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	entry, ok := k.entries[callerKey(id, secret)]
	if !ok || time.Now().After(entry.until) {
		return uuid.Nil, false
	}
	return entry.tenantID, true
}

func (k *knownCallers) put(id, secret string, tenantID uuid.UUID) {
	const maxRemembered = 1024
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.entries) >= maxRemembered {
		k.entries = map[[sha256.Size]byte]knownCaller{}
	}
	k.entries[callerKey(id, secret)] = knownCaller{tenantID: tenantID, until: time.Now().Add(k.ttl)}
}
