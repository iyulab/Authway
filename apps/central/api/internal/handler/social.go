package handler

import (
	"context"
	"net/url"
	"strings"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SocialHandler struct {
	googleService    *social.GoogleService
	githubService    *social.GitHubService
	microsoftService *social.MicrosoftService
	appleService     *social.AppleService
	userService      user.Service
	hydraClient      *hydra.Client
	logger           *zap.Logger
	auditService     audit.Service
	clientService    client.Service
	stateStore       *OAuthStateStore
	flows            *signIn
	frontendURL      string // login UI: the error screen and the second factor step
}

// NewSocialHandlerWithAllProviders creates a SocialHandler with all OAuth providers
func NewSocialHandlerWithAllProviders(
	googleService *social.GoogleService,
	githubService *social.GitHubService,
	microsoftService *social.MicrosoftService,
	appleService *social.AppleService,
	userService user.Service,
	clientService client.Service,
	hydraClient *hydra.Client,
	logger *zap.Logger,
	auditService audit.Service,
	stateStore *OAuthStateStore,
	frontendURL string,
) *SocialHandler {
	var flows *signIn
	if stateStore != nil {
		flows = newSignIn(hydraClient, userService, logger, stateStore.redis)
	}
	return &SocialHandler{
		stateStore:       stateStore,
		flows:            flows,
		frontendURL:      frontendURL,
		googleService:    googleService,
		githubService:    githubService,
		microsoftService: microsoftService,
		appleService:     appleService,
		userService:      userService,
		clientService:    clientService,
		hydraClient:      hydraClient,
		logger:           logger,
		auditService:     auditService,
	}
}

// logSocialLogin records a sign-in completed on the provider's word alone,
// with the fields every completed sign-in records (signInMethod.details).
func (s *SocialHandler) logSocialLogin(c *fiber.Ctx, u *user.User, method signInMethod, flow, clientID string) {
	if s.auditService == nil || u == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, audit.ActionUserLogin, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	for k, v := range method.details(flow, true) {
		entry.Details[k] = v
	}
	entry.Details["client_id"] = clientID
	s.auditService.LogAsync(entry)
}

// failSocialSignIn ends a provider callback that did not sign anyone in: it
// records the failure against the tenant the client belongs to and returns
// the user to the application with the matching OAuth error. A refusal (the
// address is not invited, not verified, or linked elsewhere) is a decision,
// logged as a warning; anything else is a fault.
func (s *SocialHandler) failSocialSignIn(c *fiber.Ctx, provider, clientID, loginChallenge string, err error) error {
	errCode, description, reason := socialFailure(err)
	fields := []zap.Field{zap.String("provider", provider), zap.String("client_id", clientID), zap.String("reason", reason), zap.Error(err)}
	if errCode == "access_denied" {
		s.logger.Warn("Social sign-in refused", fields...)
	} else {
		s.logger.Error("Social sign-in failed", fields...)
	}
	s.logSocialLoginFailure(c, s.tenantOfClient(clientID), provider, reason, map[string]any{
		"client_id": clientID,
		"error":     err.Error(),
	})
	return s.endSignIn(c, loginChallenge, errCode, description)
}

// tenantOfClient returns the tenant a client belongs to, or uuid.Nil when
// the client cannot be resolved.
func (s *SocialHandler) tenantOfClient(clientID string) uuid.UUID {
	if clientID == "" || s.clientService == nil {
		return uuid.Nil
	}
	registered, err := s.clientService.GetByClientID(clientID)
	if err != nil || registered == nil {
		return uuid.Nil
	}
	return registered.TenantID
}

// logSocialLoginFailure records a failed social sign-in in the audit log of
// tenantID, so the tenant's administrators see attempts against it. tenantID
// is uuid.Nil only when the failure happened before a client was known.
func (s *SocialHandler) logSocialLoginFailure(c *fiber.Ctx, tenantID uuid.UUID, provider, reason string, extra map[string]any) {
	if s.auditService == nil {
		return
	}
	details := map[string]any{
		"provider": provider,
		"method":   "social",
		"reason":   reason,
	}
	for k, v := range extra {
		details[k] = v
	}
	entry := &audit.AuditEntry{
		TenantID:     tenantID,
		ActorType:    "anonymous",
		Action:       audit.ActionUserLoginFailed,
		Severity:     audit.SeverityWarning,
		ResourceType: "user",
		IPAddress:    c.IP(),
		UserAgent:    c.Get("User-Agent"),
		Details:      details,
		Success:      false,
		ErrorMsg:     reason,
	}
	if err := s.auditService.Log(context.Background(), entry); err != nil {
		s.logger.Warn("Failed to record social auth-failure audit", zap.Error(err), zap.String("provider", provider))
	}
}

// GoogleCallback handles the Google OAuth callback
func (s *SocialHandler) GoogleCallback(c *fiber.Ctx) error {
	// IMPORTANT: Make copies of query strings because Fiber reuses internal buffers
	code := string([]byte(c.Query("code")))
	state := string([]byte(c.Query("state")))
	errorParam := string([]byte(c.Query("error")))

	// Debug: log what parameters we received
	s.logger.Info("GoogleCallback received",
		zap.String("state", state),
		zap.Int("code_length", len(code)))

	// Check for OAuth error
	if errorParam != "" {
		s.logger.Warn("Google OAuth error", zap.String("error", errorParam))
		return s.endSignIn(c, s.recoverChallenge(c, state), "access_denied", msgProviderRefused)
	}

	// Validate required parameters
	if code == "" || state == "" {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	// Verify state against cookie (CSRF protection)
	// IMPORTANT: Make copy of cookie value because Fiber reuses internal buffers
	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		s.logger.Warn("State mismatch",
			zap.String("cookie_state", stateCookie),
			zap.String("param_state", state))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	if !found {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	loginChallenge := stored.LoginChallenge
	retrievedClientID := stored.ClientID

	// Clear the state cookie
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
	})

	// Process Google OAuth callback (client-specific or central)
	authUser, err := s.googleService.HandleCallbackForClient(c.Context(), code, state, retrievedClientID)
	if err != nil {
		return s.failSocialSignIn(c, "google", retrievedClientID, loginChallenge, err)
	}

	return s.finishSignIn(c, "google", retrievedClientID, loginChallenge, authUser)
}

// ======================================
// GitHub OAuth Handlers
// ======================================

// GitHubCallback handles the GitHub OAuth callback
func (s *SocialHandler) GitHubCallback(c *fiber.Ctx) error {
	if s.githubService == nil {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	code := string([]byte(c.Query("code")))
	state := string([]byte(c.Query("state")))
	errorParam := string([]byte(c.Query("error")))

	if errorParam != "" {
		s.logger.Warn("GitHub OAuth error", zap.String("error", errorParam))
		return s.endSignIn(c, s.recoverChallenge(c, state), "access_denied", msgProviderRefused)
	}

	if code == "" || state == "" {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	if !found {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	loginChallenge := stored.LoginChallenge
	retrievedClientID := stored.ClientID

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
	})

	authUser, err := s.githubService.HandleCallbackForClient(c.Context(), code, state, retrievedClientID)
	if err != nil {
		return s.failSocialSignIn(c, "github", retrievedClientID, loginChallenge, err)
	}

	return s.finishSignIn(c, "github", retrievedClientID, loginChallenge, authUser)
}

// ======================================
// Microsoft OAuth Handlers
// ======================================

// MicrosoftCallback handles the Microsoft OAuth callback
func (s *SocialHandler) MicrosoftCallback(c *fiber.Ctx) error {
	if s.microsoftService == nil {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	code := string([]byte(c.Query("code")))
	state := string([]byte(c.Query("state")))
	errorParam := string([]byte(c.Query("error")))

	if errorParam != "" {
		s.logger.Warn("Microsoft OAuth error", zap.String("error", errorParam))
		return s.endSignIn(c, s.recoverChallenge(c, state), "access_denied", msgProviderRefused)
	}

	if code == "" || state == "" {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	if !found {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	loginChallenge := stored.LoginChallenge
	retrievedClientID := stored.ClientID

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
	})

	authUser, err := s.microsoftService.HandleCallbackForClient(c.Context(), code, state, retrievedClientID)
	if err != nil {
		return s.failSocialSignIn(c, "microsoft", retrievedClientID, loginChallenge, err)
	}

	return s.finishSignIn(c, "microsoft", retrievedClientID, loginChallenge, authUser)
}

// ======================================
// Apple OAuth Handlers
// ======================================

// AppleCallback handles the Apple OAuth callback (POST because of form_post response_mode)
func (s *SocialHandler) AppleCallback(c *fiber.Ctx) error {
	if s.appleService == nil {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	// Apple uses form_post response mode
	code := c.FormValue("code")
	state := c.FormValue("state")
	errorParam := c.FormValue("error")

	// Also try query params for GET requests
	if code == "" {
		code = string([]byte(c.Query("code")))
	}
	if state == "" {
		state = string([]byte(c.Query("state")))
	}
	if errorParam == "" {
		errorParam = string([]byte(c.Query("error")))
	}

	if errorParam != "" {
		s.logger.Warn("Apple OAuth error", zap.String("error", errorParam))
		return s.endSignIn(c, s.recoverChallenge(c, state), "access_denied", msgProviderRefused)
	}

	if code == "" || state == "" {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	if !found {
		return s.endSignIn(c, "", "invalid_request", msgInvalidSignIn)
	}
	loginChallenge := stored.LoginChallenge
	retrievedClientID := stored.ClientID

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
	})

	authUser, err := s.appleService.HandleCallbackForClient(c.Context(), code, state, retrievedClientID)
	if err != nil {
		return s.failSocialSignIn(c, "apple", retrievedClientID, loginChallenge, err)
	}

	return s.finishSignIn(c, "apple", retrievedClientID, loginChallenge, authUser)
}

// ======================================
// Helper Methods
// ======================================

// finishSignIn continues the login flow for a user the provider has vouched
// for. The provider's word is one factor: a user with a second factor of
// their own is sent to the sign-in screen to give it, whatever the provider
// asked of them.
func (s *SocialHandler) finishSignIn(c *fiber.Ctx, provider, clientID, flow string, u *user.User) error {
	method := byProvider(provider)
	step, err := s.flows.afterFirstFactor(flow, u, method, true, 3600)
	if err != nil {
		s.logger.Error("Failed to continue the login flow after a social sign-in",
			zap.String("provider", provider), zap.Error(err))
		return s.endSignIn(c, flow, "server_error", msgSignInFailed)
	}
	if step.MFAChallenge != "" {
		s.logger.Info("Provider sign-in verified, second factor required",
			zap.String("user_id", u.ID.String()), zap.String("provider", provider))
		q := url.Values{"flow": {flow}, "mfa_challenge": {step.MFAChallenge}}
		return c.Redirect(strings.TrimRight(s.frontendURL, "/")+"/mfa/verify?"+q.Encode(), fiber.StatusFound)
	}

	s.logger.Info("Social sign-in successful",
		zap.String("user_id", u.ID.String()), zap.String("provider", provider))
	s.logSocialLogin(c, u, method, flow, clientID)
	return s.renderRedirectPage(c, step.RedirectTo)
}

// renderRedirectPage renders an HTML page with JavaScript redirect
func (s *SocialHandler) renderRedirectPage(c *fiber.Ctx, redirectTo string) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Redirecting...</title>
    <meta charset="utf-8">
</head>
<body>
    <div style="text-align: center; padding: 50px; font-family: sans-serif;">
        <div style="font-size: 18px; color: #666; margin-bottom: 20px;">로그인 처리 중...</div>
        <div style="width: 40px; height: 40px; margin: 0 auto; border: 4px solid #f3f3f3; border-top: 4px solid #4F46E5; border-radius: 50%; animation: spin 1s linear infinite;"></div>
    </div>
    <style>
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
    <script>
        window.location.href = "` + redirectTo + `";
    </script>
</body>
</html>`

	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}
