package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminAuth creates a middleware that validates Admin API Key
// Usage: app.Use("/api/v1/admin", middleware.AdminAuth(config.AdminAPIKey))
func AdminAuth(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip if no API key configured (development mode)
		if apiKey == "" {
			return c.Next()
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

		// Validate token
		if token != apiKey {
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
