package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"authway/src/server/internal/hydra"
	"authway/src/server/internal/service/social"
	"authway/src/server/pkg/user"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// oauthStateData stores OAuth state information server-side to avoid large URLs
type oauthStateData struct {
	LoginChallenge string
	ClientID       string
	CreatedAt      time.Time
}

// oauthStateStore is a thread-safe in-memory store for OAuth state data
// Using sync.Map for concurrent access safety
// In production, use Redis or similar distributed cache
var oauthStateStore sync.Map

// cleanExpiredStates removes OAuth states older than 15 minutes
func cleanExpiredStates() {
	now := time.Now()
	oauthStateStore.Range(func(key, value interface{}) bool {
		data := value.(*oauthStateData)
		if now.Sub(data.CreatedAt) > 15*time.Minute {
			oauthStateStore.Delete(key)
		}
		return true // continue iteration
	})
}

type SocialHandler struct {
	googleService *social.GoogleService
	userService   user.Service
	hydraClient   *hydra.Client
	logger        *zap.Logger
}

func NewSocialHandler(
	googleService *social.GoogleService,
	userService user.Service,
	hydraClient *hydra.Client,
	logger *zap.Logger,
) *SocialHandler {
	return &SocialHandler{
		googleService: googleService,
		userService:   userService,
		hydraClient:   hydraClient,
		logger:        logger,
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

	// Generate state parameter for CSRF protection
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		s.logger.Error("Failed to generate state parameter", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "internal_server_error",
			"error_description": "Failed to generate secure state parameter",
		})
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Store login challenge and client_id server-side to avoid large URLs
	// This prevents HTTP 431 errors with long Hydra login_challenge values
	// Create a new struct to avoid any reference issues
	stateInfo := &oauthStateData{
		LoginChallenge: loginChallenge,
		ClientID:       clientID,
		CreatedAt:      time.Now(),
	}

	// Store in thread-safe sync.Map
	oauthStateStore.Store(state, stateInfo)

	// Debug: log what we're storing
	s.logger.Info("Stored OAuth state",
		zap.String("state", state),
		zap.String("stored_client_id", stateInfo.ClientID),
		zap.String("stored_challenge_prefix", stateInfo.LoginChallenge[:min(20, len(stateInfo.LoginChallenge))]),
		zap.Int("challenge_length", len(stateInfo.LoginChallenge)))

	// Immediately verify what was stored
	verifyValue, verifyFound := oauthStateStore.Load(state)
	if verifyFound {
		verifyData := verifyValue.(*oauthStateData)
		s.logger.Info("Verification: Immediately after storage",
			zap.String("verify_client_id", verifyData.ClientID),
			zap.String("verify_challenge_prefix", verifyData.LoginChallenge[:min(20, len(verifyData.LoginChallenge))]),
			zap.Bool("matches_stored", verifyData.ClientID == stateInfo.ClientID))
	}

	// Clean up expired states (synchronous, lightweight operation)
	cleanExpiredStates()

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

	// Debug: Check all keys in the map before retrieval
	keyCount := 0
	oauthStateStore.Range(func(key, value interface{}) bool {
		keyCount++
		keyStr := key.(string)
		data := value.(*oauthStateData)
		s.logger.Info("Map contains entry",
			zap.String("map_key", keyStr[:min(20, len(keyStr))]),
			zap.String("map_client_id", data.ClientID),
			zap.String("map_challenge_prefix", data.LoginChallenge[:min(20, len(data.LoginChallenge))]),
			zap.Bool("is_target_key", keyStr == state))
		return true
	})
	s.logger.Info("Total keys in map", zap.Int("key_count", keyCount))

	// Retrieve stored state data from server-side storage
	value, found := oauthStateStore.Load(state)
	if !found {
		s.logger.Warn("State not found in storage", zap.String("state", state))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "OAuth state not found or expired",
			"hint":              "The OAuth state parameter has expired (15 min timeout) or was already used. Please restart the login flow.",
			"possible_causes": []string{
				"State expired after 15 minutes",
				"State was already used (duplicate callback)",
				"Server restarted and in-memory state was cleared",
			},
			"solution": "Return to your application and click login again",
		})
	}

	// Type assert the retrieved value
	stateData := value.(*oauthStateData)
	loginChallenge := stateData.LoginChallenge
	retrievedClientID := stateData.ClientID

	// Debug: log what we retrieved
	s.logger.Info("Retrieved OAuth state",
		zap.String("state", state),
		zap.String("retrieved_client_id", retrievedClientID),
		zap.String("retrieved_challenge_prefix", loginChallenge[:min(20, len(loginChallenge))]),
		zap.Int("challenge_length", len(loginChallenge)))

	// Clean up used state from storage
	oauthStateStore.Delete(state)

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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process Google OAuth callback",
			"details":           err.Error(),
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
		Context: map[string]interface{}{
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
			"details":           err.Error(),
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

	// Hydra's AcceptLoginRequest returns a redirect_to URL that contains a login_verifier
	// This URL should be redirected to (browser → Hydra → consent page)
	// Hydra will then redirect to the consent page with the consent_challenge parameter
	return c.Redirect(acceptResp.RedirectTo, http.StatusFound)
}

// GetGoogleAuthURL returns the Google OAuth URL for frontend use
func (s *SocialHandler) GetGoogleAuthURL(c *fiber.Ctx) error {
	// Get client_id from query parameters (optional for hybrid OAuth)
	// IMPORTANT: Make copy of query string because Fiber reuses internal buffers
	clientID := string([]byte(c.Query("client_id")))

	// Generate state parameter
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate state parameter",
		})
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Get the authorization URL (client-specific or central)
	authURL := s.googleService.GetAuthURLForClient(state, clientID)

	response := fiber.Map{
		"auth_url": authURL,
		"state":    state,
	}

	// Include client info if specified
	if clientID != "" {
		response["client_id"] = clientID
		response["oauth_type"] = "client_specific"
	} else {
		response["oauth_type"] = "central"
	}

	return c.JSON(response)
}
