package tenant

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for tenant operations
type Handler struct {
	service *Service
}

// NewHandler creates a new tenant handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers tenant routes
// All routes require Admin API Key authentication
func (h *Handler) RegisterRoutes(app *fiber.App, adminMiddleware fiber.Handler) {
	api := app.Group("/api/v1/tenants")

	// Apply admin middleware to all routes
	api.Use(adminMiddleware)

	api.Post("/", h.CreateTenant)       // POST /api/v1/tenants
	api.Get("/", h.ListTenants)         // GET /api/v1/tenants
	api.Get("/:id", h.GetTenant)        // GET /api/v1/tenants/:id
	api.Put("/:id", h.UpdateTenant)     // PUT /api/v1/tenants/:id
	api.Delete("/:id", h.DeleteTenant)  // DELETE /api/v1/tenants/:id
}

// CreateTenant creates a new tenant
// POST /api/v1/tenants
func (h *Handler) CreateTenant(c *fiber.Ctx) error {
	var req CreateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// TODO: Add validation
	// if err := validate.Struct(req); err != nil {
	// 	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
	// 		"error": "Validation failed",
	// 		"details": err.Error(),
	// 	})
	// }

	tenant, err := h.service.CreateTenant(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(tenant.ToPublic())
}

// ListTenants lists all active tenants
// GET /api/v1/tenants
func (h *Handler) ListTenants(c *fiber.Ctx) error {
	tenants, err := h.service.ListTenants()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Convert to public format
	publicTenants := make([]PublicTenant, len(tenants))
	for i, tenant := range tenants {
		publicTenants[i] = tenant.ToPublic()
	}

	return c.JSON(publicTenants)
}

// GetTenant retrieves a tenant by ID
// GET /api/v1/tenants/:id
func (h *Handler) GetTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid tenant ID format",
		})
	}

	tenant, err := h.service.GetTenantByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(tenant.ToPublic())
}

// UpdateTenant updates a tenant
// PUT /api/v1/tenants/:id
func (h *Handler) UpdateTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid tenant ID format",
		})
	}

	var req UpdateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	tenant, err := h.service.UpdateTenant(id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(tenant.ToPublic())
}

// DeleteTenant soft deletes a tenant
// DELETE /api/v1/tenants/:id
func (h *Handler) DeleteTenant(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid tenant ID format",
		})
	}

	if err := h.service.DeleteTenant(id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
