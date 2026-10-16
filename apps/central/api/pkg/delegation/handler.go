package delegation

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler serves delegations to an administrator (a tenant's, by the admin
// key or the console session) and to the user who granted them (with their
// own access token).
type Handler struct {
	service  Service
	consents ConsentRevoker
	audit    audit.Service
	logger   *zap.Logger
}

// ConsentRevoker withdraws what a user granted a client at the
// authorization server: the remembered consent (and with it the client's
// refresh tokens for the user).
type ConsentRevoker interface {
	RevokeClientConsent(subject, clientID string) error
}

// NewHandler returns a Handler. auditService may be nil.
func NewHandler(service Service, consents ConsentRevoker, auditService audit.Service, logger *zap.Logger) *Handler {
	return &Handler{service: service, consents: consents, audit: auditService, logger: logger}
}

// List handles GET /api/v1/delegations: the tenant's delegations in force,
// narrowed by user_id and client_id (the OAuth client_id).
func (h *Handler) List(c *fiber.Ctx) error {
	tenantID, err := tenantscope.FromRequest(c)
	if err != nil {
		return err
	}
	var f Filter
	if v := c.Query("user_id"); v != "" {
		if f.UserID, err = uuid.Parse(v); err != nil {
			return apierror.RefuseWith(c, fiber.StatusBadRequest, "invalid_request", "user_id is not a valid id", fiber.Map{"field": "user_id"})
		}
	}
	f.ClientID = c.Query("client_id")
	views, err := h.service.List(c.Context(), tenantID, f)
	if err != nil {
		h.logger.Error("Failed to list delegations", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to list the delegations")
	}
	return c.JSON(fiber.Map{"delegations": views})
}

// Revoke handles DELETE /api/v1/delegations/:id.
func (h *Handler) Revoke(c *fiber.Ctx) error {
	v, err := h.find(c)
	if err != nil {
		return err
	}
	if err := tenantscope.Admit(c, v.TenantID, "Delegation not found"); err != nil {
		return err
	}
	return h.revoke(c, v, "administrator")
}

// ListMine handles GET /api/v1/users/delegations: the delegations in force
// the signed-in user granted.
func (h *Handler) ListMine(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to see your account.")
	}
	views, err := h.service.ListForUser(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to list a user's delegations", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to list the delegations")
	}
	return c.JSON(fiber.Map{"delegations": views})
}

// RevokeMine handles DELETE /api/v1/users/delegations/:id. Another user's
// delegation is not found.
func (h *Handler) RevokeMine(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to change your account.")
	}
	v, err := h.find(c)
	if err != nil {
		return err
	}
	if v.UserID != userID {
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Delegation not found")
	}
	return h.revoke(c, v, "user")
}

func (h *Handler) find(c *fiber.Ctx) (*View, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return nil, apierror.Reject(fiber.StatusNotFound, "not_found", "Delegation not found")
	}
	v, err := h.service.Find(c.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, apierror.Reject(fiber.StatusNotFound, "not_found", "Delegation not found")
	case err != nil:
		h.logger.Error("Failed to read a delegation", zap.Error(err))
		return nil, apierror.Reject(fiber.StatusInternalServerError, "internal_server_error", "Failed to read the delegation")
	}
	return v, nil
}

// revoke ends a delegation. The user's remembered consent to the client goes
// first: while it stands, the client's next sign-in for the user would be
// granted the delegation scope again without asking. If it cannot be
// withdrawn, the delegation stays, and the caller can try again.
func (h *Handler) revoke(c *fiber.Ctx, v *View, by string) error {
	if h.consents != nil {
		if err := h.consents.RevokeClientConsent(v.UserID.String(), v.ClientID); err != nil {
			h.logger.Error("Failed to withdraw the consent behind a delegation", zap.Error(err), zap.String("delegation_id", v.ID.String()))
			return apierror.Refuse(c, fiber.StatusBadGateway, "authorization_server_unavailable", "The delegation could not be revoked. Try again shortly.")
		}
	}
	err := h.service.Revoke(c.Context(), v.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Delegation not found")
	case err != nil:
		h.logger.Error("Failed to revoke a delegation", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to revoke the delegation")
	}
	if h.audit != nil {
		entry := audit.EntryFromFiber(c, v.TenantID, audit.ActionDelegationRevoked, "delegation", v.ID.String())
		entry.Details["user_id"] = v.UserID.String()
		entry.Details["client_id"] = v.ClientID
		entry.Details["revoked_by"] = by
		h.audit.LogAsync(entry)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
