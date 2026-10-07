package handler

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
)

// LogoutFlowHandler completes Hydra logout requests on behalf of the logout
// screen: it validates the requested post-logout redirect against the
// client's logout policy, accepts the request, and revokes the user's
// sessions so previously issued tokens stop working.
type LogoutFlowHandler struct {
	clients client.Service
	hydra   *hydra.Client
	logger  *zap.Logger
}

func NewLogoutFlowHandler(clients client.Service, hydraClient *hydra.Client, logger *zap.Logger) *LogoutFlowHandler {
	return &LogoutFlowHandler{clients: clients, hydra: hydraClient, logger: logger}
}

// HandleLogout serves GET /logout?logout_challenge=…[&post_logout_redirect_uri=…].
func (h *LogoutFlowHandler) HandleLogout(c *fiber.Ctx) error {
	challenge := c.Query("logout_challenge")
	if challenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "logout_challenge parameter is required",
		})
	}

	logoutReq, err := h.hydra.GetLogoutRequest(challenge)
	if err != nil {
		h.logger.Error("Failed to get logout request from Hydra", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	clientID := ""
	if logoutReq.Client != nil {
		clientID = logoutReq.Client.ClientID
	}
	if clientID == "" {
		return h.accept(c, challenge, "", logoutReq.Subject)
	}

	cfg, err := h.clients.GetByClientID(clientID)
	if err != nil {
		// Without the client's policy there is nothing to validate a
		// redirect against; finish the logout and let Hydra pick the target.
		h.logger.Warn("Logout for a client Authway does not know", zap.String("client_id", clientID), zap.Error(err))
		return h.accept(c, challenge, "", logoutReq.Subject)
	}

	requested := c.Query("post_logout_redirect_uri")
	target, err := resolveLogoutRedirect(cfg, requested)
	if err != nil {
		h.logger.Warn("Logout redirect rejected by client policy",
			zap.String("client_id", clientID), zap.String("policy", cfg.LogoutRedirectPolicy), zap.Error(err))
		// The fallback lets the logout screen still send the user back to
		// their application instead of stranding them on an error.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": err.Error(),
			"fallback_redirect": logoutFallbackURI(cfg, requested),
			"client_id":         clientID,
		})
	}
	return h.accept(c, challenge, target, logoutReq.Subject)
}

func (h *LogoutFlowHandler) accept(c *fiber.Ctx, challenge, postLogoutRedirectURI, subject string) error {
	resp, err := h.hydra.AcceptLogoutRequest(challenge, postLogoutRedirectURI)
	if err != nil {
		h.logger.Error("Failed to accept logout request", zap.Error(err))
		if errors.Is(err, hydra.ErrFlowNotFound) || errors.Is(err, hydra.ErrFlowExpired) {
			return respondFlowLookupError(c, err)
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to complete logout"})
	}

	// Accepting the logout only ends the browser's login session; tokens
	// issued before it stay valid. Revoke every session of the subject across
	// all clients so they stop working too. Best effort: a revocation failure
	// must not strand the user on this redirect (POST /api/v1/logout, whose
	// whole purpose is revocation, does fail on it).
	if subject != "" {
		if err := h.hydra.RevokeUserSessions(subject); err != nil {
			h.logger.Error("Failed to revoke user sessions during logout", zap.String("subject", subject), zap.Error(err))
		}
	}
	return c.Redirect(resp.RedirectTo, fiber.StatusFound)
}

// resolveLogoutRedirect applies the client's logout_redirect_policy to the
// requested post-logout redirect and returns the URI to hand Hydra.
//
//   - strict (default): a URI is required and must be on the whitelist.
//   - lenient: an unlisted or missing URI falls back to the client's default.
//   - disabled: the requested URI is passed through unchecked.
func resolveLogoutRedirect(c *client.Client, requested string) (string, error) {
	policy := c.LogoutRedirectPolicy
	if policy == "" {
		policy = "strict"
	}
	switch policy {
	case "disabled":
		return requested, nil
	case "lenient":
		if requested != "" && logoutURIAllowed(requested, c.PostLogoutRedirectURIs, c.AllowWildcardLogout) {
			return requested, nil
		}
		if c.DefaultLogoutURI != nil && *c.DefaultLogoutURI != "" {
			return *c.DefaultLogoutURI, nil
		}
		return c.Website, nil
	case "strict":
		if requested == "" {
			return "", fmt.Errorf("post_logout_redirect_uri is required (strict policy)")
		}
		if !logoutURIAllowed(requested, c.PostLogoutRedirectURIs, c.AllowWildcardLogout) {
			return "", fmt.Errorf("post_logout_redirect_uri is not whitelisted")
		}
		return requested, nil
	default:
		return "", fmt.Errorf("unknown logout_redirect_policy: %s", policy)
	}
}

// logoutFallbackURI picks where to send the user when their requested
// redirect was refused: the client's default logout URI, its first
// whitelisted URI, its website, or the origin of what they asked for.
func logoutFallbackURI(c *client.Client, requested string) string {
	if c.DefaultLogoutURI != nil && *c.DefaultLogoutURI != "" {
		return *c.DefaultLogoutURI
	}
	if len(c.PostLogoutRedirectURIs) > 0 {
		return c.PostLogoutRedirectURIs[0]
	}
	if c.Website != "" {
		return c.Website
	}
	if u, err := url.Parse(requested); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return ""
}

func logoutURIAllowed(uri string, whitelist []string, allowWildcard bool) bool {
	for _, allowed := range whitelist {
		if uri == allowed || (allowWildcard && matchesLogoutWildcard(uri, allowed)) {
			return true
		}
	}
	return false
}

// matchesLogoutWildcard matches "http(s)://localhost:*" and "*.example.com"
// patterns against the parsed host — never the raw string, so a query such as
// "?x=y.example.com" cannot satisfy "*.example.com".
func matchesLogoutWildcard(uri, pattern string) bool {
	if pattern == "" {
		return false
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Host == "" {
		return false
	}
	if pattern == "http://localhost:*" || pattern == "https://localhost:*" {
		return parsed.Scheme == strings.TrimSuffix(pattern, "://localhost:*") && parsed.Hostname() == "localhost"
	}
	if pattern[0] == '*' && len(pattern) > 1 {
		domain := strings.TrimPrefix(pattern[1:], ".")
		host := parsed.Hostname()
		return host == domain || strings.HasSuffix(host, "."+domain)
	}
	return false
}
