package handler

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/user"
)

// Adding a provider account to the signed-in user's own account — "link
// another Google account" on the account screen.
//
// The API takes bearer tokens, and the provider answers a browser, so the
// round trip is split in four, and each split closes a gap:
//
//  1. POST …/connections/{provider}/link (token) hands back a one-time ticket
//     address. The browser then navigates to it, because the state cookie of a
//     provider round trip can only be set by a first-party navigation.
//  2. GET …/connections/link/{ticket} turns the ticket into the provider state
//     and sends the browser to the provider.
//  3. The provider's callback reads who signed in and keeps that as the link's
//     result — it links nothing — and returns the browser to the account screen
//     with the result's id.
//  4. POST …/connections/link/complete (token) links the result to the account
//     of the token. The result names the account the ticket was made for; a
//     token of anyone else is refused. That is what stops a link started from
//     someone else's ticket — a sign-in forced on this browser — from attaching
//     this user's provider account to that someone's account, or the reverse.

// identityReader is what a provider service gives the link: who signed in.
type identityReader interface {
	ReadIdentity(ctx context.Context, code, state, clientID string) (uuid.UUID, social.Identity, error)
}

// reader returns the provider service's identity reader, or nil.
func (s *SocialHandler) reader(provider string) identityReader {
	switch provider {
	case "google":
		if s.googleService != nil {
			return s.googleService
		}
	case "github":
		if s.githubService != nil {
			return s.githubService
		}
	case "microsoft":
		if s.microsoftService != nil {
			return s.microsoftService
		}
	case "apple":
		if s.appleService != nil {
			return s.appleService
		}
	}
	return nil
}

// linkableFor answers whether the signed-in user's token — its client — may
// start linking provider accounts at provider: the same rule as signing in with
// it, and the account must belong to the client's tenant.
func (s *SocialHandler) linkableFor(userID uuid.UUID, clientID, provider string) (*user.User, bool) {
	p := s.provider(provider)
	cl, err := s.clientService.GetByClientID(clientID)
	if p == nil || err != nil || !p.ConfiguredFor(clientID) || !cl.AllowsSignInMethod(provider) {
		return nil, false
	}
	u, err := s.userService.GetByID(userID)
	if err != nil || u.TenantID != cl.TenantID || !u.Active {
		return nil, false
	}
	return u, true
}

// ListLinkableProviders names the providers the signed-in user's account screen
// can add an account of.
// GET /api/v1/users/connections/available
func (s *SocialHandler) ListLinkableProviders(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	clientID, _ := c.Locals("client_id").(string)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to see your account.")
	}
	providers := []string{}
	for _, name := range socialProviderNames {
		if _, linkable := s.linkableFor(userID, clientID, name); linkable {
			providers = append(providers, name)
		}
	}
	return c.JSON(fiber.Map{"providers": providers})
}

// StartConnectionLink starts adding a provider account to the signed-in
// user's account.
// POST /api/v1/users/connections/{provider}/link
//
// It needs a recent sign-in, like the other changes to how an account signs
// in. The answer is the address the browser navigates to next.
func (s *SocialHandler) StartConnectionLink(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	clientID, _ := c.Locals("client_id").(string)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to change your account.")
	}
	if !signedInRecently(c) {
		return refuseStaleSignIn(c, "Sign in again to add a sign-in account.")
	}
	provider := c.Params("provider")
	if _, linkable := s.linkableFor(userID, clientID, provider); !linkable {
		return apierror.Refuse(c, fiber.StatusNotFound, "provider_unavailable", "This provider cannot be added here.")
	}
	ticket, err := s.stateStore.Save(c.Context(), oauthState{ClientID: clientID, LinkUserID: userID.String(), LinkProvider: provider, LinkKind: linkTicket})
	if err != nil {
		s.logger.Error("Failed to store a link ticket", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The link could not be started. Try again shortly.")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"url": "/api/v1/users/connections/link/" + url.PathEscape(ticket)})
}

// accountScreen is where a link's outcome is shown: the account screen of the
// login UI, with the result's id or the reason it did not work.
func (s *SocialHandler) accountScreen(q url.Values) string {
	return strings.TrimRight(s.frontendURL, "/") + "/account?" + q.Encode()
}

// RunConnectionLink sends the browser to the provider for a link started with
// a ticket.
// GET /api/v1/users/connections/link/{ticket}
//
// A page navigation, not an API call: it sets the state cookie that binds the
// round trip to this browser. A ticket works once.
func (s *SocialHandler) RunConnectionLink(c *fiber.Ctx) error {
	ticketValue := c.Params("ticket")
	if decoded, err := url.PathUnescape(ticketValue); err == nil {
		ticketValue = decoded
	}
	ticket, found, err := s.stateStore.Consume(c.Context(), ticketValue)
	if err != nil || !found || ticket.LinkKind != linkTicket || ticket.LinkUserID == "" {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"invalid_request"}}), fiber.StatusFound)
	}
	userID, parseErr := uuid.Parse(ticket.LinkUserID)
	if parseErr != nil {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"invalid_request"}}), fiber.StatusFound)
	}
	if _, linkable := s.linkableFor(userID, ticket.ClientID, ticket.LinkProvider); !linkable {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"provider_unavailable"}}), fiber.StatusFound)
	}
	state, err := s.stateStore.Save(c.Context(), oauthState{ClientID: ticket.ClientID, LinkUserID: ticket.LinkUserID, LinkProvider: ticket.LinkProvider, LinkKind: linkState})
	if err != nil {
		s.logger.Error("Failed to store OAuth state for a link", zap.Error(err))
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"server_error"}}), fiber.StatusFound)
	}
	s.setStateCookie(c, state)
	s.logger.Info("Starting a provider link", zap.String("provider", ticket.LinkProvider), zap.String("client_id", ticket.ClientID))
	return c.Redirect(s.provider(ticket.LinkProvider).GetAuthURLForClient(state, ticket.ClientID), fiber.StatusFound)
}

