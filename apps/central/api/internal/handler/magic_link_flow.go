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
	flow := flowParam(c)
	var req sendMagicLinkRequest
	if err := c.BodyParser(&req); err != nil || !strings.Contains(req.Email, "@") {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "A valid email address is required.")
	}

	loginReq, err := h.auth.hydraClient.GetLoginRequest(flow)
	if err != nil {
		return respondFlowLookupError(c, err)
	}
	cl, err := h.auth.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.auth.logger.Error("OAuth client is not registered in Authway", zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "client_not_registered", "This application is not registered with Authway.")
	}
	if !cl.AllowsSignInMethod("magic_link") {
		return respondSignInMethodNotAllowed(c)
	}

	// From here a request may send an email, so each one counts against the
	// limit — whether or not the address gets one, or the answer would tell.
	// A request refused above sends nothing.
	middleware.IncrementRateLimitOnFailure(c)

	if _, err := h.links.SendMagicLink(cl.TenantID, strings.TrimSpace(req.Email), flow, c.IP(), c.Get("User-Agent")); err != nil {
		h.auth.logger.Error("Failed to issue magic link", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The sign-in link could not be issued. Try again shortly.")
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
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "token is required")
	}
	link, err := h.links.InspectMagicLink(req.Token)
	if err != nil || link.LoginFlow == "" {
		return c.JSON(fiber.Map{"valid": false, "detail": apierror.Message(err, "This sign-in link is not valid.")})
	}
	// The flow lets the page tell whether it is the browser that started the
	// sign-in — only that one can finish it with the link; another one shows
	// the code to type there instead.
	return c.JSON(fiber.Map{"valid": true, "email": link.Email, "expires_at": link.ExpiresAt, "flow": link.LoginFlow})
}

// Redeem uses the link up and continues its login flow: signed in, or — for
// a user with a second factor — on to that step.
// POST /api/v1/magic-links/redeem
func (h *MagicLinkFlowHandler) Redeem(c *fiber.Ctx) error {
	var req magicLinkTokenRequest
	if err := c.BodyParser(&req); err != nil || req.Token == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "token is required")
	}
	link, u, err := h.links.VerifyMagicLink(req.Token)
	if err != nil {
		h.auth.logger.Warn("Magic link not redeemed", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_link", apierror.Message(err, "This sign-in link is not valid."))
	}
	if link.LoginFlow == "" {
		// Issued before links were tied to a login flow; there is nothing to sign in to.
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_link", "This sign-in link is no longer valid. Start again from the application.")
	}
	return h.auth.firstFactorPassed(c, link.LoginFlow, u, byEmailedLink, false)
}

type magicLinkCodeRequest struct {
	Code string `json:"code"`
}

// RedeemCode does what Redeem does, for the code the email carries — typed
// into the screen that started the sign-in, when the email was read on
// another device than that screen.
// POST /api/v1/login-flows/:flow/magic-link/code
func (h *MagicLinkFlowHandler) RedeemCode(c *fiber.Ctx) error {
	flow := flowParam(c)
	var req magicLinkCodeRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "code is required")
	}
	link, u, err := h.links.VerifyMagicLinkCode(flow, req.Code)
	if err != nil {
		middleware.IncrementRateLimitOnFailure(c)
		h.auth.logger.Warn("Sign-in code not redeemed", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_code", apierror.Message(err, "This code is not valid."))
	}
	return h.auth.firstFactorPassed(c, link.LoginFlow, u, byEmailedLink, false)
}
