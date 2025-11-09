package middleware

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// InternalAPIAuth creates a middleware that validates X-API-Key header
func InternalAPIAuth(apiKey string, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get X-API-Key header
		providedKey := c.Get("X-API-Key")

		if providedKey == "" {
			logger.Warn("Missing X-API-Key header",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()))
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing X-API-Key header",
			})
		}

		if providedKey != apiKey {
			logger.Warn("Invalid X-API-Key",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()),
				zap.String("provided_key", providedKey),
				zap.String("expected_key", apiKey),
				zap.Int("provided_len", len(providedKey)),
				zap.Int("expected_len", len(apiKey)))
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}

		// API key is valid, continue
		return c.Next()
	}
}
