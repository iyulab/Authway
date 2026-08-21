package admin

import (
	"crypto/subtle"
	"strings"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tokenhash"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// apiKeyHint returns the first 8 hex chars of SHA-256(key) — a stable,
// non-reversible identifier for audit logs so operators can distinguish
// which provisioned key performed an action without ever storing the key.
func apiKeyHint(key string) string {
	return tokenhash.Hash(key)[:8]
}

type Handler struct {
	service      Service
	logger       *zap.Logger
	version      string
	apiKey       string // Empty = dev mode (skip auth for admin console)
	auditService audit.Service
}

// NewHandler builds the admin Handler. auditService may be nil — the handler
// tolerates missing audit wiring so tests without the full DI graph still
// compile. In prod, main.go passes the live audit.Service so every
// authentication failure is captured.
func NewHandler(service Service, logger *zap.Logger, version string, apiKey string, auditService audit.Service) *Handler {
	return &Handler{
		service:      service,
		logger:       logger,
		version:      version,
		apiKey:       apiKey,
		auditService: auditService,
	}
}

// logAuthFailure records an authentication failure synchronously. Failure
// events must not be dropped on buffer overflow, so we use Log (not LogAsync).
// Tenant is unknown at this point (auth has not succeeded), so we write
// uuid.Nil and stash the *attempted* tenant from the X-Tenant-ID header /
// query param into Details for operator forensics.
func (h *Handler) logAuthFailure(c *fiber.Ctx, reason string, errMsg string) {
	if h.auditService == nil {
		return
	}
	tenantAttempted := c.Query("tenant_id")
	if tenantAttempted == "" {
		tenantAttempted = c.Get("X-Tenant-ID")
	}

	entry := &audit.AuditEntry{
		TenantID:     uuid.Nil,
		Action:       audit.ActionUserLoginFailed,
		Severity:     audit.SeverityWarning,
		ResourceType: "admin_console",
		ResourceID:   c.Path(),
		IPAddress:    c.IP(),
		UserAgent:    c.Get("User-Agent"),
		Success:      false,
		ErrorMsg:     errMsg,
		Details: map[string]any{
			"reason": reason,
			"method": c.Method(),
		},
	}
	if tenantAttempted != "" {
		entry.Details["tenant_id_attempted"] = tenantAttempted
	}
	if err := h.auditService.Log(c.UserContext(), entry); err != nil {
		h.logger.Error("Failed to write admin auth failure audit log",
			zap.Error(err),
			zap.String("reason", reason),
		)
	}
}

// RegisterRoutes registers admin console routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
	// Public routes (no auth required)
	admin := app.Group("/admin")
	admin.Post("/login", h.Login)
	admin.Get("/info", h.Info)

	// Protected routes (admin session required)
	admin.Post("/logout", h.AdminAuthMiddleware(), h.Logout)
	admin.Get("/validate", h.AdminAuthMiddleware(), h.Validate)
}

// Login authenticates admin and returns session token
func (h *Handler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password is required",
		})
	}

	session, err := h.service.Authenticate(req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid password",
		})
	}

	if h.auditService != nil {
		h.auditService.LogAsync(&audit.AuditEntry{
			TenantID:     uuid.Nil,
			Action:       audit.ActionAdminLoginSuccess,
			Severity:     audit.SeverityInfo,
			ResourceType: "admin_session",
			ResourceID:   session.ID.String(),
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
			Success:      true,
		})
	}

	return c.JSON(LoginResponse{
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
	})
}

// Logout terminates admin session
func (h *Handler) Logout(c *fiber.Ctx) error {
	token := h.extractToken(c)
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "No token provided",
		})
	}

	if err := h.service.Logout(token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	if h.auditService != nil {
		h.auditService.LogAsync(&audit.AuditEntry{
			TenantID:     uuid.Nil,
			Action:       audit.ActionAdminLogout,
			Severity:     audit.SeverityInfo,
			ResourceType: "admin_session",
			ResourceID:   apiKeyHint(token),
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
			Success:      true,
		})
	}

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// Validate checks if current session is valid
func (h *Handler) Validate(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"valid": true,
		"info": AdminInfo{
			Authenticated: true,
			Version:       h.version,
		},
	})
}

// Info returns admin console information (public)
func (h *Handler) Info(c *fiber.Ctx) error {
	return c.JSON(AdminInfo{
		Authenticated: false,
		Version:       h.version,
	})
}

// AdminAuthMiddleware validates admin session token
func (h *Handler) AdminAuthMiddleware() fiber.Handler {
	return h.createAdminAuthHandler()
}

// GetAdminSessionAuth returns the admin session authentication middleware
// for use by other route handlers that need admin session validation
func (h *Handler) GetAdminSessionAuth() fiber.Handler {
	return h.createAdminAuthHandler()
}

// createAdminAuthHandler creates the admin session validation handler
func (h *Handler) createAdminAuthHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := h.extractToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No authorization token provided",
			})
		}

		valid, err := h.service.ValidateToken(token)
		if err != nil {
			h.logger.Error("Failed to validate admin token", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to validate session",
			})
		}

		if !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired session",
			})
		}

		c.Locals("admin_authenticated", true)
		return c.Next()
	}
}

