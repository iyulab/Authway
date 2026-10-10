package handler

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/user"
)

// recentAuthentication is how long ago a user may have signed in and still
// delete their own account. A longer-lived or stolen session must sign in
// again first.
const recentAuthentication = 10 * time.Minute

// removeAccount ends everything the authorization server holds for the user —
// sign-in sessions, consents and the tokens issued under them — and then
// deletes the account. The order matters: an account that still exists after
// a failure can be removed again, while tokens outliving a deleted account
// could not be revoked by anyone looking for that account.
func removeAccount(hc *hydra.Client, users user.Service, id uuid.UUID) error {
	if hc != nil {
		if err := hc.RevokeUserSessions(id.String()); err != nil {
			return fmt.Errorf("revoke the user's sessions: %w", err)
		}
	}
	return users.Delete(id)
}

// DeleteMe deletes the signed-in user's own account.
// DELETE /api/v1/profile/me
//
// The access token must come from a session whose user signed in within
// recentAuthentication. Otherwise the answer is the step-up challenge of
// RFC 9470: 401 with WWW-Authenticate naming insufficient_user_authentication
// and the max_age to request, so the application sends the user through
// sign-in again and retries with the new token.
func (h *AuthHandler) DeleteMe(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to delete your account.")
	}
	if authTime, _ := c.Locals("auth_time").(time.Time); authTime.IsZero() || time.Since(authTime) > recentAuthentication {
		maxAge := strconv.Itoa(int(recentAuthentication.Seconds()))
		c.Set(fiber.HeaderWWWAuthenticate,
			`Bearer error="insufficient_user_authentication", error_description="A more recent sign-in is required", max_age="`+maxAge+`"`)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "Sign in again to delete your account.",
			"code":    "insufficient_user_authentication",
			"max_age": int(recentAuthentication.Seconds()),
		})
	}

	u, err := h.userService.GetByID(userID)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Account not found.")
	}
	if err := removeAccount(h.hydraClient, h.userService, u.ID); err != nil {
		h.logger.Error("Failed to delete an account at its owner's request", zap.String("user_id", u.ID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadGateway, "authorization_server_unavailable", "The account could not be deleted. Try again shortly.")
	}
	h.logUserAudit(c, u, audit.ActionUserDeleted, map[string]any{"self": true})
	return c.JSON(fiber.Map{"message": "Account deleted"})
}
