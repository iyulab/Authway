package handler

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// LogoutFlowHandler completes logout flows for the logout screen.
//
// Where the browser lands afterwards is decided by the authorization server:
// on RP-initiated logout it only starts a logout flow after checking the
// requested post_logout_redirect_uri against the URIs registered for the
// client, and it sends the browser there once the flow is accepted.
type LogoutFlowHandler struct {
	hydra  *hydra.Client
	logger *zap.Logger
}

func NewLogoutFlowHandler(hydraClient *hydra.Client, logger *zap.Logger) *LogoutFlowHandler {
	return &LogoutFlowHandler{hydra: hydraClient, logger: logger}
}

// CompleteLogout accepts a logout flow and revokes the user's sessions so
// previously issued tokens stop working too. It answers
// {"next":"redirect","redirect_to":…}.
// POST /api/v1/logout-flows/:flow
func (h *LogoutFlowHandler) CompleteLogout(c *fiber.Ctx) error {
	flow := flowParam(c)

	logoutReq, err := h.hydra.GetLogoutRequest(flow)
	if err != nil {
		h.logger.Warn("Failed to get logout request from Hydra", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	resp, err := h.hydra.AcceptLogoutRequest(flow)
	if err != nil {
		h.logger.Error("Failed to accept logout request", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	// Accepting the logout only ends the browser's login session; tokens
	// issued before it stay valid. Revoke every session of the subject across
	// all clients so they stop working too. Best effort: a revocation failure
	// must not strand the user on the logout screen (POST /api/v1/logout,
	// whose whole purpose is revocation, does fail on it).
	if logoutReq.Subject != "" {
		if err := h.hydra.RevokeUserSessions(logoutReq.Subject); err != nil {
			h.logger.Error("Failed to revoke user sessions during logout", zap.String("subject", logoutReq.Subject), zap.Error(err))
		}
	}
	return c.JSON(fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo})
}
