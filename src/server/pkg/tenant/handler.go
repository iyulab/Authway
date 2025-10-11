package tenant

import (
	"errors"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for tenant operations
type Handler struct {
	service  *Service
	validate *validator.Validate
}

// NewHandler creates a new tenant handler
func NewHandler(service *Service, validate *validator.Validate) *Handler {
	return &Handler{
		service:  service,
		validate: validate,
	}
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

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"details": err.Error(),
		})
	}

	tenant, err := h.service.CreateTenant(req)
	if err != nil {
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrDuplicateSlug) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "A tenant with this slug already exists",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create tenant",
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

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"details": err.Error(),
		})
	}

	tenant, err := h.service.UpdateTenant(id, req)
	if err != nil {
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Tenant not found",
			})
		}
		if errors.Is(err, ErrCannotDeactivateDefault) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Cannot deactivate the default tenant",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update tenant",
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
		// Handle specific errors with appropriate status codes
		if errors.Is(err, ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Tenant not found",
			})
		}
		if errors.Is(err, ErrCannotDeleteDefault) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Cannot delete the default tenant",
			})
		}
		if errors.Is(err, ErrHasUsers) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Cannot delete tenant with existing users",
			})
		}
		if errors.Is(err, ErrHasClients) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Cannot delete tenant with existing clients",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete tenant",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
