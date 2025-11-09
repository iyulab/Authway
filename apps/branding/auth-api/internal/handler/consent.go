package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ConsentHandler struct {
	centralAPIURL  string
	internalAPIKey string
	logger         *zap.Logger
}

func NewConsentHandler(centralAPIURL string, internalAPIKey string, logger *zap.Logger) *ConsentHandler {
	return &ConsentHandler{
		centralAPIURL:  centralAPIURL,
		internalAPIKey: internalAPIKey,
		logger:         logger,
	}
}

// GetConsentInfo proxies consent request to Central API
func (h *ConsentHandler) GetConsentInfo(c *fiber.Ctx) error {
	h.logger.Info("Proxying consent info request to Central API")

	// Read request body using Fiber's Body() method
	bodyBytes := c.Body()

	// Forward to Central API
	url := fmt.Sprintf("%s/consent", h.centralAPIURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Content-Type", "application/json")
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

	h.logger.Info("Successfully proxied consent info request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}

// AcceptConsent proxies consent accept request to Central API
func (h *ConsentHandler) AcceptConsent(c *fiber.Ctx) error {
	h.logger.Info("Proxying consent accept request to Central API")

	// Read request body using Fiber's Body() method
	bodyBytes := c.Body()

	// Log request for debugging
	var reqData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqData); err == nil {
		h.logger.Info("Consent accept request",
			zap.Any("request", reqData))
	}

	// Forward to Central API
	url := fmt.Sprintf("%s/consent/accept", h.centralAPIURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Content-Type", "application/json")
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

	h.logger.Info("Successfully proxied consent accept request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}

// RejectConsent proxies consent reject request to Central API
func (h *ConsentHandler) RejectConsent(c *fiber.Ctx) error {
	h.logger.Info("Proxying consent reject request to Central API")

	// Read request body using Fiber's Body() method
	bodyBytes := c.Body()

	// Forward to Central API
	url := fmt.Sprintf("%s/consent/reject", h.centralAPIURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Content-Type", "application/json")
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

	h.logger.Info("Successfully proxied consent reject request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}
