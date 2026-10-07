package handler

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
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
	p := s.provider(provider)
	if p == nil || !p.ConfiguredFor(clientID) || !cl.AllowsSignInMethod(provider) {
		s.logger.Warn("Social sign-in with a provider the client does not offer",
			zap.String("client_id", clientID), zap.String("provider", provider), zap.Bool("configured", p != nil && p.ConfiguredFor(clientID)))
		return s.endSignIn(c, flow, "invalid_request", msgSignInFailed)
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: flow, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return s.endSignIn(c, flow, "server_error", msgSignInFailed)
	}
	s.setStateCookie(c, state)

	s.logger.Info("Starting social sign-in", zap.String("provider", provider), zap.String("client_id", clientID))
	return c.Redirect(p.GetAuthURLForClient(state, clientID), fiber.StatusFound)
}

// socialProvider is what starting a sign-in needs from a provider service.
type socialProvider interface {
	ConfiguredFor(clientID string) bool
	GetAuthURLForClient(state, clientID string) string
}

// socialProviderNames lists the providers in the order sign-in screens show them.
var socialProviderNames = []string{"google", "github", "microsoft", "apple"}

// provider returns the service for a provider name, or nil when the name is
// unknown or the service is absent.
func (s *SocialHandler) provider(name string) socialProvider {
	switch name {
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

// SignInMethodsFor lists how users of cl can sign in here: "email" when the
// client allows passwords, "magic_link" when it allows emailed links, then
// each social provider the client enables and someone (the client or the
// deployment) has credentials for.
func (s *SocialHandler) SignInMethodsFor(cl *client.Client) []string {
	out := []string{}
	for _, method := range []string{"email", "magic_link"} {
		if cl.AllowsSignInMethod(method) {
			out = append(out, method)
		}
	}
	for _, name := range socialProviderNames {
		if p := s.provider(name); p != nil && cl.AllowsSignInMethod(name) && p.ConfiguredFor(cl.ClientID) {
			out = append(out, name)
		}
	}
	return out
}

// ConfiguredProviders lists the providers this deployment has credentials for.
func (s *SocialHandler) ConfiguredProviders() []string {
	out := []string{}
	for _, name := range socialProviderNames {
		if p := s.provider(name); p != nil && p.ConfiguredFor("") {
			out = append(out, name)
		}
	}
	return out
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
