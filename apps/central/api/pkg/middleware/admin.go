package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminAuth creates a middleware that validates Admin API Key.
//
// Deprecated (0.3.0): Prefer admin.Handler.GetAdminConsoleAuth() — it accepts
// EITHER the long-lived API key OR an admin session token issued by
// /admin/login (used by the Admin Console UI). main.go now wires every
// admin route through GetAdminConsoleAuth so a single auth surface is
// maintained. This function is preserved for back-compat with any external
// importers but new code should not call it.
//
// Fail-closed: if apiKey is empty, every request is rejected with 503.
// Callers MUST ensure the admin key is configured before starting the server
// in any non-development environment — see config.Load() validation.
func AdminAuth(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Fail-closed: misconfigured admin key must never silently grant access.
		if apiKey == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Admin API is not configured (missing ADMIN_API_KEY)",
			})
		}

		// Get Authorization header
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing Authorization header",
			})
		}

		// Check Bearer token format
		if !strings.HasPrefix(auth, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid Authorization header format. Expected: Bearer <token>",
			})
		}

		// Extract token
		token := strings.TrimPrefix(auth, "Bearer ")

		// Constant-time compare to defend against timing oracles.
		if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}

		// Set admin flag in context
		c.Locals("isAdmin", true)

		return c.Next()
	}
}

// RequireAdmin middleware checks if request has admin privileges
// Use this after AdminAuth middleware
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		isAdmin := c.Locals("isAdmin")
		if isAdmin == nil || !isAdmin.(bool) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Admin privileges required",
			})
		}
		return c.Next()
	}
}
