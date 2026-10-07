package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"authway/apps/central/api/internal/hydra"
)

// respondFlowLookupError answers a failed call on a login, consent or logout
// flow. The challenge comes from the caller, so an unknown or spent one
// is a client error; only a failure to get an answer from Hydra at all is
// reported as an upstream error. Internal addresses and raw Hydra errors are
// logged by the caller, never returned.
func respondFlowLookupError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, hydra.ErrFlowNotFound):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This sign-in link is not valid. Start again from the application.",
			"code":  "invalid_flow",
		})
	case errors.Is(err, hydra.ErrFlowExpired):
		return c.Status(fiber.StatusGone).JSON(fiber.Map{
			"error": "This sign-in has expired or was already completed. Start again from the application.",
			"code":  "flow_expired",
		})
	default:
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "The authorization server is unavailable. Try again shortly.",
			"code":  "authorization_server_unavailable",
		})
	}
}
