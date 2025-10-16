package telemetry

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// RequestTracking creates a middleware that tracks all HTTP requests to Application Insights
func RequestTracking(telemetryClient *Client, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip if telemetry is not enabled
		if telemetryClient == nil || !telemetryClient.IsEnabled() {
			return c.Next()
		}

		start := time.Now()

		// Process request
		err := c.Next()

		// Calculate request duration
		duration := time.Since(start)

		// Get request details
		method := c.Method()
		path := c.Path()
		statusCode := c.Response().StatusCode()
		success := statusCode < 400

		// Track request to Application Insights
		requestName := fmt.Sprintf("%s %s", method, path)
		requestURL := fmt.Sprintf("%s://%s%s", c.Protocol(), c.Hostname(), path)
		responseCode := fmt.Sprintf("%d", statusCode)

		telemetryClient.TrackRequest(requestName, requestURL, duration, responseCode, success)

		// Track exceptions if request failed with error
		if err != nil {
			telemetryClient.TrackException(err)
			logger.Error("Request failed",
				zap.String("method", method),
				zap.String("path", path),
				zap.Int("status", statusCode),
				zap.Duration("duration", duration),
				zap.Error(err))
		}

		return err
	}
}
