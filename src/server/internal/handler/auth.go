package handler

import (
	"authway/src/server/internal/hydra"
	"authway/src/server/pkg/client"
	"authway/src/server/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userService   user.Service
	clientService client.Service
	hydraClient   *hydra.Client
	logger        *zap.Logger
}

func NewAuthHandler(userService user.Service, clientService client.Service, hydraClient *hydra.Client, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		userService:   userService,
		clientService: clientService,
		hydraClient:   hydraClient,
		logger:        logger,
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LoginPageRequest for POST request body
type LoginPageRequest struct {
	LoginChallenge string `json:"login_challenge"`
}

// Login flow handler - supports both GET and POST
func (h *AuthHandler) LoginPage(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("login_challenge")

	// If not in query, try POST body
	if challenge == "" && c.Method() == "POST" {
		var req LoginPageRequest
		if err := c.BodyParser(&req); err == nil {
			challenge = req.LoginChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "login_challenge parameter is required",
			"hint":  "The login_challenge parameter must be included in the URL query string or POST body. This parameter is provided by Ory Hydra in the OAuth 2.0 authorization flow.",
			"docs":  "https://www.ory.sh/docs/hydra/guides/login",
		})
	}

	// Get login request from Hydra
	h.logger.Info("Getting login request from Hydra", zap.String("challenge", challenge))
	loginReq, err := h.hydraClient.GetLoginRequest(challenge)
	if err != nil {
		h.logger.Error("Failed to get login request from Hydra",
			zap.String("challenge", challenge),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get login request from Hydra",
			"details": err.Error(),
			"hint":    "Verify that Ory Hydra is running and accessible. Check the HYDRA_ADMIN_URL environment variable.",
			"debug": fiber.Map{
				"hydra_admin_url": h.hydraClient.AdminURL,
				"challenge":       challenge[:min(50, len(challenge))] + "...",
			},
		})
	}

	// Get client information to check tenant
	h.logger.Info("Looking for client", zap.String("client_id", loginReq.Client.ClientID))
	requestedClient, err := h.clientService.GetByClientID(loginReq.Client.ClientID)
	if err != nil {
		h.logger.Error("Failed to get client information",
			zap.String("client_id", loginReq.Client.ClientID),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "OAuth client not registered in Authway",
			"details": err.Error(),
			"hint":    "Register this OAuth client in Authway Admin Console before using it. Each client must be associated with a tenant.",
			"solution": fiber.Map{
				"step_1": "Go to Admin Console: http://localhost:3000",
				"step_2": "Navigate to Clients section",
				"step_3": "Register client with client_id: " + loginReq.Client.ClientID,
			},
			"client_id": loginReq.Client.ClientID,
		})
	}

	// SSO Check: If user is already authenticated, verify tenant match
	if loginReq.Skip && loginReq.Subject != "" {
		userID, err := uuid.Parse(loginReq.Subject)
		if err != nil {
			// Invalid user ID format - revoke sessions and force fresh login
			h.logger.Warn("Invalid user ID in skip request, revoking sessions",
				zap.String("subject", loginReq.Subject),
				zap.Error(err))
			// Revoke all sessions for this subject
			if revokeErr := h.hydraClient.RevokeUserSessions(loginReq.Subject); revokeErr != nil {
				h.logger.Error("Failed to revoke user sessions", zap.Error(revokeErr))
			}
			// Reject with login_required to show login form without propagating error to OAuth client
			resp, rejectErr := h.hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")
			if rejectErr != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to reject login request",
				})
			}
			// Return JSON response with redirect_to for frontend to handle
			return c.JSON(fiber.Map{
				"redirect_to":     resp.RedirectTo,
				"session_cleared": true,
			})
		}

		// Get user to check tenant
		authenticatedUser, err := h.userService.GetByID(userID)
		if err != nil {
			// User not found - revoke sessions and force fresh login
			h.logger.Warn("User not found in skip request, revoking sessions",
				zap.String("user_id", userID.String()),
				zap.Error(err))
			// Revoke all sessions for this subject
			if revokeErr := h.hydraClient.RevokeUserSessions(userID.String()); revokeErr != nil {
				h.logger.Error("Failed to revoke user sessions", zap.Error(revokeErr))
			}
			// Reject with login_required to show login form without propagating error to OAuth client
			resp, rejectErr := h.hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")
			if rejectErr != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to reject login request",
				})
			}
			// Return JSON response with redirect_to for frontend to handle
			return c.JSON(fiber.Map{
				"redirect_to":     resp.RedirectTo,
				"session_cleared": true,
			})
		}

		// Compare tenant_id for SSO eligibility
		if authenticatedUser.TenantID == requestedClient.TenantID {
			// Same tenant → SSO automatic approval
			h.logger.Info("SSO approved - same tenant",
				zap.String("user_id", authenticatedUser.ID.String()),
				zap.String("tenant_id", authenticatedUser.TenantID.String()))
			acceptBody := &hydra.AcceptLoginRequest{
				Subject:     loginReq.Subject,
				Remember:    true,
				RememberFor: 3600,
				Context: map[string]interface{}{
					"email":     authenticatedUser.Email,
					"name":      authenticatedUser.Name,
					"tenant_id": authenticatedUser.TenantID.String(),
				},
			}

			resp, err := h.hydraClient.AcceptLoginRequest(challenge, acceptBody)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to accept login request",
				})
			}

			// Return JSON response for SSO auto-login
			return c.JSON(fiber.Map{
				"redirect_to": resp.RedirectTo,
				"sso":         true,
			})
		}
		// Different tenant → Force re-authentication by showing login form
		h.logger.Info("Different tenant - forcing re-authentication",
			zap.String("user_tenant_id", authenticatedUser.TenantID.String()),
			zap.String("client_tenant_id", requestedClient.TenantID.String()))
	}

	// Render login form with challenge
	return c.JSON(fiber.Map{
		"challenge":       challenge,
		"client_name":     loginReq.Client.ClientName,
		"requested_scope": loginReq.RequestedScope,
		"tenant_id":       requestedClient.TenantID.String(),
		"client": fiber.Map{
			"client_id": loginReq.Client.ClientID,
		},
	})
}

