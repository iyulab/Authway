package apierror

import "github.com/gofiber/fiber/v2"

// Refuse writes the API's error shape, {error, code}: error is a message for a
// person, code a stable identifier a caller can branch on. Refusals that carry
// nothing beyond a status can instead return fiber.NewError, which the
// framework error handler answers in the same shape with a code named after
// the status.
func Refuse(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message, "code": code})
}

// Refusal is a refusal carried as an error, for code that decides to refuse
// before it holds the response: a handler returns it unchanged and the
// framework error handler writes it as {error, code} with its status.
type Refusal struct {
	Status  int
	Code    string
	Message string
}

func (r *Refusal) Error() string { return r.Message }

// Reject builds a Refusal.
func Reject(status int, code, message string) *Refusal {
	return &Refusal{Status: status, Code: code, Message: message}
}
