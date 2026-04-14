package admin

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type Handler struct {
	service Service
	logger  *zap.Logger
	version string
	apiKey  string // Empty = dev mode (skip auth for admin console)
}

func NewHandler(service Service, logger *zap.Logger, version string, apiKey string) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
		version: version,
		apiKey:  apiKey,
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
		// Fail-closed: missing key indicates misconfiguration.
		if h.apiKey == "" {
			h.logger.Warn("AdminConsoleAuth: refusing request — admin API key not configured",
				zap.String("path", c.Path()),
			)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Admin API is not configured (missing ADMIN_API_KEY)",
			})
		}

		token := h.extractToken(c)
		if token == "" {
			h.logger.Warn("AdminConsoleAuth: No token provided",
				zap.String("path", c.Path()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No authorization token provided",
			})
		}

		// Programmatic auth: long-lived API key match (constant-time).
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.apiKey)) == 1 {
			c.Locals("admin_authenticated", true)
			c.Locals("is_admin_console", true)
			c.Locals("auth_method", "api_key")

			// Extract tenant_id from query parameter or header
			tenantID := c.Query("tenant_id")
			if tenantID == "" {
				tenantID = c.Get("X-Tenant-ID")
			}
			if tenantID != "" {
				c.Locals("tenant_id", tenantID)
			}

			return c.Next()
		}

		// Session-token auth: Admin Console UI login.
		valid, err := h.service.ValidateToken(token)
		if err != nil {
			h.logger.Error("Failed to validate admin token", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to validate session",
			})
		}

		if !valid {
			h.logger.Warn("AdminConsoleAuth: Token invalid or expired",
				zap.String("path", c.Path()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired session",
			})
		}
		c.Locals("auth_method", "session")

		// Extract tenant_id from query parameter or header
		tenantID := c.Query("tenant_id")
		if tenantID == "" {
			tenantID = c.Get("X-Tenant-ID")
		}
		if tenantID != "" {
			c.Locals("tenant_id", tenantID)
		}

		c.Locals("admin_authenticated", true)
		c.Locals("is_admin_console", true)
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
