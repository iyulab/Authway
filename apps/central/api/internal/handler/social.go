package handler

import (
	"context"
	"net/http"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
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
	stateStore       *OAuthStateStore
}

// NewSocialHandlerWithAllProviders creates a SocialHandler with all OAuth providers
func NewSocialHandlerWithAllProviders(
	googleService *social.GoogleService,
	githubService *social.GitHubService,
	microsoftService *social.MicrosoftService,
	appleService *social.AppleService,
	userService user.Service,
	hydraClient *hydra.Client,
	logger *zap.Logger,
	auditService audit.Service,
	stateStore *OAuthStateStore,
) *SocialHandler {
	return &SocialHandler{
		stateStore:       stateStore,
		googleService:    googleService,
		githubService:    githubService,
		microsoftService: microsoftService,
		appleService:     appleService,
		userService:      userService,
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

// GoogleLoginRequest for POST request body
type GoogleLoginRequest struct {
	LoginChallenge string `json:"login_challenge"`
	ClientID       string `json:"client_id"`
}

// GoogleLogin initiates Google OAuth flow
func (s *SocialHandler) GoogleLogin(c *fiber.Ctx) error {
	var loginChallenge, clientID string

	// Support both GET and POST methods to avoid HTTP 431 errors with long login_challenge
	if c.Method() == "POST" {
		// POST method: get parameters from body
		var req GoogleLoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":             "invalid_request_body",
				"error_description": "Failed to parse request body",
			})
		}
		loginChallenge = req.LoginChallenge
		clientID = req.ClientID
	} else {
		// GET method: get parameters from query string
		// IMPORTANT: Make copies of query strings because Fiber reuses internal buffers
		loginChallengeRaw := c.Query("login_challenge")
		clientIDRaw := c.Query("client_id")
		loginChallenge = string([]byte(loginChallengeRaw))
		clientID = string([]byte(clientIDRaw))
	}

	// Validate login_challenge
	if loginChallenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "missing_login_challenge",
			"error_description": "login_challenge parameter is required for OAuth flow",
			"hint":              "Include login_challenge in the URL or POST body",
			"example":           "POST /auth/google/login with body: {\"login_challenge\":\"...\",\"client_id\":\"...\"}",
		})
	}

	// If client_id is not provided, extract it from login_challenge
	if clientID == "" {
		loginReq, err := s.hydraClient.GetLoginRequest(loginChallenge)
		if err != nil {
			s.logger.Error("Failed to get login request from Hydra",
				zap.String("challenge", loginChallenge[:min(50, len(loginChallenge))]),
				zap.Error(err))
			return respondFlowLookupError(c, err)
		}
		clientID = loginReq.Client.ClientID
		s.logger.Info("Extracted client_id from login_challenge",
			zap.String("client_id", clientID))
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: loginChallenge, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to start sign-in",
		})
	}

	// Get Google authorization URL (client-specific or central)
	// Now using just the short state value instead of encoding all data
	authURL := s.googleService.GetAuthURLForClient(state, clientID)

	// Set state cookie for additional security
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HTTPOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: "Lax",
	})

	s.logger.Info("Initiating Google OAuth flow",
		zap.String("client_id", clientID),
		zap.String("login_challenge", loginChallenge))

	// For POST requests from fetch API, return JSON with redirect URL
	// (Cannot use HTTP redirect due to CORS with cross-origin OAuth providers)
	if c.Method() == "POST" {
		return c.JSON(fiber.Map{
			"redirect_url": authURL,
			"state":        state,
		})
	}

	// For GET requests (backward compatibility), use HTTP redirect
	return c.Redirect(authURL, http.StatusTemporaryRedirect)
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": c.Query("error_description"),
		})
	}

	// Validate required parameters
	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing required parameters",
		})
	}

	// Verify state against cookie (CSRF protection)
	// IMPORTANT: Make copy of cookie value because Fiber reuses internal buffers
	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		s.logger.Warn("State mismatch",
			zap.String("cookie_state", stateCookie),
			zap.String("param_state", state))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "state_mismatch",
			"error_description": "State parameter does not match",
		})
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to complete sign-in",
		})
	}
	if !found {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "This sign-in has expired or was already completed. Start again from the application.",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process Google OAuth callback",
			"details":           apierror.Message(err, "OAuth callback failed"),
			"hint":              "Verify Google OAuth configuration. Check client_id, client_secret, and redirect_uri in your environment variables.",
			"debug": fiber.Map{
				"client_id": retrievedClientID,
				"has_code":  len(code) > 0,
			},
			"possible_causes": []string{
				"Invalid Google OAuth credentials (CLIENT_ID or CLIENT_SECRET)",
				"Incorrect redirect_uri configuration",
				"Google API quota exceeded",
				"User denied permission",
			},
		})
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

	acceptResp, err := s.hydraClient.AcceptLoginRequest(loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request",
			zap.Error(err),
			zap.String("challenge", loginChallenge[:min(50, len(loginChallenge))]))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "hydra_login_failed",
			"error_description": "Failed to complete OAuth login with Hydra",
			"details":           apierror.Message(err, "unable to reach Hydra"),
			"hint":              "Verify Hydra is accessible and the login_challenge is still valid",
			"debug": fiber.Map{
				"hydra_admin_url": s.hydraClient.AdminURL,
				"challenge":       loginChallenge[:min(50, len(loginChallenge))] + "...",
				"user_id":         authUser.ID.String(),
			},
			"possible_causes": []string{
				"Hydra admin API is not accessible",
				"Login challenge expired or already used",
				"Network connectivity issue",
			},
		})
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

