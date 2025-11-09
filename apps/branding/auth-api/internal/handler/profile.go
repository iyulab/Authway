package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ProfileHandler struct {
	centralAPIURL  string
	internalAPIKey string
	logger         *zap.Logger
}

func NewProfileHandler(centralAPIURL string, internalAPIKey string, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		centralAPIURL:  centralAPIURL,
		internalAPIKey: internalAPIKey,
		logger:         logger,
	}
}

// GetProfile proxies GET /api/v1/profile/me to Central API
func (h *ProfileHandler) GetProfile(c *fiber.Ctx) error {
	h.logger.Info("Proxying get profile request to Central API")

	// Get Authorization header from request
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Forward to Central API
	url := fmt.Sprintf("%s/api/v1/profile/me", h.centralAPIURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.logger.Error("Failed to create request", zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	req.Header.Set("Authorization", authHeader)
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

	h.logger.Info("Successfully proxied get profile request",
		zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code
	return c.Status(resp.StatusCode).Send(respBody)
}
