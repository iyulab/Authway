package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/invitation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Accepting an invitation with a provider.
//
// The person opened the mailed link and presses "continue with Google" on the
// accept screen. There is no application's login flow behind it, so the
// sign-in is started for the invitation itself: it names the client whose
// provider credentials and tenant apply (invitation.Service.SignInClientID),
// and the callback accepts the invitation only for the Google account that
// has verified the address the invitation was sent to. Anyone else changes
// nothing — no account is created or linked and the invitation stays pending.

// invitationReceiptTTL is how long the screen shown after accepting can be
// read again — a reload keeps it.
const invitationReceiptTTL = 10 * time.Minute

const invitationReceiptPrefix = "invitation:receipt:"

// OfferForInvitation lists the providers the invitation can be accepted with:
// Google, when the invitation's client allows it and has credentials for it.
func (s *SocialHandler) OfferForInvitation(inv *invitation.Invitation) []string {
	if s.invitations == nil || s.googleService == nil || inv == nil {
		return nil
	}
	clientID := s.invitations.SignInClientID(inv)
	if clientID == "" {
		return nil
	}
	cl, err := s.clientService.GetByClientID(clientID)
	if err != nil || cl.TenantID != inv.TenantID {
		return nil
	}
	if s.googleService.ConfiguredFor(clientID) && cl.AllowsSignInMethod("google") {
		return []string{"google"}
	}
	return nil
}

// StartInvitationSocial sends the browser to the provider to accept an
// invitation.
//
// GET /api/v1/invitations/token/:token/social/:provider is a page navigation,
// for the same reason the application flow's start is: the state cookie is
// set first-party. Every failure ends back on the accept screen.
func (s *SocialHandler) StartInvitationSocial(c *fiber.Ctx) error {
	token := c.Params("token")
	if decoded, err := url.PathUnescape(token); err == nil {
		token = decoded
	}
	provider := c.Params("provider")
	if s.invitations == nil || s.googleService == nil || token == "" {
		return s.invitationFailed(c, "", "invalid_request")
	}

	inv, err := s.invitations.GetByToken(token)
	if err != nil || !inv.CanBeAccepted() {
		return s.invitationFailed(c, "", "invitation_unavailable")
	}
	if provider != "google" || !slices.Contains(s.OfferForInvitation(inv), provider) {
		return s.invitationFailed(c, token, "provider_unavailable")
	}
	clientID := s.invitations.SignInClientID(inv)

	state, err := s.stateStore.Save(c.Context(), oauthState{ClientID: clientID, InvitationID: inv.ID.String()})
	if err != nil {
		s.logger.Error("Failed to store OAuth state for an invitation", zap.Error(err))
		return s.invitationFailed(c, token, "failed")
	}
	s.setStateCookie(c, state)
	s.setInvitationTokenCookie(c, token)

	authURL, err := url.Parse(s.googleService.GetAuthURLForClient(state, clientID))
	if err != nil {
		s.logger.Error("Google sign-in address is not a URL", zap.Error(err))
		return s.invitationFailed(c, token, "failed")
	}
	q := authURL.Query()
	q.Set("login_hint", inv.Email)
	authURL.RawQuery = q.Encode()

	s.logger.Info("Starting an invitation sign-in", zap.String("provider", provider), zap.String("client_id", clientID))
	return c.Redirect(authURL.String(), fiber.StatusFound)
}