// GitHubLogin initiates GitHub OAuth flow
func (s *SocialHandler) GitHubLogin(c *fiber.Ctx) error {
	if s.githubService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "github_not_configured",
			"error_description": "GitHub OAuth is not configured",
		})
	}

	var loginChallenge, clientID string
	if c.Method() == "POST" {
		var req GoogleLoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":             "invalid_request_body",
				"error_description": "Failed to parse request body",
			})
		}
		loginChallenge = req.LoginChallenge
		clientID = req.ClientID
	} else {
		loginChallenge = string([]byte(c.Query("login_challenge")))
		clientID = string([]byte(c.Query("client_id")))
	}

	if loginChallenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "missing_login_challenge",
			"error_description": "login_challenge parameter is required for OAuth flow",
		})
	}

	if clientID == "" {
		loginReq, err := s.hydraClient.GetLoginRequest(loginChallenge)
		if err != nil {
			s.logger.Error("Failed to get login request from Hydra", zap.Error(err))
			return respondFlowLookupError(c, err)
		}
		clientID = loginReq.Client.ClientID
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: loginChallenge, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to start sign-in",
		})
	}

	authURL := s.githubService.GetAuthURLForClient(state, clientID)

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
	})

	s.logger.Info("Initiating GitHub OAuth flow",
		zap.String("client_id", clientID),
		zap.String("login_challenge", loginChallenge))

	if c.Method() == "POST" {
		return c.JSON(fiber.Map{
			"redirect_url": authURL,
			"state":        state,
		})
	}

	return c.Redirect(authURL, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the GitHub OAuth callback
func (s *SocialHandler) GitHubCallback(c *fiber.Ctx) error {
	if s.githubService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "github_not_configured",
			"error_description": "GitHub OAuth is not configured",
		})
	}

	code := string([]byte(c.Query("code")))
	state := string([]byte(c.Query("state")))
	errorParam := string([]byte(c.Query("error")))

	if errorParam != "" {
		s.logger.Warn("GitHub OAuth error", zap.String("error", errorParam))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": c.Query("error_description"),
		})
	}

	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing required parameters",
		})
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "state_mismatch",
			"error_description": "State parameter does not match",
		})
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to complete sign-in",
		})
	}
	if !found {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "This sign-in has expired or was already completed. Start again from the application.",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process GitHub OAuth callback",
			"details":           apierror.Message(err, "OAuth callback failed"),
		})
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

	acceptResp, err := s.hydraClient.AcceptLoginRequest(loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "hydra_login_failed",
			"error_description": "Failed to complete OAuth login with Hydra",
		})
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

// MicrosoftLogin initiates Microsoft OAuth flow
func (s *SocialHandler) MicrosoftLogin(c *fiber.Ctx) error {
	if s.microsoftService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "microsoft_not_configured",
			"error_description": "Microsoft OAuth is not configured",
		})
	}

	var loginChallenge, clientID string
	if c.Method() == "POST" {
		var req GoogleLoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":             "invalid_request_body",
				"error_description": "Failed to parse request body",
			})
		}
		loginChallenge = req.LoginChallenge
		clientID = req.ClientID
	} else {
		loginChallenge = string([]byte(c.Query("login_challenge")))
		clientID = string([]byte(c.Query("client_id")))
	}

	if loginChallenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "missing_login_challenge",
			"error_description": "login_challenge parameter is required for OAuth flow",
		})
	}

	if clientID == "" {
		loginReq, err := s.hydraClient.GetLoginRequest(loginChallenge)
		if err != nil {
			s.logger.Error("Failed to get login request from Hydra", zap.Error(err))
			return respondFlowLookupError(c, err)
		}
		clientID = loginReq.Client.ClientID
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: loginChallenge, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to start sign-in",
		})
	}

	authURL := s.microsoftService.GetAuthURLForClient(state, clientID)

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
	})

	s.logger.Info("Initiating Microsoft OAuth flow",
		zap.String("client_id", clientID),
		zap.String("login_challenge", loginChallenge))

	if c.Method() == "POST" {
		return c.JSON(fiber.Map{
			"redirect_url": authURL,
			"state":        state,
		})
	}

	return c.Redirect(authURL, http.StatusTemporaryRedirect)
}

