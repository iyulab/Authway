package middleware

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ErrorHandler answers errors no handler turned into a response.
func ErrorHandler(c *fiber.Ctx, err error) error {
	var refusal *apierror.Refusal
	if errors.As(err, &refusal) {
		return apierror.RefuseWith(c, refusal.Status, refusal.Code, refusal.Message, refusal.Extensions)
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return apierror.Refuse(c, fiberErr.Code, apierror.CodeForStatus(fiberErr.Code), fiberErr.Message)
	}

	// Log unexpected errors
	if logger, ok := c.Locals("logger").(*zap.Logger); ok {
		logger.Error("Unexpected error",
			zap.Error(err),
			zap.String("path", c.Path()),
			zap.String("method", c.Method()),
		)
	}

	return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "An unexpected error occurred")
}

// RequestLogger middleware adds logger to context
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("logger", logger)
		return c.Next()
	}
}
