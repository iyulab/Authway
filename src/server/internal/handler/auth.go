package handler

import (
	"time"

	"authway/src/server/internal/hydra"
	"authway/src/server/pkg/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userService user.Service
	hydraClient *hydra.Client
}

func NewAuthHandler(userService user.Service, hydraClient *hydra.Client) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		hydraClient: hydraClient,
	}
}

// Login flow handler
func (h *AuthHandler) LoginPage(c *fiber.Ctx) error {
	challenge := c.Query("login_challenge")
	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "login_challenge parameter is required",
		})
	}

	// Get login request from Hydra
	loginReq, err := h.hydraClient.GetLoginRequest(challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get login request",
		})
	}

	// If user is already authenticated and session is still valid, skip login
	if loginReq.Skip {
		acceptBody := &hydra.AcceptLoginRequest{
			Subject:     loginReq.Subject,
			Remember:    true,
			RememberFor: 3600,
		}

		resp, err := h.hydraClient.AcceptLoginRequest(challenge, acceptBody)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to accept login request",
			})
		}

		return c.Redirect(resp.RedirectTo)
	}

	// Render login form with challenge
	return c.JSON(fiber.Map{
		"challenge":       challenge,
		"client_name":     loginReq.Client.ClientName,
		"requested_scope": loginReq.RequestedScope,
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
			"email": user.Email,
			"name":  user.Name,
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

// Consent flow handler
func (h *AuthHandler) ConsentPage(c *fiber.Ctx) error {
	challenge := c.Query("consent_challenge")
	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
		})
	}

	// Get consent request from Hydra
	consentReq, err := h.hydraClient.GetConsentRequest(challenge)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get consent request",
		})
	}

	// Get user information
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
		GrantScope:    req.GrantScope,
		GrantAudience: consentReq.RequestedAudience,
		Remember:      req.Remember,
		RememberFor:   req.RememberFor,
		HandledAt:     time.Now(),
		Session: map[string]interface{}{
			"access_token": map[string]interface{}{
				"email": user.Email,
				"name":  user.Name,
			},
			"id_token": map[string]interface{}{
				"email":          user.Email,
				"name":           user.Name,
				"email_verified": user.EmailVerified,
			},
		},
	}

	resp, err := h.hydraClient.AcceptConsentRequest(req.Challenge, acceptBody)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to accept consent request",
		})
	}

	return c.JSON(fiber.Map{
		"redirect_to": resp.RedirectTo,
	})
}

func (h *AuthHandler) RejectConsent(c *fiber.Ctx) error {
	challenge := c.Query("consent_challenge")
	if challenge == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "consent_challenge parameter is required",
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

	// Create user request
	createReq := &user.CreateUserRequest{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
	}

	createdUser, err := h.userService.Create(createReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":    createdUser.ID,
		"email": createdUser.Email,
		"name":  createdUser.Name,
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
