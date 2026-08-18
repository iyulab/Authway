package invitation

import (
	"net/url"

	"authway/apps/central/api/pkg/apierror"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles invitation HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new invitation handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// CreateInvitation creates a new organization invitation
// POST /api/v1/invitations
func (h *Handler) CreateInvitation(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized - tenant_id required"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	// A signed-in user is attributed as the inviter. The Admin Console
	// authenticates with the admin API key and has no user behind it, so it
	// invites as the system actor — inviterID stays nil, which the schema now
	// expresses as a NULL inviter_id. (It previously pointed at a hard-coded
	// UUID that no users row ever had, which failed every admin-key invite.)
	var inviterID *uuid.UUID
	isAdminConsole := c.Locals("is_admin_console")
	userIDStr := c.Locals("user_id")

	if userIDStr != nil {
		parsed, err := uuid.Parse(userIDStr.(string))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
		}
		inviterID = &parsed
	} else if isAdminConsole != nil && isAdminConsole.(bool) {
		// system actor — nil inviter
	} else {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized - user_id required"})
	}

	var req CreateInvitationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}

	invitation, err := h.service.Create(tenantID, inviterID, &req)
	if err != nil {
		h.logger.Warn("Failed to create invitation", zap.Error(err), zap.String("email", req.Email))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to create invitation")})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"invitation": invitation,
		"message":    "invitation sent successfully",
	})
}

// ListInvitations lists all invitations for the tenant
// GET /api/v1/invitations
func (h *Handler) ListInvitations(c *fiber.Ctx) error {
	tenantIDStr := c.Locals("tenant_id")
	if tenantIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(tenantIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant ID"})
	}

	invitations, err := h.service.ListByTenant(tenantID)
	if err != nil {
		h.logger.Error("Failed to list invitations", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list invitations"})
	}

	return c.JSON(fiber.Map{
		"invitations": invitations,
		"count":       len(invitations),
	})
}

// GetInvitation gets an invitation by ID
// GET /api/v1/invitations/:id
func (h *Handler) GetInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid invitation ID"})
	}

	invitation, err := h.service.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invitation not found"})
	}

	return c.JSON(fiber.Map{"invitation": invitation})
}

// GetInvitationByToken gets invitation details by token (public endpoint)
// GET /api/v1/invitations/token/:token
func (h *Handler) GetInvitationByToken(c *fiber.Ctx) error {
	// Fiber hands back path params exactly as they appear in the URL — it does
	// not percent-decode them. Invitation tokens are base64 and end in "=",
	// which any correct client encodes as %3D, so the raw param never matched
	// and every invitation opened from an email reported "not found". Decoding
	// here (rather than flipping Fiber's global UnescapePath) keeps the change
	// to the one route that carries an opaque token in its path.
	token := c.Params("token")
	if decoded, err := url.PathUnescape(token); err == nil {
		token = decoded
	}
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	invitation, err := h.service.GetByToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invitation not found or expired"})
	}

	if !invitation.CanBeAccepted() {
		status := "invalid"
		if invitation.IsExpired() {
			status = "expired"
		} else {
			status = string(invitation.Status)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":  "invitation cannot be accepted",
			"status": status,
		})
	}

	return c.JSON(fiber.Map{
		"invitation": InvitationResponse{
			ID:          invitation.ID.String(),
			TenantName:  invitation.TenantName,
			InviterName: invitation.InviterName,
			Email:       invitation.Email,
			Role:        invitation.Role,
			Message:     invitation.Message,
			ExpiresAt:   invitation.ExpiresAt,
		},
	})
}

// AcceptInvitation accepts an invitation
// POST /api/v1/invitations/accept
func (h *Handler) AcceptInvitation(c *fiber.Ctx) error {
	var req AcceptInvitationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	// Check if user is already logged in
	var userID *uuid.UUID
	if userIDStr := c.Locals("user_id"); userIDStr != nil {
		if id, err := uuid.Parse(userIDStr.(string)); err == nil {
			userID = &id
		}
	}

	user, err := h.service.Accept(req.Token, userID, req.Name, req.Password)
	if err != nil {
		h.logger.Warn("Failed to accept invitation", zap.Error(err), zap.String("token_length", string(rune(len(req.Token)))))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to accept invitation")})
	}

	h.logger.Info("Invitation accepted", zap.String("user_id", user.ID.String()), zap.String("email", user.Email))
	return c.JSON(fiber.Map{
		"message": "invitation accepted successfully",
		"user": fiber.Map{
			"id":    user.ID.String(),
			"email": user.Email,
		},
	})
}

// DeclineInvitation declines an invitation
// POST /api/v1/invitations/decline
func (h *Handler) DeclineInvitation(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&body); err == nil {
			token = body.Token
		}
	}

	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required"})
	}

	if err := h.service.Decline(token); err != nil {
		h.logger.Warn("Failed to decline invitation", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to decline invitation")})
	}

	return c.JSON(fiber.Map{"message": "invitation declined"})
}

// RevokeInvitation revokes a pending invitation
// DELETE /api/v1/invitations/:id
func (h *Handler) RevokeInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid invitation ID"})
	}

	if err := h.service.Revoke(id); err != nil {
		h.logger.Warn("Failed to revoke invitation", zap.Error(err), zap.String("invitation_id", idStr))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to revoke invitation")})
	}

	return c.JSON(fiber.Map{"message": "invitation revoked"})
}

// ResendInvitation resends an invitation email
// POST /api/v1/invitations/:id/resend
func (h *Handler) ResendInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid invitation ID"})
	}

	if err := h.service.Resend(id); err != nil {
		h.logger.Warn("Failed to resend invitation", zap.Error(err), zap.String("invitation_id", idStr))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to resend invitation")})
	}

	return c.JSON(fiber.Map{"message": "invitation resent"})
}

// GetPendingInvitations gets pending invitations for an email (public)
// GET /api/v1/invitations/pending
func (h *Handler) GetPendingInvitations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}

	invitations, err := h.service.ListPendingByEmail(email)
	if err != nil {
		h.logger.Error("Failed to get pending invitations", zap.Error(err), zap.String("email", email))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get invitations"})
	}

	// Return minimal info for security
	responses := make([]InvitationResponse, 0, len(invitations))
	for _, inv := range invitations {
		responses = append(responses, InvitationResponse{
			ID:          inv.ID.String(),
			TenantName:  inv.TenantName,
			InviterName: inv.InviterName,
			Email:       inv.Email,
			Role:        inv.Role,
			ExpiresAt:   inv.ExpiresAt,
		})
	}

	return c.JSON(fiber.Map{
		"invitations": responses,
		"count":       len(responses),
	})
}

// RegisterRoutes registers invitation routes
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
	// Public endpoints (no auth required)
	public := app.Group("/invitations")
	public.Get("/token/:token", h.GetInvitationByToken)
	public.Post("/accept", h.AcceptInvitation)
	public.Post("/decline", h.DeclineInvitation)
	public.Get("/pending", h.GetPendingInvitations)

	// Admin Console protected endpoints - use adminMiddleware only (validates admin session + tenant context)
	protected := app.Group("/invitations", adminMiddleware)
	protected.Post("/", h.CreateInvitation)
	protected.Get("/", h.ListInvitations)
	protected.Get("/:id", h.GetInvitation)
	protected.Delete("/:id", h.RevokeInvitation)
	protected.Post("/:id/resend", h.ResendInvitation)
}
