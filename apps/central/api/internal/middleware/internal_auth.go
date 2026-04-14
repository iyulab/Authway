package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// InternalAPIAuth creates a middleware that validates the X-API-Key header
// against the configured internal API key.
//
// Fail-closed: if apiKey is empty the middleware refuses every request (503).
// Constant-time compare defends against timing oracles. Neither the expected
// nor the provided key is logged — the previous implementation logged
// `expected_key` (the actual configured secret) on every failed attempt,
// turning the audit log into a credential disclosure surface.
func InternalAPIAuth(apiKey string, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if apiKey == "" {
			logger.Warn("InternalAPIAuth: refusing request — internal API key not configured",
				zap.String("path", c.Path()))
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Internal API is not configured",
			})
		}

		providedKey := c.Get("X-API-Key")
		if providedKey == "" {
			logger.Warn("Missing X-API-Key header",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()))
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing X-API-Key header",
			})
		}

		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
			logger.Warn("Invalid X-API-Key",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()))
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}

		return c.Next()
	}
}
