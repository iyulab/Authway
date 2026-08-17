package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ClaimsHandler struct {
	centralAPIURL  string
	internalAPIKey string
	logger         *zap.Logger
}

func NewClaimsHandler(centralAPIURL string, internalAPIKey string, logger *zap.Logger) *ClaimsHandler {
	return &ClaimsHandler{
		centralAPIURL:  centralAPIURL,
		internalAPIKey: internalAPIKey,
		logger:         logger,
	}
}

// GetClaims proxies GET /api/v1/claims to Central API
func (h *ClaimsHandler) GetClaims(c *fiber.Ctx) error {
	h.logger.Info("Proxying get claims request to Central API")

	// Get Authorization header from request
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Forward to Central API
	url := fmt.Sprintf("%s/api/v1/claims", h.centralAPIURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
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

	h.logger.Info("Successfully proxied get claims request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}

// UpdateClaims proxies PATCH /api/v1/claims to Central API
func (h *ClaimsHandler) UpdateClaims(c *fiber.Ctx) error {
	h.logger.Info("Proxying update claims request to Central API")

	// Get Authorization header from request
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Read request body
	bodyBytes := c.Body()

	// Forward to Central API
	url := fmt.Sprintf("%s/api/v1/claims", h.centralAPIURL)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
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

	h.logger.Info("Successfully proxied update claims request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}

// GetUserClaims proxies GET /api/v1/claims/user to Central API
func (h *ClaimsHandler) GetUserClaims(c *fiber.Ctx) error {
	h.logger.Info("Proxying get user claims request to Central API")

	// Get Authorization header from request
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Forward to Central API
	url := fmt.Sprintf("%s/api/v1/claims/user", h.centralAPIURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
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

	h.logger.Info("Successfully proxied get user claims request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}

// UpdateUserClaims proxies PATCH /api/v1/claims/user to Central API
func (h *ClaimsHandler) UpdateUserClaims(c *fiber.Ctx) error {
	h.logger.Info("Proxying update user claims request to Central API")

	// Get Authorization header from request
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Read request body
	bodyBytes := c.Body()

	// Forward to Central API
	url := fmt.Sprintf("%s/api/v1/claims/user", h.centralAPIURL)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
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

	h.logger.Info("Successfully proxied update user claims request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}
