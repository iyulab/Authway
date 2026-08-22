package handler

import (
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/serviceclient"
)

type ServiceClientHandler struct {
	service   serviceclient.Service
	logger    *zap.Logger
	validator *validator.Validate
	audit     audit.Service
}

func NewServiceClientHandler(service serviceclient.Service, logger *zap.Logger, auditService audit.Service) *ServiceClientHandler {
	return &ServiceClientHandler{service: service, logger: logger, validator: validator.New(), audit: auditService}
}

// Create provisions a new service_client credential for the tenant in the
// :id URL parameter. Admin-only: this endpoint mints new machine-to-machine
// credentials, so it stays behind the same admin authentication as tenant
// management, never the scoped authentication those credentials themselves
// use.
func (h *ServiceClientHandler) Create(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid tenant ID")
	}

	var req serviceclient.CreateServiceClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	sc, creds, err := h.service.Create(tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to create service client", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return fiber.NewError(fiber.StatusBadRequest, apierror.Message(err, "failed to create service client"))
	}

	if h.audit != nil {
		entry := audit.EntryFromFiber(c, tenantID, audit.ActionServiceClientCreated, "service_client", sc.ID.String())
		entry.Details["hydra_client_id"] = creds.ClientID
		entry.Details["name"] = sc.Name
		entry.Details["scopes"] = []string(sc.GrantedScopes)
		h.audit.LogAsync(entry)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":        "Service client created successfully",
		"service_client": sc,
		"credentials":    creds,
	})
}

// List returns the tenant's service_client credentials, paginated. Never
// includes a secret — ServiceClient carries none; Create is the only place
// the raw secret is ever exposed, exactly once.
// Admin-only, for the same reason as Create.
func (h *ServiceClientHandler) List(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid tenant ID")
	}

	limit, err := strconv.Atoi(c.Query("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	scs, total, err := h.service.ListByTenant(tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list service clients", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve service clients")
	}

	return c.JSON(fiber.Map{
		"service_clients": scs,
		"total":           total,
		"limit":           limit,
		"offset":          offset,
	})
}

// Revoke revokes the service_client identified by :service_client_id, scoped
// to the tenant in :id — a service_client belonging to a different tenant is
// reported not-found rather than revoked.
// Admin-only, for the same reason as Create.
func (h *ServiceClientHandler) Revoke(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid tenant ID")
	}

	id, err := uuid.Parse(c.Params("service_client_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid service client ID")
	}

	if err := h.service.Revoke(tenantID, id); err != nil {
		h.logger.Error("Failed to revoke service client", zap.Error(err), zap.String("id", id.String()), zap.String("tenant_id", tenantID.String()))
		return fiber.NewError(fiber.StatusNotFound, apierror.Message(err, "service client not found"))
	}

	if h.audit != nil {
		h.audit.LogAsync(audit.EntryFromFiber(c, tenantID, audit.ActionServiceClientRevoked, "service_client", id.String()))
	}

	return c.JSON(fiber.Map{"message": "Service client revoked successfully"})
}
