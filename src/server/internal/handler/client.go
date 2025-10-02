package handler

import (
	"strconv"

	"authway/src/server/internal/service"
	"authway/src/server/pkg/client"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ClientHandler struct {
	services  *service.Services
	logger    *zap.Logger
	validator *validator.Validate
}

func NewClientHandler(services *service.Services, logger *zap.Logger) *ClientHandler {
	return &ClientHandler{
		services:  services,
		logger:    logger,
		validator: validator.New(),
	}
}

// List handles listing OAuth clients with pagination
func (h *ClientHandler) List(c *fiber.Ctx) error {
	// Parse query parameters
	limitStr := c.Query("limit", "20")
	offsetStr := c.Query("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	clients, total, err := h.services.ClientService.List(limit, offset)
	if err != nil {
		h.logger.Error("Failed to list clients", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to retrieve clients")
	}

	// Convert to public client objects
	publicClients := make([]client.PublicClient, len(clients))
	for i, cl := range clients {
		publicClients[i] = cl.ToPublic()
	}

	return c.JSON(fiber.Map{
		"clients": publicClients,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// Create handles creating a new OAuth client
func (h *ClientHandler) Create(c *fiber.Ctx) error {
	var req client.CreateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	newClient, credentials, err := h.services.ClientService.Create(&req)
	if err != nil {
		h.logger.Error("Failed to create client", zap.Error(err), zap.String("name", req.Name))
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	h.logger.Info("Client created successfully", zap.String("client_id", newClient.ClientID))

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":     "Client created successfully",
		"client":      newClient.ToPublic(),
		"credentials": credentials,
	})
}

// Get handles getting a specific OAuth client by ID
func (h *ClientHandler) Get(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	foundClient, err := h.services.ClientService.GetByID(id)
	if err != nil {
		h.logger.Error("Failed to get client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	return c.JSON(fiber.Map{
		"client": foundClient.ToPublic(),
	})
}

// Update handles updating OAuth client information
func (h *ClientHandler) Update(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	var req client.UpdateClientRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	updatedClient, err := h.services.ClientService.Update(id, &req)
	if err != nil {
		h.logger.Error("Failed to update client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("Client updated successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "Client updated successfully",
		"client":  updatedClient.ToPublic(),
	})
}

// Delete handles deleting an OAuth client
func (h *ClientHandler) Delete(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	if err := h.services.ClientService.Delete(id); err != nil {
		h.logger.Error("Failed to delete client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("Client deleted successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "Client deleted successfully",
	})
}

// RegenerateSecret handles regenerating client secret
func (h *ClientHandler) RegenerateSecret(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	credentials, err := h.services.ClientService.RegenerateSecret(id)
	if err != nil {
		h.logger.Error("Failed to regenerate client secret", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("Client secret regenerated successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message":     "Client secret regenerated successfully",
		"credentials": credentials,
	})
}

// UpdateGoogleOAuth handles updating Google OAuth configuration for a client
func (h *ClientHandler) UpdateGoogleOAuth(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	type GoogleOAuthRequest struct {
		GoogleClientID     string `json:"google_client_id" validate:"required"`
		GoogleClientSecret string `json:"google_client_secret" validate:"required"`
		GoogleRedirectURI  string `json:"google_redirect_uri" validate:"required,url"`
	}

	var req GoogleOAuthRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.validator.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Validation failed: "+err.Error())
	}

	// Update client with Google OAuth settings
	updateReq := &client.UpdateClientRequest{
		GoogleOAuthEnabled: &[]bool{true}[0], // Pointer to true
		GoogleClientID:     &req.GoogleClientID,
		GoogleClientSecret: &req.GoogleClientSecret,
		GoogleRedirectURI:  &req.GoogleRedirectURI,
	}

	updatedClient, err := h.services.ClientService.Update(id, updateReq)
	if err != nil {
		h.logger.Error("Failed to update client Google OAuth", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("Client Google OAuth updated successfully",
		zap.String("id", idStr),
		zap.String("google_client_id", req.GoogleClientID))

	return c.JSON(fiber.Map{
		"message": "Google OAuth configuration updated successfully",
		"client":  updatedClient.ToPublic(),
	})
}

// DisableGoogleOAuth handles disabling Google OAuth for a client
func (h *ClientHandler) DisableGoogleOAuth(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	// Update client to disable Google OAuth
	updateReq := &client.UpdateClientRequest{
		GoogleOAuthEnabled: &[]bool{false}[0], // Pointer to false
		GoogleClientID:     nil,
		GoogleClientSecret: nil,
		GoogleRedirectURI:  nil,
	}

	updatedClient, err := h.services.ClientService.Update(id, updateReq)
	if err != nil {
		h.logger.Error("Failed to disable client Google OAuth", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	h.logger.Info("Client Google OAuth disabled successfully", zap.String("id", idStr))

	return c.JSON(fiber.Map{
		"message": "Google OAuth configuration disabled successfully",
		"client":  updatedClient.ToPublic(),
	})
}

// GetGoogleOAuthStatus handles getting Google OAuth status for a client
func (h *ClientHandler) GetGoogleOAuthStatus(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid client ID")
	}

	foundClient, err := h.services.ClientService.GetByID(id)
	if err != nil {
		h.logger.Error("Failed to get client", zap.Error(err), zap.String("id", idStr))
		return fiber.NewError(fiber.StatusNotFound, "Client not found")
	}

	status := "disabled"
	oauthType := "central"

	if foundClient.GoogleOAuthEnabled && foundClient.GoogleClientID != nil && foundClient.GoogleClientSecret != nil {
		status = "enabled"
		oauthType = "client_specific"
	}

	response := fiber.Map{
		"client_id":           foundClient.ClientID,
		"google_oauth_status": status,
		"oauth_type":          oauthType,
		"google_redirect_uri": foundClient.GoogleRedirectURI,
	}

	// Include Google Client ID (but not secret) if enabled
	if status == "enabled" {
		response["google_client_id"] = foundClient.GoogleClientID
	}

	return c.JSON(response)
}
