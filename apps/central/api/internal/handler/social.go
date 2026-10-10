package handler

import (
	"context"

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
	authTimes        *AuthTimeStore
	frontendURL      string // login UI, for the error screen
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
	var authTimes *AuthTimeStore
	if stateStore != nil {
		authTimes = NewAuthTimeStore(stateStore.redis)
	}
	return &SocialHandler{
		stateStore:       stateStore,
		authTimes:        authTimes,
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

// logSocialLogin emits a success-path login audit entry with the resolved user
// as actor, tagging the OAuth provider in Details.
func (s *SocialHandler) logSocialLogin(c *fiber.Ctx, u *user.User, provider string, extra map[string]any) {
	if s.auditService == nil || u == nil {
		return
	}
	entry := audit.EntryFromFiber(c, u.TenantID, audit.ActionUserLogin, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	entry.Details["provider"] = provider
	entry.Details["method"] = "social"
	for k, v := range extra {
		entry.Details[k] = v
	}
	s.auditService.LogAsync(entry)
}

// logSocialLoginFailure emits a sync failure audit for social callbacks. We
// rarely know the user at failure time (OAuth handshake broke before user
// resolution), so tenantID falls back to uuid.Nil when unknown.
func (s *SocialHandler) logSocialLoginFailure(c *fiber.Ctx, provider, reason string, extra map[string]any) {
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
		TenantID:     uuid.Nil,
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
		s.logger.Error("Google OAuth callback failed",
			zap.Error(err),
			zap.String("client_id", retrievedClientID))
		s.logSocialLoginFailure(c, "google", "oauth_callback_failed", map[string]any{
			"client_id": retrievedClientID,
			"error":     err.Error(),
		})
		errCode, description := socialFailure(err)
		return s.endSignIn(c, loginChallenge, errCode, description)
	}

	// Update last login time using the service
	if err := s.userService.UpdateLastLogin(authUser.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
		// Continue despite error as user is authenticated
	}

	// Accept the Hydra login request
	acceptLoginRequest := &hydra.AcceptLoginRequest{
		Subject:     authUser.ID.String(), // Use user ID as subject (consistent with regular login)
		Remember:    true,
		RememberFor: 3600, // 1 hour
		Context: map[string]any{
			"user_id":   authUser.ID.String(),
			"provider":  "google",
			"email":     authUser.Email,
			"tenant_id": authUser.TenantID.String(),
		},
	}

	s.logger.Info("Sending AcceptLoginRequest to Hydra",
		zap.String("challenge", loginChallenge[:min(50, len(loginChallenge))]),
		zap.String("subject", authUser.ID.String()),
		zap.String("email", authUser.Email),
		zap.String("tenant_id", authUser.TenantID.String()))

	acceptResp, err := acceptAuthenticatedLogin(s.hydraClient, s.authTimes, loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request",
			zap.Error(err),
			zap.String("challenge", loginChallenge[:min(50, len(loginChallenge))]))
		return s.endSignIn(c, loginChallenge, "server_error", msgSignInFailed)
	}

	s.logger.Info("Google OAuth login successful",
		zap.String("user_id", authUser.ID.String()),
		zap.String("email", authUser.Email),
		zap.String("provider", "google"),
		zap.String("redirect_to", acceptResp.RedirectTo))

	s.logSocialLogin(c, authUser, "google", map[string]any{
		"client_id": retrievedClientID,
	})

	// Return HTML page with JavaScript redirect to ensure proper browser navigation
	// This is more reliable than HTTP 302 redirect for cross-origin OAuth flows
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
        // Redirect to Hydra's OAuth endpoint with login_verifier
        window.location.href = "` + acceptResp.RedirectTo + `";
    </script>
</body>
</html>`

	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
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
		s.logger.Error("GitHub OAuth callback failed", zap.Error(err))
		s.logSocialLoginFailure(c, "github", "oauth_callback_failed", map[string]any{
			"client_id": retrievedClientID,
			"error":     err.Error(),
		})
		errCode, description := socialFailure(err)
		return s.endSignIn(c, loginChallenge, errCode, description)
	}

	if err := s.userService.UpdateLastLogin(authUser.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
	}

	acceptLoginRequest := &hydra.AcceptLoginRequest{
		Subject:     authUser.ID.String(),
		Remember:    true,
		RememberFor: 3600,
		Context: map[string]any{
			"user_id":   authUser.ID.String(),
			"provider":  "github",
			"email":     authUser.Email,
			"tenant_id": authUser.TenantID.String(),
		},
	}

	acceptResp, err := acceptAuthenticatedLogin(s.hydraClient, s.authTimes, loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return s.endSignIn(c, loginChallenge, "server_error", msgSignInFailed)
	}

	s.logger.Info("GitHub OAuth login successful",
		zap.String("user_id", authUser.ID.String()),
		zap.String("email", authUser.Email))

	s.logSocialLogin(c, authUser, "github", map[string]any{
		"client_id": retrievedClientID,
	})

	return s.renderRedirectPage(c, acceptResp.RedirectTo)
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
		s.logger.Error("Microsoft OAuth callback failed", zap.Error(err))
		s.logSocialLoginFailure(c, "microsoft", "oauth_callback_failed", map[string]any{
			"client_id": retrievedClientID,
			"error":     err.Error(),
		})
		errCode, description := socialFailure(err)
		return s.endSignIn(c, loginChallenge, errCode, description)
	}

	if err := s.userService.UpdateLastLogin(authUser.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
	}

	acceptLoginRequest := &hydra.AcceptLoginRequest{
		Subject:     authUser.ID.String(),
		Remember:    true,
		RememberFor: 3600,
		Context: map[string]any{
			"user_id":   authUser.ID.String(),
			"provider":  "microsoft",
			"email":     authUser.Email,
			"tenant_id": authUser.TenantID.String(),
		},
	}

	acceptResp, err := acceptAuthenticatedLogin(s.hydraClient, s.authTimes, loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return s.endSignIn(c, loginChallenge, "server_error", msgSignInFailed)
	}

	s.logger.Info("Microsoft OAuth login successful",
		zap.String("user_id", authUser.ID.String()),
		zap.String("email", authUser.Email))

	s.logSocialLogin(c, authUser, "microsoft", map[string]any{
		"client_id": retrievedClientID,
	})

	return s.renderRedirectPage(c, acceptResp.RedirectTo)
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
		s.logger.Error("Apple OAuth callback failed", zap.Error(err))
		s.logSocialLoginFailure(c, "apple", "oauth_callback_failed", map[string]any{
			"client_id": retrievedClientID,
			"error":     err.Error(),
		})
		errCode, description := socialFailure(err)
		return s.endSignIn(c, loginChallenge, errCode, description)
	}

	if err := s.userService.UpdateLastLogin(authUser.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
	}

	acceptLoginRequest := &hydra.AcceptLoginRequest{
		Subject:     authUser.ID.String(),
		Remember:    true,
		RememberFor: 3600,
		Context: map[string]any{
			"user_id":   authUser.ID.String(),
			"provider":  "apple",
			"email":     authUser.Email,
			"tenant_id": authUser.TenantID.String(),
		},
	}

	acceptResp, err := acceptAuthenticatedLogin(s.hydraClient, s.authTimes, loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return s.endSignIn(c, loginChallenge, "server_error", msgSignInFailed)
	}

	s.logger.Info("Apple OAuth login successful",
		zap.String("user_id", authUser.ID.String()),
		zap.String("email", authUser.Email))

	s.logSocialLogin(c, authUser, "apple", map[string]any{
		"client_id": retrievedClientID,
	})

	return s.renderRedirectPage(c, acceptResp.RedirectTo)
}

// ======================================
// Helper Methods
// ======================================

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
