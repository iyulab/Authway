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

// codeForStatus names a refusal that carries only a status — one the framework
// produced (no route, wrong method, unreadable request) or a handler returned as
// fiber.NewError. A handler with a more specific reason answers with its own code.
func codeForStatus(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "invalid_request"
	case fiber.StatusUnauthorized:
		return "unauthorized"
	case fiber.StatusForbidden:
		return "forbidden"
	case fiber.StatusNotFound:
		return "not_found"
	case fiber.StatusMethodNotAllowed:
		return "method_not_allowed"
	case fiber.StatusConflict:
		return "conflict"
	case fiber.StatusRequestEntityTooLarge:
		return "request_too_large"
	case fiber.StatusTooManyRequests:
		return "too_many_requests"
	case fiber.StatusBadGateway:
		return "bad_gateway"
	case fiber.StatusServiceUnavailable:
		return "service_unavailable"
	}
	if status >= 500 {
		return "internal_server_error"
	}
	return "request_error"
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