// finishInvitationSignIn completes the callback of a sign-in started by
// StartInvitationSocial.
func (s *SocialHandler) finishInvitationSignIn(c *fiber.Ctx, code, state string, stored oauthState) error {
	// The token only brings the person back to their form on a refusal; what
	// is authorized is the invitation the server stored the id of.
	token := string([]byte(c.Cookies(invitationTokenCookie)))
	s.clearInvitationTokenCookie(c)
	if s.invitations == nil || s.googleService == nil {
		return s.invitationFailed(c, "", "invalid_request")
	}
	invID, err := uuid.Parse(stored.InvitationID)
	if err != nil {
		return s.invitationFailed(c, "", "invalid_request")
	}
	inv, err := s.invitations.GetByID(invID)
	if err != nil || !inv.CanBeAccepted() {
		return s.invitationFailed(c, "", "invitation_unavailable")
	}
	// The client was named by the server when the sign-in started; hold it to
	// the invitation's tenant anyway, so the state can never carry one across.
	cl, err := s.clientService.GetByClientID(stored.ClientID)
	if err != nil || cl.TenantID != inv.TenantID {
		return s.invitationFailed(c, token, "failed")
	}

	account, err := s.googleService.HandleCallbackForInvitation(c.Context(), code, state, stored.ClientID, inv.Email)
	if err != nil {
		reason := "failed"
		switch {
		case errors.Is(err, social.ErrInvitationAddressMismatch):
			reason = "address_mismatch"
		case errors.Is(err, social.ErrLinkedToAnother):
			reason = "linked_to_another"
		case errors.Is(err, social.ErrInvitationAccountSuspended):
			reason = "account_suspended"
		}
		s.logSocialLoginFailure(c, inv.TenantID, "google", "invitation_"+reason, nil)
		return s.invitationFailed(c, token, reason)
	}
	// An account reached by the Google account's earlier link may carry another
	// address than the one invited; the invitation is then not this person's.
	if !strings.EqualFold(strings.TrimSpace(account.Email), strings.TrimSpace(inv.Email)) {
		s.logSocialLoginFailure(c, inv.TenantID, "google", "invitation_address_mismatch", nil)
		return s.invitationFailed(c, token, "address_mismatch")
	}
	if !account.Active {
		return s.invitationFailed(c, token, "account_suspended")
	}
	if err := s.invitations.AcceptInvitedUser(inv.ID, account.ID); err != nil {
		s.logger.Error("Failed to mark the invitation accepted after a Google sign-in",
			zap.String("user_id", account.ID.String()), zap.Error(err))
		return s.invitationFailed(c, "", "invitation_unavailable")
	}

	receipt, err := s.saveInvitationReceipt(c, inv)
	if err != nil {
		s.logger.Error("Failed to store the invitation receipt", zap.Error(err))
		return s.invitationFailed(c, "", "failed")
	}
	s.logger.Info("Invitation accepted with Google",
		zap.String("user_id", account.ID.String()), zap.String("tenant_id", inv.TenantID.String()))
	return c.Redirect(strings.TrimRight(s.frontendURL, "/")+"/invitation/accept?"+url.Values{"receipt": {receipt}}.Encode(), fiber.StatusSeeOther)
}

// invitationTokenCookie remembers, for the browser that started an invitation
// sign-in, the token it opened — so a refusal can send it back to the same
// form. It authorizes nothing and holds nothing the browser does not have.
const invitationTokenCookie = "invitation_token"

func (s *SocialHandler) setInvitationTokenCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     invitationTokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(oauthStateTTL.Seconds()),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func (s *SocialHandler) clearInvitationTokenCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{Name: invitationTokenCookie, Value: "", Path: "/", MaxAge: -1, HTTPOnly: true})
}

// saveInvitationReceipt keeps what the screen after accepting shows, under a
// random id: the accepted invitation itself is no longer readable by its token.
func (s *SocialHandler) saveInvitationReceipt(c *fiber.Ctx, inv *invitation.Invitation) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	fields := map[string]string{
		"tenant_name":  inv.TenantName,
		"inviter_name": inv.InviterName,
		"email":        inv.Email,
		"message":      inv.Message,
	}
	if inv.App != nil {
		app, err := json.Marshal(inv.App)
		if err != nil {
			return "", err
		}
		fields["client"] = string(app)
	}
	if err := s.stateStore.store.Put(c.Context(), invitationReceiptPrefix+id, fields, invitationReceiptTTL); err != nil {
		return "", err
	}
	return id, nil
}

// InvitationReceipt answers what the accept screen shows after an invitation
// was accepted with a provider.
//
// GET /api/v1/invitations/receipts/:id
func (s *SocialHandler) InvitationReceipt(c *fiber.Ctx) error {
	id := c.Params("id")
	var fields map[string]string
	if id != "" {
		var err error
		if fields, err = s.stateStore.store.Fields(c.Context(), invitationReceiptPrefix+id); err != nil {
			s.logger.Error("Failed to read an invitation receipt", zap.Error(err))
			return apierror.Refuse(c, fiber.StatusInternalServerError, apierror.CodeForStatus(fiber.StatusInternalServerError), "Failed to read the receipt")
		}
	}
	if len(fields) == 0 {
		return apierror.Refuse(c, fiber.StatusNotFound, "invalid_receipt", "receipt not found or expired")
	}
	out := fiber.Map{
		"tenant_name":  fields["tenant_name"],
		"inviter_name": fields["inviter_name"],
		"email":        fields["email"],
		"message":      fields["message"],
	}
	if raw := fields["client"]; raw != "" {
		var app invitation.InvitationApp
		if err := json.Unmarshal([]byte(raw), &app); err == nil {
			out["client"] = app
		}
	}
	return c.JSON(fiber.Map{"invitation": out})
}

// invitationFailed sends the browser back to the accept screen with the
// reason. The token goes back too when the invitation is still the person's to
// use, so the screen can show the form again.
func (s *SocialHandler) invitationFailed(c *fiber.Ctx, token, reason string) error {
	q := url.Values{"error": {reason}}
	if token != "" {
		q.Set("token", token)
	}
	return c.Redirect(strings.TrimRight(s.frontendURL, "/")+"/invitation/accept?"+q.Encode(), fiber.StatusFound)
}