type LoginRequest struct {
	Challenge string `json:"challenge"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Remember  bool   `json:"remember"`
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get login request from Hydra
	_, err := h.hydraClient.GetLoginRequest(req.Challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get login request",
		})
	}

	// Authenticate user
	user, err := h.userService.GetByEmail(req.Email)
	if err != nil {
		// Reject login request
		resp, _ := h.hydraClient.RejectLoginRequest(req.Challenge, "invalid_credentials", "Invalid email or password")
		return c.JSON(fiber.Map{
			"error":       "Invalid email or password",
			"redirect_to": resp.RedirectTo,
		})
	}

	// Verify password
	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		// Reject login request
		resp, _ := h.hydraClient.RejectLoginRequest(req.Challenge, "invalid_credentials", "Invalid email or password")
		return c.JSON(fiber.Map{
			"error":       "Invalid email or password",
			"redirect_to": resp.RedirectTo,
		})
	}

	// Accept login request
	rememberFor := 0
	if req.Remember {
		rememberFor = 3600 // 1 hour
	}

	acceptBody := &hydra.AcceptLoginRequest{
		Subject:     user.ID.String(),
		Remember:    req.Remember,
		RememberFor: rememberFor,
		Context: map[string]interface{}{
			"email":     user.Email,
			"name":      user.Name,
			"tenant_id": user.TenantID.String(),
		},
	}

	resp, err := h.hydraClient.AcceptLoginRequest(req.Challenge, acceptBody)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to accept login request",
		})
	}

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// ConsentPageRequest for POST request body
type ConsentPageRequest struct {
	ConsentChallenge string `json:"consent_challenge" form:"consent_challenge"`
}

// Consent flow handler - supports both GET and POST
func (h *AuthHandler) ConsentPage(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("consent_challenge")

	// If not in query, try POST body (supports both JSON and form-urlencoded)
	if challenge == "" && c.Method() == "POST" {
		var req ConsentPageRequest
		// BodyParser supports both JSON and form-urlencoded automatically
		if err := c.BodyParser(&req); err == nil {
			challenge = req.ConsentChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
			"hint":  "The consent_challenge parameter must be included in the URL query string or POST body. This parameter is provided by Ory Hydra after successful login.",
			"docs":  "https://www.ory.sh/docs/hydra/guides/consent",
		})
	}

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to get consent request from Hydra",
			"details": err.Error(),
			"hint":    "Verify that Ory Hydra is running and the consent_challenge is valid. The challenge may have expired or been used already.",
			"debug": fiber.Map{
				"hydra_admin_url": h.hydraClient.AdminURL,
				"challenge":       challenge[:min(50, len(challenge))] + "...",
			},
		})
	}

	// Get user information
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		h.logger.Error("Invalid user ID in consent request",
			zap.String("subject", consentReq.Subject),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "Invalid user ID format in consent request",
			"details": err.Error(),
			"hint":    "The subject (user ID) in the consent request is not a valid UUID. This may indicate a session corruption issue.",
			"subject": consentReq.Subject,
		})
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		h.logger.Error("User not found in consent flow",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error":   "User not found",
			"details": err.Error(),
			"hint":    "The authenticated user no longer exists in the database. This may happen if the user was deleted after login but before consent.",
			"user_id": userID.String(),
		})
	}

	return c.JSON(fiber.Map{
		"challenge":       challenge,
		"client_name":     consentReq.Client.ClientName,
		"requested_scope": consentReq.RequestedScope,
		"user": fiber.Map{
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

type ConsentRequest struct {
	Challenge   string   `json:"challenge"`
	GrantScope  []string `json:"grant_scope"`
	Remember    bool     `json:"remember"`
	RememberFor int      `json:"remember_for"`
}

func (h *AuthHandler) Consent(c *fiber.Ctx) error {
	var req ConsentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(req.Challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get consent request",
		})
	}

	// Get user information for session
	userID, err := uuid.Parse(consentReq.Subject)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}
	user, err := h.userService.GetByID(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get user information",
		})
	}

	// Accept consent request
	acceptBody := &hydra.AcceptConsentRequest{
		GrantScope:               req.GrantScope,
		GrantAccessTokenAudience: consentReq.RequestedAudience,
		Remember:                 req.Remember,
		RememberFor:              req.RememberFor,
		Session: &hydra.ConsentSession{
			AccessToken: map[string]interface{}{
				"email":     user.Email,
				"name":      user.Name,
				"tenant_id": user.TenantID.String(),
			},
			IDToken: map[string]interface{}{
				"email":          user.Email,
				"name":           user.Name,
				"email_verified": user.EmailVerified,
				"tenant_id":      user.TenantID.String(),
			},
		},
	}

	// Log detailed consent request data
	h.logger.Info("Sending consent accept to Hydra",
		zap.String("challenge", req.Challenge),
		zap.Strings("grant_scope", req.GrantScope),
		zap.Strings("grant_access_token_audience", consentReq.RequestedAudience),
		zap.Bool("remember", req.Remember),
		zap.Int("remember_for", req.RememberFor),
		zap.String("user_id", user.ID.String()),
		zap.String("tenant_id", user.TenantID.String()))

	resp, err := h.hydraClient.AcceptConsentRequest(req.Challenge, acceptBody)
	if err != nil {
		h.logger.Error("Failed to accept consent request",
			zap.Error(err),
			zap.String("challenge", req.Challenge))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to accept consent request",
		})
	}

	h.logger.Info("Consent accepted, redirecting",
		zap.String("redirect_to", resp.RedirectTo),
		zap.String("user_id", user.ID.String()))

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// RejectConsentRequest for POST request body
type RejectConsentRequest struct {
	ConsentChallenge string `json:"consent_challenge" form:"consent_challenge"`
}

func (h *AuthHandler) RejectConsent(c *fiber.Ctx) error {
	// Try to get challenge from query parameter first (GET)
	challenge := c.Query("consent_challenge")

	// If not in query, try POST body (supports both JSON and form-urlencoded)
	if challenge == "" && c.Method() == "POST" {
		var req RejectConsentRequest
		if err := c.BodyParser(&req); err == nil {
			challenge = req.ConsentChallenge
		}
	}

	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
			"hint":  "The consent_challenge parameter must be included in the URL query string or POST body",
		})
	}

	resp, err := h.hydraClient.RejectConsentRequest(challenge, "access_denied", "User denied consent")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to reject consent request",
		})
	}

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

// Registration endpoint
type RegisterRequest struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	if req.TenantID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant ID is required",
		})
	}

	// Parse tenant ID
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid tenant ID format",
		})
	}

	// Create user request
	createReq := &user.CreateUserRequest{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
	}

	createdUser, err := h.userService.Create(tenantID, createReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":        createdUser.ID,
		"tenant_id": createdUser.TenantID,
		"email":     createdUser.Email,
		"name":      createdUser.Name,
	})
}

// User profile endpoint
func (h *AuthHandler) Profile(c *fiber.Ctx) error {
	userID := c.Params("id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid user ID format",
		})
	}
	user, err := h.userService.GetByID(userUUID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"email_verified": user.EmailVerified,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	})
}