// MicrosoftCallback handles the Microsoft OAuth callback
func (s *SocialHandler) MicrosoftCallback(c *fiber.Ctx) error {
	if s.microsoftService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "microsoft_not_configured",
			"error_description": "Microsoft OAuth is not configured",
		})
	}

	code := string([]byte(c.Query("code")))
	state := string([]byte(c.Query("state")))
	errorParam := string([]byte(c.Query("error")))

	if errorParam != "" {
		s.logger.Warn("Microsoft OAuth error", zap.String("error", errorParam))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": c.Query("error_description"),
		})
	}

	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing required parameters",
		})
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "state_mismatch",
			"error_description": "State parameter does not match",
		})
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to complete sign-in",
		})
	}
	if !found {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "This sign-in has expired or was already completed. Start again from the application.",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process Microsoft OAuth callback",
			"details":           apierror.Message(err, "OAuth callback failed"),
		})
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

	acceptResp, err := s.hydraClient.AcceptLoginRequest(loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "hydra_login_failed",
			"error_description": "Failed to complete OAuth login with Hydra",
		})
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

// AppleLogin initiates Apple OAuth flow
func (s *SocialHandler) AppleLogin(c *fiber.Ctx) error {
	if s.appleService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "apple_not_configured",
			"error_description": "Apple OAuth is not configured",
		})
	}

	var loginChallenge, clientID string
	if c.Method() == "POST" {
		var req GoogleLoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":             "invalid_request_body",
				"error_description": "Failed to parse request body",
			})
		}
		loginChallenge = req.LoginChallenge
		clientID = req.ClientID
	} else {
		loginChallenge = string([]byte(c.Query("login_challenge")))
		clientID = string([]byte(c.Query("client_id")))
	}

	if loginChallenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "missing_login_challenge",
			"error_description": "login_challenge parameter is required for OAuth flow",
		})
	}

	if clientID == "" {
		loginReq, err := s.hydraClient.GetLoginRequest(loginChallenge)
		if err != nil {
			s.logger.Error("Failed to get login request from Hydra", zap.Error(err))
			return respondFlowLookupError(c, err)
		}
		clientID = loginReq.Client.ClientID
	}

	state, err := s.stateStore.Save(c.Context(), oauthState{LoginChallenge: loginChallenge, ClientID: clientID})
	if err != nil {
		s.logger.Error("Failed to store OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to start sign-in",
		})
	}

	authURL := s.appleService.GetAuthURLForClient(state, clientID)

	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
	})

	s.logger.Info("Initiating Apple OAuth flow",
		zap.String("client_id", clientID),
		zap.String("login_challenge", loginChallenge))

	if c.Method() == "POST" {
		return c.JSON(fiber.Map{
			"redirect_url": authURL,
			"state":        state,
		})
	}

	return c.Redirect(authURL, http.StatusTemporaryRedirect)
}

// AppleCallback handles the Apple OAuth callback (POST because of form_post response_mode)
func (s *SocialHandler) AppleCallback(c *fiber.Ctx) error {
	if s.appleService == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":             "apple_not_configured",
			"error_description": "Apple OAuth is not configured",
		})
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": c.FormValue("error_description"),
		})
	}

	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing required parameters",
		})
	}

	stateCookie := string([]byte(c.Cookies("oauth_state")))
	if stateCookie != state {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "state_mismatch",
			"error_description": "State parameter does not match",
		})
	}

	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil {
		s.logger.Error("Failed to read OAuth state", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to complete sign-in",
		})
	}
	if !found {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "This sign-in has expired or was already completed. Start again from the application.",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process Apple OAuth callback",
			"details":           apierror.Message(err, "OAuth callback failed"),
		})
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

	acceptResp, err := s.hydraClient.AcceptLoginRequest(loginChallenge, acceptLoginRequest)
	if err != nil {
		s.logger.Error("Failed to accept Hydra login request", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "hydra_login_failed",
			"error_description": "Failed to complete OAuth login with Hydra",
		})
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
