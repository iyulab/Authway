package handler

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

type HealthHandler struct {
	version string
}

func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{version: version}
}

// Health returns a simple health check response
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "OK",
		"service": "auth-backend",
		"version": h.version,
	})
}

// Config returns Authway configuration for clients
// This allows consumer apps to only specify auth-api URL
func (h *HealthHandler) Config(c *fiber.Ctx) error {
	// Get config from environment
	hydraPublicURL := getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444")
	authBackendURL := getEnv("AUTH_BACKEND_URL", "http://localhost:8081")

	return c.JSON(fiber.Map{
		"oauth_url": hydraPublicURL, // OAuth 2.0 server (Hydra)
		"api_url":   authBackendURL, // Auth Backend API
		"issuer":    hydraPublicURL, // Token issuer
		"version":   h.version,
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
