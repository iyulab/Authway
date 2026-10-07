package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ErrorResponse is the shape of every refusal: a message to show and a
// stable code.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// codeForStatus names the refusals the framework itself produces (no route,
// wrong method, unreadable request) when a handler did not answer first.
func codeForStatus(status int) string {
	switch status {
	case fiber.StatusNotFound:
		return "not_found"
	case fiber.StatusMethodNotAllowed:
		return "method_not_allowed"
	case fiber.StatusRequestEntityTooLarge:
		return "request_too_large"
	case fiber.StatusBadRequest:
		return "invalid_request"
	default:
		return "request_error"
	}
}

// ErrorHandler answers errors no handler turned into a response.
func ErrorHandler(c *fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(ErrorResponse{
			Error: fiberErr.Message,
			Code:  codeForStatus(fiberErr.Code),
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

	return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
		Error: "An unexpected error occurred",
		Code:  "internal_server_error",
	})
}

// RequestLogger middleware adds logger to context
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("logger", logger)
		return c.Next()
	}
}
