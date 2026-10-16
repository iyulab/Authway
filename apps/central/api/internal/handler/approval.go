package handler

import (
	"errors"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/delegation"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ApprovalHandler serves a user the approvals clients asked them for, and
// their decision — on Authway's approval screen.
//
// Deciding is the user's own act at Authway: it takes a token issued to the
// deployment's account application and a recent sign-in. An application that
// holds tokens for the user cannot decide in their place — otherwise an
// approval would prove no more than the application's own say-so.
type ApprovalHandler struct {
	approvals *delegation.Approvals
	clients   client.Service
	audit     audit.Service
	logger    *zap.Logger
}

// NewApprovalHandler returns an ApprovalHandler. auditService may be nil.
func NewApprovalHandler(approvals *delegation.Approvals, clients client.Service, auditService audit.Service, logger *zap.Logger) *ApprovalHandler {
	return &ApprovalHandler{approvals: approvals, clients: clients, audit: auditService, logger: logger}
}

// Get handles GET /api/v1/users/approvals/:id — what the client asks, for
// the approval screen. Another user's request is not found.
func (h *ApprovalHandler) Get(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to see the request.")
	}
	v, err := h.find(c, userID)
	if err != nil {
		return err
	}
	return c.JSON(v)
}

// Approve handles POST /api/v1/users/approvals/:id/approve.
func (h *ApprovalHandler) Approve(c *fiber.Ctx) error { return h.decide(c, true) }

// Deny handles POST /api/v1/users/approvals/:id/deny.
func (h *ApprovalHandler) Deny(c *fiber.Ctx) error { return h.decide(c, false) }

func (h *ApprovalHandler) decide(c *fiber.Ctx, approve bool) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Sign in to answer the request.")
	}
	clientID, _ := c.Locals("client_id").(string)
	cl, err := h.clients.GetByClientID(clientID)
	if err != nil || !cl.System {
		return apierror.Refuse(c, fiber.StatusForbidden, "account_application_required",
			"Answer the request on the approval screen: a token another application holds for you cannot answer it.")
	}
	if !signedInRecently(c) {
		return refuseStaleSignIn(c, "Sign in again to answer the request.")
	}
	v, err := h.find(c, userID)
	if err != nil {
		return err
	}
	err = h.approvals.Decide(c.UserContext(), userID, v.ID, approve)
	switch {
	case errors.Is(err, delegation.ErrApprovalNotFound):
		return apierror.Refuse(c, fiber.StatusNotFound, "not_found", "Request not found.")
	case errors.Is(err, delegation.ErrApprovalSettled):
		return apierror.Refuse(c, fiber.StatusConflict, "approval_settled", "The request was answered already, or it expired.")
	case err != nil:
		h.logger.Error("Failed to record an approval decision", zap.Error(err))
		return apierror.Refuse(c, fiber.StatusInternalServerError, "internal_server_error", "Failed to record the answer.")
	}
	if h.audit != nil {
		action := audit.ActionApprovalDenied
		if approve {
			action = audit.ActionApprovalApproved
		}
		tenantID, _ := c.Locals("tenant_id").(uuid.UUID)
		entry := audit.EntryFromFiber(c, tenantID, action, "approval_request", v.ID.String())
		entry.ActorType = "user"
		entry.ActorID = &userID
		entry.Details["client_id"] = v.ClientID
		h.audit.LogAsync(entry)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ApprovalHandler) find(c *fiber.Ctx, userID uuid.UUID) (*delegation.ApprovalView, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return nil, apierror.Reject(fiber.StatusNotFound, "not_found", "Request not found.")
	}
	v, err := h.approvals.ForUser(c.UserContext(), userID, id)
	switch {
	case errors.Is(err, delegation.ErrApprovalNotFound):
		return nil, apierror.Reject(fiber.StatusNotFound, "not_found", "Request not found.")
	case err != nil:
		h.logger.Error("Failed to read an approval request", zap.Error(err))
		return nil, apierror.Reject(fiber.StatusInternalServerError, "internal_server_error", "Failed to read the request.")
	}
	return v, nil
}
