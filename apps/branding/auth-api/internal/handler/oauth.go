package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"authway/apps/branding/auth-api/internal/service"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type OAuthHandler struct {
	googleService  *service.GoogleService
	centralAPI     *service.CentralAPIClient
	hydraClient    *service.HydraClient
	logger         *zap.Logger
	stateStore     *StateStore
}

func NewOAuthHandler(
	googleService *service.GoogleService,
	centralAPI *service.CentralAPIClient,
	hydraClient *service.HydraClient,
	logger *zap.Logger,
) *OAuthHandler {
	return &OAuthHandler{
		googleService:  googleService,
		centralAPI:     centralAPI,
		hydraClient:    hydraClient,
		logger:         logger,
		stateStore:     NewStateStore(),
	}
}

// StateData represents the data stored server-side for OAuth state
type StateData struct {
	LoginChallenge string
	ClientID       string
	CreatedAt      time.Time
}

// StateStore manages OAuth state data with automatic cleanup
type StateStore struct {
	mu     sync.RWMutex
	states map[string]*StateData
}

func NewStateStore() *StateStore {
	store := &StateStore{
		states: make(map[string]*StateData),
	}
	// Start cleanup goroutine
	go store.cleanupExpired()
	return store
}

func (s *StateStore) Set(state string, data *StateData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = data
}

func (s *StateStore) Get(state string) (*StateData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, exists := s.states[state]
	return data, exists
}

func (s *StateStore) Delete(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, state)
}

func (s *StateStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for state, data := range s.states {
			if now.Sub(data.CreatedAt) > 10*time.Minute {
				delete(s.states, state)
			}
		}
		s.mu.Unlock()
	}
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// truncateString safely truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GoogleLoginRequest is the request body for initiating Google OAuth
type GoogleLoginRequest struct {
	LoginChallenge string `json:"login_challenge"`
	ClientID       string `json:"client_id"`
}

// GoogleLoginGet handles GET requests for initiating Google OAuth flow
func (h *OAuthHandler) GoogleLoginGet(c *fiber.Ctx) error {
	loginChallenge := c.Query("login_challenge")
	if loginChallenge == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "login_challenge query parameter is required",
		})
	}

	// Fetch client_id from Hydra using the login_challenge
	loginReq, err := h.hydraClient.GetLoginRequest(loginChallenge)
	if err != nil {
		h.logger.Error("Failed to get login request from Hydra", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch login request details",
		})
	}

	var clientID string
	var clientName string
	var requestedScope []string

	if loginReq.Client != nil {
		clientID = loginReq.Client.ClientID
		clientName = loginReq.Client.ClientName
	}

	if loginReq.RequestedScope != nil {
		requestedScope = loginReq.RequestedScope
	}

	// Fetch client auth configuration from Central API
	clientConfig := fiber.Map{
		"client_id": clientID,
	}

	if clientID != "" {
		authConfig, err := h.centralAPI.GetClientByClientID(c.Context(), clientID)
		if err != nil {
			h.logger.Warn("Failed to fetch client auth config from Central API, using defaults",
				zap.Error(err),
				zap.String("client_id", clientID))
			// Use defaults if Central API is not available
			clientConfig["enabled_auth_providers"] = []string{"email", "google"}
			clientConfig["allow_email_signup"] = true
			clientConfig["allow_email_login"] = true
			clientConfig["google_oauth_enabled"] = true
			clientConfig["github_oauth_enabled"] = false
			clientConfig["microsoft_oauth_enabled"] = false
			clientConfig["apple_oauth_enabled"] = false
		} else {
			clientConfig["enabled_auth_providers"] = authConfig.EnabledAuthProviders
			clientConfig["allow_email_signup"] = authConfig.AllowEmailSignup
			clientConfig["allow_email_login"] = authConfig.AllowEmailLogin
			clientConfig["google_oauth_enabled"] = authConfig.GoogleOAuthEnabled
			clientConfig["github_oauth_enabled"] = authConfig.GithubOAuthEnabled
			clientConfig["microsoft_oauth_enabled"] = authConfig.MicrosoftOAuthEnabled
			clientConfig["apple_oauth_enabled"] = authConfig.AppleOAuthEnabled
		}
	}

	// Return login page information
	return c.JSON(fiber.Map{
		"challenge":       loginChallenge,
		"client_name":     clientName,
		"requested_scope": requestedScope,
		"client":          clientConfig,
	})
}

