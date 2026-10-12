package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// GetLoginFlow tells the sign-in screen what to do with a login flow:
//
//   - {"next":"redirect","redirect_to":…} — no form is needed. Either the user
//     already has a session in the client's tenant and the flow was accepted,
//     or a stale session was cleared and the flow restarts.
//   - {"next":"form","flow":…,"client":{…}} — show the sign-in form.
//     client.sign_in_methods lists what to offer ("email" for the password
//     form, then social providers): what the client allows and this
//     deployment can actually run.
func (h *AuthHandler) GetLoginFlow(c *fiber.Ctx) error {
	flow := flowParam(c)

	loginReq, err := h.hydraClient.GetLoginRequest(flow)
	if err != nil {
		h.logger.Warn("Failed to get login request from Hydra", zap.Error(err))
		return respondFlowLookupError(c, err)
	}

	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("OAuth client is not registered in Authway",
			zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This application is not registered with Authway.",
			"code":  "client_not_registered",
		})
	}

	if step, settled, err := h.loginWithoutForm(c.Context(), flow, loginReq, requestedClient.TenantID); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "The authorization server is unavailable. Try again shortly.",
			"code":  "authorization_server_unavailable",
		})
	} else if settled {
		return c.JSON(step)
	}

	return c.JSON(fiber.Map{
		"next":            nextForm,
		"flow":            flow,
		"client_name":     loginReq.Client.ClientName,
		"requested_scope": loginReq.RequestedScope,
		"tenant_id":       requestedClient.TenantID.String(),
		"client": fiber.Map{
			"client_id":          requestedClient.ClientID,
			"sign_in_methods":    h.signInMethods.SignInMethodsFor(requestedClient),
			"allow_email_signup": requestedClient.AllowEmailSignup,
		},
	})
}

// LoginWithoutScreen settles a login flow at the moment the authorization
// server sends the browser over, when the browser's session already answers
// it (FlowSettler). The screen would be told the same thing if it were opened
// — it is not opened, so a reused session costs no page load.
func (h *AuthHandler) LoginWithoutScreen(c *fiber.Ctx, flow string) (string, bool) {
	loginReq, err := h.hydraClient.GetLoginRequest(flow)
	if err != nil || loginReq.Client == nil {
		return "", false
	}
	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		return "", false
	}
	step, settled, err := h.loginWithoutForm(c.Context(), flow, loginReq, requestedClient.TenantID)
	if err != nil || !settled {
		return "", false
	}
	redirectTo, _ := step["redirect_to"].(string)
	return redirectTo, redirectTo != ""
}

// loginWithoutForm settles a login flow that needs no form: the browser has a
// session this client can use, or the application asked for an answer without
// a prompt and there is no such session. It reports settled=false when the
// user has to sign in.
func (h *AuthHandler) loginWithoutForm(ctx context.Context, flow string, loginReq *hydra.LoginRequest, clientTenantID uuid.UUID) (fiber.Map, bool, error) {
	if !loginReq.Skip || loginReq.Subject == "" {
		return nil, false, nil
	}
	if step, settled, err := h.skipLoginForm(ctx, flow, loginReq, clientTenantID); err != nil || settled {
		return step, settled, err
	}
	if !asksForNoPrompt(loginReq.RequestURL) {
		return nil, false, nil
	}
	// The application asked for an answer without the user being shown
	// anything, and the browser's session is not one this client can use.
	// The answer is "sign-in required", not a form.
	resp, err := h.hydraClient.RejectLoginRequest(flow, "login_required", "The browser has no session for this application.")
	if err != nil {
		return nil, false, err
	}
	return fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo}, true, nil
}

// skipLoginForm handles a flow Hydra marks as skippable (the browser already
// has a session for loginReq.Subject). It reports handled=false when the form
// must be shown anyway — the session belongs to a different tenant than the
// client, so single sign-on does not apply.
func (h *AuthHandler) skipLoginForm(ctx context.Context, flow string, loginReq *hydra.LoginRequest, clientTenantID uuid.UUID) (fiber.Map, bool, error) {
	clearSession := func(reason string) (fiber.Map, bool, error) {
		h.logger.Warn("Clearing a session that no longer maps to a user",
			zap.String("subject", loginReq.Subject), zap.String("reason", reason))
		if err := h.hydraClient.RevokeUserSessions(loginReq.Subject); err != nil {
			h.logger.Error("Failed to revoke user sessions", zap.Error(err))
		}
		// login_required restarts the flow with a form instead of surfacing an
		// error to the OAuth client.
		resp, err := h.hydraClient.RejectLoginRequest(flow, "login_required", "Please login again")
		if err != nil {
			return nil, false, err
		}
		return fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo, "session_cleared": true}, true, nil
	}

	userID, err := uuid.Parse(loginReq.Subject)
	if err != nil {
		return clearSession("subject is not a user id")
	}
	sessionUser, err := h.userService.GetByID(userID)
	if err != nil {
		return clearSession("user not found")
	}
	if sessionUser.TenantID != clientTenantID {
		h.logger.Info("Session belongs to another tenant — showing the sign-in form",
			zap.String("user_tenant_id", sessionUser.TenantID.String()),
			zap.String("client_tenant_id", clientTenantID.String()))
		return nil, false, nil
	}

	h.logger.Info("SSO login", zap.String("user_id", sessionUser.ID.String()))

	// A reused session was authenticated earlier: its tokens say how it was
	// then, not that anything was proved now.
	var amr []string
	if earlier, ok := h.authentications.Lookup(loginReq.SessionID); ok {
		amr = earlier.Methods
	}

	// Reusing a session does not lengthen it: how long it lasts was decided
	// when the user signed in (the authorization server ignores a lifetime
	// given here).
	resp, err := h.hydraClient.AcceptLoginRequest(flow, &hydra.AcceptLoginRequest{
		Subject: loginReq.Subject,
		AMR:     amr,
		Context: map[string]any{
			"email":     sessionUser.Email,
			"name":      sessionUser.Name,
			"tenant_id": sessionUser.TenantID.String(),
		},
	})
	if err != nil {
		return nil, false, err
	}
	return fiber.Map{"next": nextRedirect, "redirect_to": resp.RedirectTo, "sso": true}, true, nil
}
