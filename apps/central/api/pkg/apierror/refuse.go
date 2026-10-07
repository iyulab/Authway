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
