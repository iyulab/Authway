package invitation

import (
	"authway/apps/central/api/pkg/tenantscope"
	"errors"
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
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
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
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid user ID")
		}
		inviterID = &parsed
	} else if isAdminConsole != nil && isAdminConsole.(bool) {
		// system actor — nil inviter
	} else {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "unauthorized - user_id required")
	}

	var req CreateInvitationRequest
	if err := c.BodyParser(&req); err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid request body")
	}

	if req.Email == "" {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "email is required")
	}

	invitation, err := h.service.Create(tenantID, inviterID, &req)
	if err != nil {
		h.logger.Warn("Failed to create invitation", zap.Error(err), zap.String("email", req.Email))
		switch {
		case errors.Is(err, ErrAlreadyMember):
			return apierror.Refuse(c, fiber.StatusConflict, "user_already_member", err.Error())
		case errors.Is(err, ErrAlreadyInvited):
			return apierror.Refuse(c, fiber.StatusConflict, "invitation_already_pending", err.Error())
		case errors.Is(err, ErrTenantNotFound):
			return apierror.Refuse(c, fiber.StatusNotFound, "not_found", err.Error())
		}
		var pub *apierror.Public
		if errors.As(err, &pub) {
			return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", pub.Error())
		}
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "failed to create invitation")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"invitation": invitation,
		"message":    "invitation sent successfully",
	})
}

// ListInvitations lists all invitations for the tenant
// GET /api/v1/invitations
func (h *Handler) ListInvitations(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}

	status := InvitationStatus(c.Query("status"))
	switch status {
	case "", StatusPending, StatusAccepted, StatusDeclined, StatusExpired, StatusRevoked:
	default:
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "status must be one of pending, accepted, declined, expired, revoked")
	}
	limit := c.QueryInt("limit", 20)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}

	invitations, total, err := h.service.ListByTenant(tenantID, status, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list invitations", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "failed to list invitations")
	}

	return c.JSON(fiber.Map{
		"invitations": invitations,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})
}

// GetInvitation gets an invitation by ID
// GET /api/v1/invitations/:id
func (h *Handler) GetInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid invitation ID")
	}

	invitation, err := h.service.GetByID(id)
	if err != nil {
		return h.refuseByID(c, err, "failed to get invitation")
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required", "code": "invalid_request"})
	}

	invitation, err := h.service.GetByToken(token)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invitation not found or expired", "code": "invalid_token"})
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
			"code":   "invitation_not_acceptable",
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body", "code": "invalid_request"})
	}

	if req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "token is required", "code": "invalid_request"})
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
		h.logger.Warn("Failed to accept invitation", zap.Error(err), zap.Int("token_length", len(req.Token)))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": apierror.Message(err, "failed to accept invitation"), "code": "invitation_not_accepted"})
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
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "token is required")
	}

	if err := h.service.Decline(token); err != nil {
		h.logger.Warn("Failed to decline invitation", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusBadRequest, "invitation_not_acceptable", apierror.Message(err, "failed to decline invitation"))
	}

	return c.JSON(fiber.Map{"message": "invitation declined"})
}

// RevokeInvitation revokes a pending invitation
// DELETE /api/v1/invitations/:id
func (h *Handler) RevokeInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid invitation ID")
	}

	if err := h.service.Revoke(id); err != nil {
		h.logger.Warn("Failed to revoke invitation", zap.Error(err), zap.String("invitation_id", idStr))
		return h.refuseByID(c, err, "failed to revoke invitation")
	}

	return c.JSON(fiber.Map{"message": "invitation revoked"})
}

// ResendInvitation resends an invitation email
// POST /api/v1/invitations/:id/resend
func (h *Handler) ResendInvitation(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return apierror.Refuse(c, fiber.StatusBadRequest, "invalid_request", "invalid invitation ID")
	}

	if err := h.service.Resend(id); err != nil {
		h.logger.Warn("Failed to resend invitation", zap.Error(err), zap.String("invitation_id", idStr))
		return h.refuseByID(c, err, "failed to resend invitation")
	}

	return c.JSON(fiber.Map{"message": "invitation resent"})
}

// refuseByID answers a failed lookup or state change of one invitation.
func (h *Handler) refuseByID(c *fiber.Ctx, err error, fallback string) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrNotPending):
		return apierror.Refuse(c, fiber.StatusConflict, "invitation_not_pending", err.Error())
	}
	h.logger.Error(fallback, zap.Error(err))
	return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", fallback)
}

// RegisterRoutes registers invitation routes
// Admin Console uses adminMiddleware which validates admin session and extracts tenant_id
func (h *Handler) RegisterRoutes(app fiber.Router, authMiddleware fiber.Handler, adminMiddleware fiber.Handler) {
	// Public endpoints (no auth required)
	public := app.Group("/invitations")
	public.Get("/token/:token", h.GetInvitationByToken)
	public.Post("/accept", h.AcceptInvitation)
	public.Post("/decline", h.DeclineInvitation)

	// Admin Console protected endpoints - use adminMiddleware only (validates admin session + tenant context)
	protected := app.Group("/invitations", adminMiddleware)
	protected.Post("/", h.CreateInvitation)
	protected.Get("/", h.ListInvitations)
	protected.Get("/:id", h.GetInvitation)
	protected.Delete("/:id", h.RevokeInvitation)
	protected.Post("/:id/resend", h.ResendInvitation)
}
