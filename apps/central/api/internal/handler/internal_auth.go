package handler

import (
	"net/http"

	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InternalAuthHandler struct {
	userService   user.Service
	invitations   InvitationGate
	clientService client.Service
	logger        *zap.Logger
	auditService  audit.Service
}

func NewInternalAuthHandler(
	userService user.Service,
	invitations InvitationGate,
	clientService client.Service,
	logger *zap.Logger,
	auditService audit.Service,
) *InternalAuthHandler {
	return &InternalAuthHandler{
		userService:   userService,
		invitations:   invitations,
		clientService: clientService,
		logger:        logger,
		auditService:  auditService,
	}
}

// logGoogleAuth emits an audit entry for the Auth Backend → central
// AuthenticateGoogleUser path. `created` discriminates first-login JIT from
// returning user refresh.
func (h *InternalAuthHandler) logGoogleAuth(c *fiber.Ctx, u *user.User, clientID string, created bool) {
	if h.auditService == nil || u == nil {
		return
	}
	action := audit.ActionUserLogin
	if created {
		action = audit.ActionUserCreated
	}
	entry := audit.EntryFromFiber(c, u.TenantID, action, "user", u.ID.String())
	entry.ActorID = &u.ID
	entry.ActorEmail = u.Email
	entry.ActorType = "user"
	entry.Details["provider"] = "google"
	entry.Details["method"] = "social"
	entry.Details["source"] = "internal_api"
	entry.Details["client_id"] = clientID
	if created {
		entry.Details["jit_provisioned"] = true
	}
	h.auditService.LogAsync(entry)
}

// AuthenticateGoogleUserRequest represents the request from Auth Backend
type AuthenticateGoogleUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	GoogleID string `json:"google_id"`
	Picture  string `json:"picture"`
	ClientID string `json:"client_id"`
}

// AuthenticateGoogleUserResponse represents the response to Auth Backend
type AuthenticateGoogleUserResponse struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
}

// AuthenticateGoogleUser handles internal Google authentication from Auth Backend
func (h *InternalAuthHandler) AuthenticateGoogleUser(c *fiber.Ctx) error {
	var req AuthenticateGoogleUserRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error("Failed to parse request", zap.Error(err))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Email == "" || req.GoogleID == "" || req.ClientID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "email, google_id, and client_id are required",
		})
	}

	h.logger.Info("Processing internal Google authentication",
		zap.String("email", req.Email),
		zap.String("client_id", req.ClientID))

	// Get client to determine tenant
	oauthClient, err := h.clientService.GetByClientID(req.ClientID)
	if err != nil {
		h.logger.Error("Failed to get client for tenant determination",
			zap.Error(err),
			zap.String("client_id", req.ClientID))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid client_id",
		})
	}

	clientTenantID := oauthClient.TenantID

	// Check if user already exists in this tenant
	existingUser, err := h.userService.GetByEmailAndTenant(clientTenantID, req.Email)
	if err == nil {
		// User exists, update Google-specific fields
		existingUser.GoogleID = &req.GoogleID
		existingUser.Picture = &req.Picture
		existingUser.EmailVerified = true // Google verified

		updateReq := &user.UpdateUserRequest{
			AvatarURL: req.Picture,
		}
		if _, err := h.userService.Update(existingUser.ID, updateReq); err != nil {
			h.logger.Error("Failed to update existing user",
				zap.Error(err),
				zap.String("user_id", existingUser.ID.String()))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update user",
			})
		}

		// Update last login
		if err := h.userService.UpdateLastLogin(existingUser.ID); err != nil {
			h.logger.Warn("Failed to update last login",
				zap.Error(err),
				zap.String("user_id", existingUser.ID.String()))
			// Continue despite error
		}

		h.logger.Info("Updated existing user with Google account",
			zap.String("user_id", existingUser.ID.String()),
			zap.String("email", existingUser.Email),
			zap.String("tenant_id", clientTenantID.String()))

		h.logGoogleAuth(c, existingUser, req.ClientID, false)

		return c.JSON(AuthenticateGoogleUserResponse{
			UserID:   existingUser.ID.String(),
			TenantID: clientTenantID.String(),
			Email:    existingUser.Email,
		})
	}

	// Onboarding is invitation-only: a first-time sign-in may create an account
	// only for an address that was invited into this tenant.
	if !h.mayProvision(clientTenantID, req.Email) {
		h.logger.Warn("Social sign-in denied for uninvited address", zap.String("email", req.Email))
		return c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error": "no account for this address; ask an administrator for an invitation",
		})
	}

	// User doesn't exist, create new user
	h.logger.Info("Creating new user from Google account",
		zap.String("email", req.Email),
		zap.String("tenant_id", clientTenantID.String()))

	createReq := &user.CreateUserRequest{
		Email:    req.Email,
		Password: "", // Social login users don't need a password
		Name:     req.Name,
	}

	newUser, err := h.userService.Create(clientTenantID, createReq)
	if err != nil {
		h.logger.Error("Failed to create new user from Google",
			zap.Error(err),
			zap.String("email", req.Email))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	// Update Google-specific fields
	newUser.GoogleID = &req.GoogleID
	newUser.Picture = &req.Picture
	newUser.EmailVerified = true

	updateReq := &user.UpdateUserRequest{
		AvatarURL: req.Picture,
	}
	if _, err := h.userService.Update(newUser.ID, updateReq); err != nil {
		h.logger.Warn("Failed to update new user with Google fields",
			zap.Error(err),
			zap.String("user_id", newUser.ID.String()))
		// Continue despite error
	}

	h.logger.Info("Created new user from Google account",
		zap.String("user_id", newUser.ID.String()),
		zap.String("email", newUser.Email),
		zap.String("tenant_id", clientTenantID.String()))

	h.logGoogleAuth(c, newUser, req.ClientID, true)

	return c.JSON(AuthenticateGoogleUserResponse{
		UserID:   newUser.ID.String(),
		TenantID: clientTenantID.String(),
		Email:    newUser.Email,
	})
}

// InvitationGate reports whether an email has been invited into a tenant.
// Satisfied by *invitation.Gate.
type InvitationGate interface {
	HasValidInvitation(tenantID uuid.UUID, email string) (bool, error)
}

// mayProvision fails closed on a missing gate or a lookup error: a wiring
// mistake must not silently reopen self-registration through social login.
func (h *InternalAuthHandler) mayProvision(tenantID uuid.UUID, email string) bool {
	if h.invitations == nil {
		h.logger.Error("Invitation gate not wired; denying social provisioning")
		return false
	}
	invited, err := h.invitations.HasValidInvitation(tenantID, email)
	if err != nil {
		h.logger.Error("Invitation check failed; denying provisioning", zap.Error(err))
		return false
	}
	return invited
}