// GoogleLogin initiates Google OAuth flow
func (h *OAuthHandler) GoogleLogin(c *fiber.Ctx) error {
	var req GoogleLoginRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error("Failed to parse login request", zap.Error(err))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.LoginChallenge == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "login_challenge is required",
		})
	}

	// If client_id is not provided, fetch it from Hydra using the login_challenge
	clientID := req.ClientID
	if clientID == "" {
		loginReq, err := h.hydraClient.GetLoginRequest(req.LoginChallenge)
		if err != nil {
			h.logger.Error("Failed to get login request from Hydra", zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch login request details",
			})
		}
		if loginReq.Client != nil {
			clientID = loginReq.Client.ClientID
		} else {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Could not determine client_id from login_challenge",
			})
		}
	}

	// Generate short random state
	state, err := generateState()
	if err != nil {
		h.logger.Error("Failed to generate state", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate state",
		})
	}

	// Store state data server-side
	stateData := &StateData{
		LoginChallenge: req.LoginChallenge,
		ClientID:       clientID,
		CreatedAt:      time.Now(),
	}
	h.stateStore.Set(state, stateData)

	// Generate Google OAuth URL
	authURL := h.googleService.GetAuthURL(state)

	h.logger.Info("Generated Google OAuth URL",
		zap.String("client_id", clientID),
		zap.String("state", state),
		zap.String("login_challenge", truncateString(req.LoginChallenge, 10)))

	return c.JSON(fiber.Map{
		"redirect_url": authURL,
		"state":        state,
	})
}

// GoogleCallback handles the OAuth callback from Google
func (h *OAuthHandler) GoogleCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")

	// Handle OAuth error from Google
	if errorParam != "" {
		h.logger.Error("Google OAuth error",
			zap.String("error", errorParam),
			zap.String("description", c.Query("error_description")))
		return c.SendString(fmt.Sprintf(`
			<!DOCTYPE html>
			<html>
			<head><title>Authentication Error</title></head>
			<body>
				<h1>Authentication Failed</h1>
				<p>Error: %s</p>
				<p>%s</p>
			</body>
			</html>
		`, errorParam, c.Query("error_description")))
	}

	if code == "" || state == "" {
		h.logger.Error("Missing code or state parameter")
		return c.Status(http.StatusBadRequest).SendString("Missing code or state parameter")
	}

	// Retrieve state data from server-side store
	stateData, exists := h.stateStore.Get(state)
	if !exists {
		h.logger.Error("State not found or expired", zap.String("state", state))
		return c.Status(http.StatusBadRequest).SendString("Invalid or expired state parameter")
	}

	// Delete state after retrieval (one-time use)
	h.stateStore.Delete(state)

	h.logger.Info("Processing Google callback",
		zap.String("client_id", stateData.ClientID),
		zap.String("state", state),
		zap.String("login_challenge", truncateString(stateData.LoginChallenge, 10)))

	// Exchange code for token
	tokenResp, err := h.googleService.ExchangeCode(c.Context(), code)
	if err != nil {
		h.logger.Error("Failed to exchange code", zap.Error(err))
		return c.Status(http.StatusInternalServerError).SendString("Failed to exchange authorization code")
	}

	// Get user info from Google
	userInfo, err := h.googleService.GetUserInfo(c.Context(), tokenResp.AccessToken)
	if err != nil {
		h.logger.Error("Failed to get user info", zap.Error(err))
		return c.Status(http.StatusInternalServerError).SendString("Failed to retrieve user information")
	}

	h.logger.Info("Retrieved Google user info",
		zap.String("email", userInfo.Email),
		zap.String("name", userInfo.Name))

	// Call Central API to authenticate/create user
	authReq := &service.AuthenticateGoogleUserRequest{
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		GoogleID: userInfo.ID,
		Picture:  userInfo.Picture,
		ClientID: stateData.ClientID,
	}

	authResp, err := h.centralAPI.AuthenticateGoogleUser(c.Context(), authReq)
	if err != nil {
		h.logger.Error("Failed to authenticate with Central API", zap.Error(err))

		// Reject Hydra login
		rejectResp, rejectErr := h.hydraClient.RejectLoginRequest(
			stateData.LoginChallenge,
			"authentication_failed",
			"Failed to authenticate user with central API",
		)
		if rejectErr != nil {
			h.logger.Error("Failed to reject Hydra login", zap.Error(rejectErr))
			return c.Status(http.StatusInternalServerError).SendString("Authentication failed")
		}

		return c.Redirect(rejectResp.RedirectTo, http.StatusFound)
	}

	h.logger.Info("User authenticated successfully",
		zap.String("user_id", authResp.UserID),
		zap.String("tenant_id", authResp.TenantID))

	// Accept Hydra login request
	acceptLoginRequest := &service.AcceptLoginRequest{
		Subject:     authResp.UserID,
		Remember:    true,
		RememberFor: 3600,
		Context: map[string]interface{}{
			"email":     authResp.Email,
			"tenant_id": authResp.TenantID,
		},
	}

	acceptResp, err := h.hydraClient.AcceptLoginRequest(stateData.LoginChallenge, acceptLoginRequest)
	if err != nil {
		h.logger.Error("Failed to accept Hydra login", zap.Error(err))
		return c.Status(http.StatusInternalServerError).SendString("Failed to complete authentication")
	}

	h.logger.Info("Login accepted, redirecting to Hydra",
		zap.String("redirect_to", acceptResp.RedirectTo))

	// Use proper HTTP redirect to maintain session continuity with Hydra
	return c.Redirect(acceptResp.RedirectTo, http.StatusFound)
}