// GetAdminConsoleAuth returns a middleware for Admin Console API endpoints.
//
// Accepts EITHER:
//   - an admin session token (issued by /admin/login) — used by the Admin
//     Console UI, or
//   - the long-lived AUTHWAY_ADMIN_API_KEY — used by programmatic admin
//     scripts (curl, CI, integrations).
//
// Fail-closed: when the API key is unset the middleware refuses every
// request (503). Operators must set AUTHWAY_ADMIN_API_KEY in any
// non-development environment — this is enforced at config validation.
func (h *Handler) GetAdminConsoleAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.apiKey == "" {
			h.logger.Warn("AdminConsoleAuth: refusing request — admin API key not configured",
				zap.String("path", c.Path()))
			h.logAuthFailure(c, "api_key_not_configured", "admin API key missing — fail-closed")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Admin API is not configured (missing ADMIN_API_KEY)",
			})
		}

		token := h.extractToken(c)
		if token == "" {
			h.logger.Warn("AdminConsoleAuth: No token provided", zap.String("path", c.Path()))
			h.logAuthFailure(c, "no_token", "missing or malformed Authorization header")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No authorization token provided",
			})
		}

		ok, sessionErr := h.checkAdminAuth(c, token)
		if sessionErr != nil {
			h.logger.Error("Failed to validate admin token", zap.Error(sessionErr))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to validate session",
			})
		}
		if !ok {
			h.logger.Warn("AdminConsoleAuth: Token invalid or expired", zap.String("path", c.Path()))
			h.logAuthFailure(c, "invalid_or_expired_session", "token rejected by session validator (also covers api_key mismatch)")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired session",
			})
		}
		return c.Next()
	}
}

// checkAdminAuth attempts admin API-key or session-token auth for token,
// WITHOUT writing any response — callers own the response for both the
// failure and success case, so this can be composed into GetClientAuth's
// three-way fallback as well as GetAdminConsoleAuth's two-way check. ok=true
// means c.Locals was already populated (admin_authenticated, is_admin_console,
// auth_method, actor_type, and tenant_id if present in the request) and it is
// safe for the caller to call c.Next(). A non-nil error means the session
// validator itself failed (infra error, not "invalid token") — callers must
// surface that as 500, not 401.
func (h *Handler) checkAdminAuth(c *fiber.Ctx, token string) (ok bool, err error) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(h.apiKey)) == 1 {
		c.Locals("admin_authenticated", true)
		c.Locals("is_admin_console", true)
		c.Locals("auth_method", "api_key")
		c.Locals("actor_type", "api_key")
		c.Locals("actor_key_hint", apiKeyHint(h.apiKey))
		h.setTenantIDLocal(c)
		return true, nil
	}

	valid, err := h.service.ValidateToken(token)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, nil
	}
	c.Locals("auth_method", "session")
	c.Locals("actor_type", "admin_session")
	h.setTenantIDLocal(c)
	c.Locals("admin_authenticated", true)
	c.Locals("is_admin_console", true)
	return true, nil
}

func (h *Handler) setTenantIDLocal(c *fiber.Ctx) {
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		tenantID = c.Get("X-Tenant-ID")
	}
	if tenantID != "" {
		c.Locals("tenant_id", tenantID)
	}
}

// GetClientAuth returns middleware for routes that accept EITHER admin auth
// (API key / session — see GetAdminConsoleAuth) OR a scoped service_client
// credential validated via Hydra token introspection. requiredScope gates
// which scope a service_client's granted_scopes must include (e.g.
// "admin.clients:write"); the admin-auth legs are exempt from this check —
// an admin caller already has full access.
//
// Auth is attempted cheap-to-expensive: constant-time API-key compare,
// then admin session lookup, then (only if both miss) a Hydra introspection
// call — so the common admin-caller path costs nothing extra.
func (h *Handler) GetClientAuth(hydraClient *hydra.Client, svc serviceclient.Service, requiredScope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.apiKey == "" {
			h.logger.Warn("GetClientAuth: refusing request — admin API key not configured",
				zap.String("path", c.Path()))
			h.logAuthFailure(c, "api_key_not_configured", "admin API key missing — fail-closed")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Admin API is not configured (missing ADMIN_API_KEY)",
			})
		}

		token := h.extractToken(c)
		if token == "" {
			h.logAuthFailure(c, "no_token", "missing or malformed Authorization header")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No authorization token provided",
			})
		}

		if ok, err := h.checkAdminAuth(c, token); err != nil {
			h.logger.Error("Failed to validate admin token", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate session"})
		} else if ok {
			return c.Next()
		}

		introspectResp, err := hydraClient.IntrospectToken(token)
		if err != nil || !introspectResp.Active || introspectResp.ClientID == "" || introspectResp.ClientID != introspectResp.Subject {
			h.logAuthFailure(c, "invalid_credential", "token rejected by api key, admin session, and service-client introspection")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired session"})
		}

		sc, err := svc.GetByHydraClientID(introspectResp.ClientID)
		if err != nil || sc.IsRevoked() {
			h.logAuthFailure(c, "unknown_or_revoked_service_client", "hydra_client_id not found in service_clients, or revoked")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired session"})
		}
		if !sc.HasScope(requiredScope) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Insufficient scope"})
		}

		c.Locals("admin_authenticated", false)
		c.Locals("is_admin_console", false)
		c.Locals("auth_method", "service_client")
		c.Locals("actor_type", "service_client")
		c.Locals("tenant_id", sc.TenantID.String())
		return c.Next()
	}
}

// extractToken extracts bearer token from Authorization header
func (h *Handler) extractToken(c *fiber.Ctx) string {
	auth := c.Get("Authorization")
	if auth == "" {
		return ""
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}

	return strings.TrimPrefix(auth, "Bearer ")
}
