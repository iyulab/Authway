package handler

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// StartSocialLogin sends the browser to a social provider for a login flow.
//
// GET /api/v1/login-flows/:flow/social/:provider is a page navigation, not an
// API call: the state cookie is then set in a first-party context (a
// cross-site fetch cannot set it), and failures end in the browser flow like
// the provider callbacks do. The client comes from the flow itself, never
// from the request.
func (s *SocialHandler) StartSocialLogin(c *fiber.Ctx) error {
	flow := flowParam(c)
	provider := c.Params("provider")

	loginReq, err := s.hydraClient.GetLoginRequest(flow)
	if err != nil {
		s.logger.Warn("Social sign-in started for an unknown login flow", zap.Error(err))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	clientID := loginReq.Client.ClientID

	cl, err := s.clientService.GetByClientID(clientID)
	if err != nil {
		s.logger.Error("OAuth client is not registered in Authway", zap.String("client_id", clientID), zap.Error(err))
		return s.endSignIn(c, flow, "invalid_client", msgSignInFailed)
	}
	authURLFor := s.authURLBuilder(provider)
	if authURLFor == nil || !cl.AllowsSignInMethod(provider) {
		s.logger.Warn("Social sign-in with a provider the client does not offer",
			zap.String("client_id", clientID), zap.String("provider", provider), zap.Bool("configured", authURLFor != nil))
		return s.endSignIn(c, flow, "invalid_request", msgSignInFailed)
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: flow, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return s.endSignIn(c, flow, "server_error", msgSignInFailed)
	}
	s.setStateCookie(c, state)

	s.logger.Info("Starting social sign-in", zap.String("provider", provider), zap.String("client_id", clientID))
	return c.Redirect(authURLFor(state, clientID), fiber.StatusFound)
}

// authURLBuilder returns the authorization-URL builder for a configured
// provider, or nil when the provider is unknown or not configured.
func (s *SocialHandler) authURLBuilder(provider string) func(state, clientID string) string {
	switch provider {
	case "google":
		if s.googleService != nil {
			return s.googleService.GetAuthURLForClient
		}
	case "github":
		if s.githubService != nil {
			return s.githubService.GetAuthURLForClient
		}
	case "microsoft":
		if s.microsoftService != nil {
			return s.microsoftService.GetAuthURLForClient
		}
	case "apple":
		if s.appleService != nil {
			return s.appleService.GetAuthURLForClient
		}
	}
	return nil
}

// setStateCookie binds the sign-in to this browser. Over HTTPS it is
// SameSite=None so it also comes back on Apple's cross-site form_post
// callback; plain-HTTP local runs keep Lax, which browsers require there.
func (s *SocialHandler) setStateCookie(c *fiber.Ctx, state string) {
	secure := c.Protocol() == "https"
	sameSite := fiber.CookieSameSiteLaxMode
	if secure {
		sameSite = fiber.CookieSameSiteNoneMode
	}
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}
