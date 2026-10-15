package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/user"
)

// ChangePassword sets a new password for the signed-in user.
// POST /api/v1/users/password
//
// An account with a password proves it with current_password — a stolen
// token alone cannot take the account over. An account without one (it signs
// in by link or by another provider) sets its first with a recent sign-in
// instead (refuseStaleSignIn).
//
// Sessions opened with the old password end, with the tokens issued in them;
// this one stays. A token that does not say which session it came from
// cannot be told apart, so then every session ends — sessions_ended says
// which, as turning on a second factor does.
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to change your password.")
	}
	var req user.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.NewPassword) == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "new_password is required")
	}
	u, err := h.userService.GetByID(userID)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Account not found.")
	}
	if u.PasswordHash == "" && !signedInRecently(c) {
		return refuseStaleSignIn(c, "Sign in again to set a password.")
	}

	if err := h.userService.ChangePassword(userID, &req); err != nil {
		var refused *user.PasswordError
		switch {
		case errors.Is(err, user.ErrWrongPassword):
			middleware.IncrementRateLimitOnFailure(c)
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_current_password", "The current password is not correct.")
		case errors.As(err, &refused):
			return apierror.RefuseWith(c, fiber.StatusBadRequest, refused.Code(), refused.Error(), fiber.Map{"min_length": refused.MinLength})
		}
		h.logger.Error("Failed to change a password", zap.String("user_id", userID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The password could not be changed. Try again shortly.")
	}

	keep, _ := c.Locals("session_id").(string)
	ended := "others"
	if keep == "" {
		ended = "all"
	}
	if h.hydraClient != nil {
		if err := h.hydraClient.RevokeOtherSessions(userID.String(), keep); err != nil && !errors.Is(err, hydra.ErrApplicationsNotNotified) {
			// The password is changed either way; say what did not happen.
			h.logger.Error("Password changed, but the user's other sessions could not be ended",
				zap.String("user_id", userID.String()), zap.Error(err))
			ended = "none"
		}
	}
	middleware.ResetRateLimitOnSuccess(c)
	h.logUserAudit(c, u, audit.ActionUserPasswordChanged, map[string]any{"sessions_ended": ended, "first_password": u.PasswordHash == ""})
	return c.JSON(fiber.Map{"message": "Password changed", "sessions_ended": ended})
}
