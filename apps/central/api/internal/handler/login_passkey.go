package handler

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/passkey"
)

// GrantsPasskeys lets a login flow be completed with a passkey. Without it the
// passkey sign-in endpoints answer 404 and the screen offers none.
func (h *AuthHandler) GrantsPasskeys(c *passkey.Ceremonies) {
	h.passkeys = c
}

// offersPasskey reports whether a login flow of cl can be completed with a
// passkey here: passkeys are set up on this deployment, and the client offers
// them - wherever it offers the password form, and where it names "passkey".
func (h *AuthHandler) offersPasskey(cl *client.Client) bool {
	return h.passkeys != nil && cl.AllowsPasskey()
}

// passkeyFlowClient finds the client a login flow is for, once the flow can be
// completed with a passkey. It writes the refusal itself, and then returns nil.
func (h *AuthHandler) passkeyFlowClient(c *fiber.Ctx, flow string) *client.Client {
	if h.passkeys == nil {
		_ = apierror.Refuse(c, fiber.StatusNotFound, "passkeys_unavailable", "Passkeys are not set up on this server.")
		return nil
	}
	loginReq, err := h.hydraClient.GetLoginRequest(flow)
	if err != nil {
		_ = respondFlowLookupError(c, err)
		return nil
	}
	cl, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("OAuth client is not registered in Authway",
			zap.String("client_id", loginReq.Client.ClientID), zap.Error(err))
		_ = apierror.Refuse(c, fiber.StatusBadRequest, "client_not_registered", "This application is not registered with Authway.")
		return nil
	}
	if !cl.AllowsPasskey() {
		_ = respondSignInMethodNotAllowed(c)
		return nil
	}
	return cl
}

// BeginPasskeyLogin gives the browser what it needs to ask the user's device
// for a passkey.
// POST /api/v1/login-flows/:flow/passkey/begin
//
// Nobody has said who is signing in, so the options name no credential: the
// device offers the passkeys it holds for this login screen, and the answer
// says whose they are. The ceremony belongs to this flow and its client's
// tenant - the same screen serves every tenant, so a passkey registered in one
// opens no other tenant's flow.
func (h *AuthHandler) BeginPasskeyLogin(c *fiber.Ctx) error {
	flow := flowParam(c)
	cl := h.passkeyFlowClient(c, flow)
	if cl == nil {
		return nil
	}
	login, err := h.passkeys.BeginLogin(c.UserContext(), flow, cl.TenantID)
	if err != nil {
		h.logger.Error("Failed to begin a passkey sign-in", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "A passkey sign-in could not be started. Try again shortly.")
	}
	return c.JSON(fiber.Map{"ceremony_id": login.CeremonyID, "options": login.Options})
}

// PasskeyLoginRequest is the body of POST /api/v1/login-flows/:flow/passkey/finish.
type PasskeyLoginRequest struct {
	CeremonyID string `json:"ceremony_id"`
	// Credential is the browser's answer: the PublicKeyCredential as its
	// toJSON() writes it.
	Credential json.RawMessage `json:"credential"`
	// Remember is the user's choice to stay signed in on this browser.
	Remember bool `json:"remember"`
}

// FinishPasskeyLogin checks the device's answer and, when it holds, continues
// the flow as a password would: a second factor, the terms and the consent
// still apply.
// POST /api/v1/login-flows/:flow/passkey/finish
func (h *AuthHandler) FinishPasskeyLogin(c *fiber.Ctx) error {
	flow := flowParam(c)
	var req PasskeyLoginRequest
	if err := c.BodyParser(&req); err != nil || req.CeremonyID == "" || len(req.Credential) == 0 {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "ceremony_id and credential are required")
	}
	cl := h.passkeyFlowClient(c, flow)
	if cl == nil {
		return nil
	}
	signedIn, err := h.passkeys.FinishLogin(c.UserContext(), flow, cl.TenantID, req.CeremonyID, req.Credential)
	switch {
	case errors.Is(err, passkey.ErrCeremonyNotFound):
		middleware.IncrementRateLimitOnFailure(c)
		return apierror.Refuse(c, fiber.StatusBadRequest, "ceremony_expired", "The passkey sign-in took too long. Try again.")
	case errors.Is(err, passkey.ErrRejected), errors.Is(err, passkey.ErrCloneSuspected):
		reason := "passkey_rejected"
		if errors.Is(err, passkey.ErrCloneSuspected) {
			reason = "passkey_clone_suspected"
		}
		// The reason is for the log; what the caller learns is the same for all.
		h.logAuthFailure(c, cl.TenantID, audit.ActionUserLoginFailed, "", reason, map[string]any{"method": byPasskey.audit})
		h.logger.Warn("A passkey sign-in was refused", zap.String("reason", reason), zap.Error(err))
		middleware.IncrementRateLimitOnFailure(c)
		return apierror.Refuse(c, fiber.StatusUnauthorized, "passkey_rejected", "The passkey could not be verified.")
	case err != nil:
		h.logger.Error("Failed to finish a passkey sign-in", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The passkey sign-in could not be completed. Try again shortly.")
	}
	return h.firstFactorPassed(c, flow, signedIn.User, byPasskey, req.Remember)
}
