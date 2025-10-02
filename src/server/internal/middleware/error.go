package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// ErrorHandler handles all errors in a consistent format
func ErrorHandler(c *fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(ErrorResponse{
			Error:   "request_error",
			Message: fiberErr.Message,
			Code:    fiberErr.Code,
		})
	}

	// Log unexpected errors
	if logger, ok := c.Locals("logger").(*zap.Logger); ok {
		logger.Error("Unexpected error",
			zap.Error(err),
			zap.String("path", c.Path()),
			zap.String("method", c.Method()),
		)
	}

	// Default internal server error
	return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
		Error:   "internal_server_error",
		Message: "An unexpected error occurred",
		Code:    fiber.StatusInternalServerError,
	})
}

// RequestLogger middleware adds logger to context
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("logger", logger)
		return c.Next()
	}
}
