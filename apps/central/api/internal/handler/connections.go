package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/user"
)

// ListConnections names the sign-in providers the signed-in user's account
// is linked to.
// GET /api/v1/users/connections
func (h *AuthHandler) ListConnections(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to see your account.")
	}
	u, err := h.userService.GetByID(userID)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Account not found.")
	}
	connections := []fiber.Map{}
	for _, p := range u.LinkedProviders() {
		connections = append(connections, fiber.Map{"provider": p})
	}
	return c.JSON(fiber.Map{"connections": connections})
}

// RemoveConnection ends the signed-in user's link to a provider: that
// provider's account no longer signs in to this one — the way out of a link
// the user did not mean, and of "linked to another account" when signing in
// with a different account at that provider.
// DELETE /api/v1/users/connections/:provider
//
// It needs a recent sign-in, like the other changes to how an account signs
// in, and it does not take away the account's only way to sign in.
func (h *AuthHandler) RemoveConnection(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to change your account.")
	}
	if !signedInRecently(c) {
		return refuseStaleSignIn(c, "Sign in again to remove a connection.")
	}
	provider := c.Params("provider")
	if err := h.userService.UnlinkProvider(userID, provider); err != nil {
		switch {
		case errors.Is(err, user.ErrNotLinked):
			return apierror.Refuse(c, fiber.StatusNotFound, "not_connected", "The account is not connected to this provider.")
		case errors.Is(err, user.ErrLastSignInMethod):
			return apierror.Refuse(c, fiber.StatusConflict, "last_sign_in_method", "This is the only way the account signs in. Set a password first.")
		case errors.Is(err, user.ErrUserNotFound):
			return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Account not found.")
		}
		h.logger.Error("Failed to unlink a sign-in provider", zap.String("user_id", userID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The connection could not be removed. Try again shortly.")
	}
	if u, err := h.userService.GetByID(userID); err == nil {
		h.logUserAudit(c, u, audit.ActionUserConnectionRemoved, map[string]any{"provider": provider})
	}
	return c.JSON(fiber.Map{"message": "Connection removed"})
}
