package handler

import (
	"authway/apps/central/api/pkg/apierror"
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
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_flow", "This sign-in link is not valid. Start again from the application.")
	case errors.Is(err, hydra.ErrFlowExpired):
		return apierror.Refuse(c, fiber.StatusGone, "flow_expired", "This sign-in has expired or was already completed. Start again from the application.")
	default:
		return apierror.Refuse(c, fiber.StatusBadGateway, "authorization_server_unavailable", "The authorization server is unavailable. Try again shortly.")
	}
}
