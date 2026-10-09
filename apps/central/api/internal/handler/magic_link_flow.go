package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/passwordless"
)

// nextEmailSent tells the sign-in screen a link is on its way.
const nextEmailSent = "email_sent"

// MagicLinkFlowHandler signs users in to a login flow with an emailed link.
// The link must be opened in the browser that started the sign-in: the
// authorization server ties the flow to that browser.
type MagicLinkFlowHandler struct {
	auth  *AuthHandler
	links passwordless.Service
}

func NewMagicLinkFlowHandler(auth *AuthHandler, links passwordless.Service) *MagicLinkFlowHandler {
	return &MagicLinkFlowHandler{auth: auth, links: links}
}

type sendMagicLinkRequest struct {
	Email string `json:"email"`
}

// Send emails a sign-in link for the flow. The tenant comes from the flow's
// client; the caller cannot choose it. An address that may not sign in gets
// the same answer as one that may, and no link.
// POST /api/v1/login-flows/:flow/magic-link
func (h *MagicLinkFlowHandler) Send(c *fiber.Ctx) error {
	// Each request may send an email, so each one counts against the limit.
	middleware.IncrementRateLimitOnFailure(c)

	flow := flowParam(c)
	var req sendMagicLinkRequest
	if err := c.BodyParser(&req); err != nil || !strings.Contains(req.Email, "@") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "A valid email address is required.", "code": "invalid_request"})
	}

	loginReq, err := h.auth.hydraClient.GetLoginRequest(flow)
	if err != nil {
		return respondFlowLookupError(c, err)
	}
	cl, err := h.auth.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.auth.logger.Error("OAuth client is not registered in Authway", zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This application is not registered with Authway.",
			"code":  "client_not_registered",
		})
	}
	if !cl.AllowsSignInMethod("magic_link") {
		return respondSignInMethodNotAllowed(c)
	}

	if _, err := h.links.SendMagicLink(cl.TenantID, strings.TrimSpace(req.Email), flow, c.IP(), c.Get("User-Agent")); err != nil {
		h.auth.logger.Error("Failed to issue magic link", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "The sign-in link could not be issued. Try again shortly.",
			"code":  "internal_error",
		})
	}
	return c.JSON(fiber.Map{"next": nextEmailSent})
}

type magicLinkTokenRequest struct {
	Token string `json:"token"`
}

// Inspect reports whether a link can still be redeemed, without redeeming it
// — the landing page asks before the user confirms, so a mail scanner that
// opens the page does not use the link up.
// POST /api/v1/magic-links/inspect
func (h *MagicLinkFlowHandler) Inspect(c *fiber.Ctx) error {
	var req magicLinkTokenRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required", "code": "invalid_request"})
	}
	link, err := h.links.InspectMagicLink(req.Token)
	if err != nil || link.LoginFlow == "" {
		return c.JSON(fiber.Map{"valid": false, "error": apierror.Message(err, "This sign-in link is not valid.")})
	}
	return c.JSON(fiber.Map{"valid": true, "email": link.Email, "expires_at": link.ExpiresAt})
}

// Redeem uses the link up and completes its login flow.
// POST /api/v1/magic-links/redeem
func (h *MagicLinkFlowHandler) Redeem(c *fiber.Ctx) error {
	var req magicLinkTokenRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required", "code": "invalid_request"})
	}
	link, u, err := h.links.VerifyMagicLink(req.Token)
	if err != nil {
		h.auth.logger.Warn("Magic link not redeemed", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": apierror.Message(err, "This sign-in link is not valid."),
			"code":  "invalid_link",
		})
	}
	if link.LoginFlow == "" {
		// Issued before links were tied to a login flow; there is nothing to sign in to.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This sign-in link is no longer valid. Start again from the application.",
			"code":  "invalid_link",
		})
	}
	return h.auth.completeLogin(c, link.LoginFlow, u, false, 0, "magic_link")
}
