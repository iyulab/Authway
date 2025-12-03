package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AuthHandler handles authentication-related proxy requests to Central API
type AuthHandler struct {
	centralAPIURL  string
	internalAPIKey string
	logger         *zap.Logger
	defaultTenant  string
	tenantMu       sync.RWMutex
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(centralAPIURL string, internalAPIKey string, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		centralAPIURL:  centralAPIURL,
		internalAPIKey: internalAPIKey,
		logger:         logger,
	}
}

// getDefaultTenantID fetches the default tenant ID from Central API (cached)
func (h *AuthHandler) getDefaultTenantID() (string, error) {
	h.tenantMu.RLock()
	if h.defaultTenant != "" {
		defer h.tenantMu.RUnlock()
		return h.defaultTenant, nil
	}
	h.tenantMu.RUnlock()

	// Fetch from Central API
	url := fmt.Sprintf("%s/api/v1/tenants", h.centralAPIURL)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch tenants: %w", err)
	}
	defer resp.Body.Close()

	var tenants []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tenants); err != nil {
		return "", fmt.Errorf("failed to decode tenants: %w", err)
	}

	if len(tenants) == 0 {
		return "", fmt.Errorf("no tenants found")
	}

	h.tenantMu.Lock()
	h.defaultTenant = tenants[0].ID
	h.tenantMu.Unlock()

	h.logger.Info("Cached default tenant ID", zap.String("tenant_id", h.defaultTenant))
	return h.defaultTenant, nil
}

// RegisterRequest represents the incoming register request
type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
	TenantID  string `json:"tenant_id"`
}

// Register proxies POST /register to Central API with automatic tenant injection
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	h.logger.Info("Proxying register request to Central API")

	// Parse request body
	var req RegisterRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		h.logger.Error("Failed to parse request body", zap.Error(err))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// If tenant_id not provided, use default tenant
	if req.TenantID == "" {
		tenantID, err := h.getDefaultTenantID()
		if err != nil {
			h.logger.Error("Failed to get default tenant", zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to determine tenant",
			})
		}
		req.TenantID = tenantID
	}

	// Build name from first_name and last_name if name not provided
	if req.Name == "" && (req.FirstName != "" || req.LastName != "") {
		req.Name = fmt.Sprintf("%s %s", req.FirstName, req.LastName)
	}

	// Marshal modified request
	modifiedBody, err := json.Marshal(req)
	if err != nil {
		h.logger.Error("Failed to marshal request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process request",
		})
	}

	// Forward to Central API
	url := fmt.Sprintf("%s/register", h.centralAPIURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(modifiedBody))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	// Copy headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		h.logger.Error("Failed to proxy request to Central API", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to proxy request to Central API",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("Failed to read response body", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read response from Central API",
		})
	}

	h.logger.Info("Successfully proxied register request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code and content type
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	return c.Status(resp.StatusCode).Send(respBody)
}

// Authenticate proxies POST /authenticate to Central API
func (h *AuthHandler) Authenticate(c *fiber.Ctx) error {
	h.logger.Info("Proxying authenticate request to Central API")

	// Forward to Central API
	url := fmt.Sprintf("%s/authenticate", h.centralAPIURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(c.Body()))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	// Copy headers
	req.Header.Set("Content-Type", c.Get("Content-Type", "application/json"))
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Failed to proxy request to Central API", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to proxy request to Central API",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("Failed to read response body", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read response from Central API",
		})
	}

	h.logger.Info("Successfully proxied authenticate request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code and content type
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	return c.Status(resp.StatusCode).Send(respBody)
}
