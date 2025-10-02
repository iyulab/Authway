package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"authway/src/server/internal/hydra"
	"authway/src/server/internal/service/social"
	"authway/src/server/pkg/user"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

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

// GoogleLogin initiates Google OAuth flow
func (s *SocialHandler) GoogleLogin(c *fiber.Ctx) error {
	// Get login challenge from query parameters
	loginChallenge := c.Query("login_challenge")
	if loginChallenge == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "missing_login_challenge",
			"error_description": "login_challenge parameter is required",
		})
	}

	// Get client_id from query parameters (optional for hybrid OAuth)
	clientID := c.Query("client_id")

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

	// Store login challenge and client_id in session/cache with state as key
	// In production, use Redis or similar for state storage
	// For now, we'll include it in the state parameter (base64 encoded)
	stateData := fmt.Sprintf("%s:%s:%s", state, loginChallenge, clientID)
	encodedState := base64.URLEncoding.EncodeToString([]byte(stateData))

	// Get Google authorization URL (client-specific or central)
	authURL := s.googleService.GetAuthURLForClient(encodedState, clientID)

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

	return c.Redirect(authURL, http.StatusTemporaryRedirect)
}

// GoogleCallback handles the Google OAuth callback
func (s *SocialHandler) GoogleCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	encodedState := c.Query("state")
	errorParam := c.Query("error")

	// Check for OAuth error
	if errorParam != "" {
		s.logger.Warn("Google OAuth error", zap.String("error", errorParam))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": c.Query("error_description"),
		})
	}

	// Validate required parameters
	if code == "" || encodedState == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing required parameters",
		})
	}

	// Decode state to extract original state, login challenge, and client_id
	stateData, err := base64.URLEncoding.DecodeString(encodedState)
	if err != nil {
		s.logger.Error("Failed to decode state parameter", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "Invalid state parameter",
		})
	}

	stateParts := strings.Split(string(stateData), ":")
	// Extract state, login challenge, and client_id (format: "state:login_challenge:client_id")
	var originalState, loginChallenge, clientID string
	if len(stateParts) >= 2 {
		originalState = stateParts[0]
		loginChallenge = stateParts[1]
		if len(stateParts) >= 3 {
			clientID = stateParts[2]
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_state",
			"error_description": "Malformed state parameter",
		})
	}

	// Verify state against cookie (CSRF protection)
	stateCookie := c.Cookies("oauth_state")
	if stateCookie != originalState {
		s.logger.Warn("State mismatch",
			zap.String("cookie_state", stateCookie),
			zap.String("param_state", originalState))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "state_mismatch",
			"error_description": "State parameter does not match",
		})
	}

	// Clear the state cookie
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
	})

	// Process Google OAuth callback (client-specific or central)
	authUser, err := s.googleService.HandleCallbackForClient(c.Context(), code, originalState, clientID)
	if err != nil {
		s.logger.Error("Google OAuth callback failed",
			zap.Error(err),
			zap.String("client_id", clientID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":             "oauth_callback_failed",
			"error_description": "Failed to process Google OAuth callback",
		})
	}

	// Update last login time using the service
	if err := s.userService.UpdateLastLogin(authUser.ID); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err))
		// Continue despite error as user is authenticated
	}

	// Accept the Hydra login request
	acceptLoginRequest := &hydra.AcceptLoginRequest{
		Subject:     authUser.Email,
		Remember:    true,
		RememberFor: 3600, // 1 hour
		Context: map[string]interface{}{
			"user_id":  authUser.ID.String(),
			"provider": "google",
			"email":    authUser.Email,
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

	s.logger.Info("Google OAuth login successful",
		zap.String("user_id", authUser.ID.String()),
		zap.String("email", authUser.Email),
		zap.String("provider", "google"))

	// Redirect to Hydra consent flow
	return c.Redirect(acceptResp.RedirectTo, http.StatusFound)
}

// GetGoogleAuthURL returns the Google OAuth URL for frontend use
func (s *SocialHandler) GetGoogleAuthURL(c *fiber.Ctx) error {
	// Get client_id from query parameters (optional for hybrid OAuth)
	clientID := c.Query("client_id")

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