// finishConnectionLink ends a provider callback of a link: it reads who signed
// in at the provider and keeps that as the link's result for the account screen
// to complete. Nothing is linked here.
func (s *SocialHandler) finishConnectionLink(c *fiber.Ctx, provider, code, state string, stored oauthState) error {
	reader := s.reader(provider)
	if reader == nil || stored.LinkKind != linkState || stored.LinkProvider != provider {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"invalid_request"}}), fiber.StatusFound)
	}
	tenantID, identity, err := reader.ReadIdentity(c.Context(), code, state, stored.ClientID)
	if err != nil || identity.Subject == "" {
		s.logger.Warn("A provider link could not read the provider account", zap.String("provider", provider), zap.Error(err))
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"provider_failed"}}), fiber.StatusFound)
	}
	userID, parseErr := uuid.Parse(stored.LinkUserID)
	if parseErr != nil {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"invalid_request"}}), fiber.StatusFound)
	}
	if u, err := s.userService.GetByID(userID); err != nil || u.TenantID != tenantID {
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"invalid_request"}}), fiber.StatusFound)
	}
	result, err := s.stateStore.Save(c.Context(), oauthState{
		ClientID: stored.ClientID, LinkUserID: stored.LinkUserID, LinkProvider: provider,
		LinkSubject: identity.Subject, LinkEmail: identity.Email, LinkKind: linkResult,
	})
	if err != nil {
		s.logger.Error("Failed to store a link result", zap.Error(err))
		return c.Redirect(s.accountScreen(url.Values{"link_error": {"server_error"}}), fiber.StatusFound)
	}
	return c.Redirect(s.accountScreen(url.Values{"link": {result}}), fiber.StatusSeeOther)
}

// CompleteConnectionLinkRequest is the body of CompleteConnectionLink: the
// result the account screen was returned with.
type CompleteConnectionLinkRequest struct {
	Result string `json:"result"`
}

// CompleteConnectionLink adds the provider account a link's result holds to the
// account of the token.
// POST /api/v1/users/connections/link/complete  { "result": "…" }
//
// The result is refused unless the token is the user it was made for, and
// needs a recent sign-in. The provider's address does not have to be the
// account's own: it is recorded with the link, the account's address and its
// verification stay as they are.
func (s *SocialHandler) CompleteConnectionLink(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to change your account.")
	}
	if !signedInRecently(c) {
		return refuseStaleSignIn(c, "Sign in again to add a sign-in account.")
	}
	var req CompleteConnectionLinkRequest
	if err := c.BodyParser(&req); err != nil || req.Result == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "result is required")
	}
	result, found, err := s.stateStore.Consume(c.Context(), req.Result)
	if err != nil {
		s.logger.Error("Failed to read a link result", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The link could not be completed. Try again shortly.")
	}
	if !found || result.LinkKind != linkResult || result.LinkSubject == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "This link has expired or was already used. Start again.")
	}
	clientID, _ := c.Locals("client_id").(string)
	if result.LinkUserID != userID.String() || result.ClientID != clientID {
		s.logger.Warn("A link result was presented by another user", zap.String("user_id", userID.String()))
		return apierror.Refuse(c, fiber.StatusForbidden, "forbidden", "This link was started for another account.")
	}
	u, err := s.userService.GetByID(userID)
	if err != nil || !u.Active {
		return apierror.Refuse(c, fiber.StatusForbidden, "forbidden", "This account cannot add a sign-in account.")
	}
	err = s.userService.LinkProvider(userID, user.ProviderLink{Provider: result.LinkProvider, Subject: result.LinkSubject, Email: result.LinkEmail})
	switch {
	case errors.Is(err, user.ErrProviderSubjectTaken):
		return apierror.Refuse(c, fiber.StatusConflict, "linked_to_another", "That account already signs in to another account here. Remove it there first.")
	case err != nil:
		s.logger.Error("Failed to link a provider account", zap.String("user_id", userID.String()), zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_error", "The link could not be completed. Try again shortly.")
	}
	connection := fiber.Map{"provider": result.LinkProvider, "email": result.LinkEmail}
	if after, err := s.userService.GetByID(userID); err == nil {
		for _, identity := range after.Identities {
			if identity.Provider == result.LinkProvider && identity.Subject == result.LinkSubject {
				connection = fiber.Map{"id": identity.ID, "provider": identity.Provider, "email": identity.EmailAtLink, "linked_at": identity.LinkedAt}
			}
		}
	}
	if s.auditService != nil {
		entry := audit.EntryFromFiber(c, u.TenantID, audit.ActionUserConnectionAdded, "user", u.ID.String())
		entry.ActorID = &u.ID
		entry.ActorEmail = u.Email
		entry.ActorType = "user"
		entry.Details["provider"] = result.LinkProvider
		s.auditService.LogAsync(entry)
	}
	return c.JSON(fiber.Map{"connection": connection})
}
