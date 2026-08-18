package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

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

// NOTE: The /register proxy (RegisterRequest + Register) and its
// getDefaultTenantID helper were removed — onboarding is invitation-only and
// public self-registration no longer exists (decision D-a/B). Users are created
// via the Central invitation accept flow or by an admin.

// Authenticate proxies POST /authenticate to Central API
func (h *AuthHandler) Authenticate(c *fiber.Ctx) error {
	return h.proxyToCentralAPI(c, "/authenticate", "authenticate")
}

// VerifyMFA proxies POST /auth/mfa/verify to Central API — the TOTP-code leg
// of a login that Authenticate parked pending a second factor.
func (h *AuthHandler) VerifyMFA(c *fiber.Ctx) error {
	return h.proxyToCentralAPI(c, "/mfa/verify", "mfa verify")
}

// VerifyMFARecovery proxies POST /auth/mfa/recovery to Central API — the
// recovery-code counterpart of VerifyMFA.
func (h *AuthHandler) VerifyMFARecovery(c *fiber.Ctx) error {
	return h.proxyToCentralAPI(c, "/mfa/recovery", "mfa recovery")
}

// proxyToCentralAPI forwards the request body as-is to path on Central API
// and relays its response (status, content type, body) unchanged. Shared by
// every unauthenticated login-flow endpoint this handler proxies.
func (h *AuthHandler) proxyToCentralAPI(c *fiber.Ctx, path, label string) error {
	h.logger.Info("Proxying request to Central API", zap.String("endpoint", label))

	url := fmt.Sprintf("%s%s", h.centralAPIURL, path)
	req, err := http.NewRequest("POST", url, bytes.NewReader(c.Body()))
	if err != nil {
		h.logger.Error("Failed to create request", zap.String("endpoint", label), zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	// Copy headers
	req.Header.Set("Content-Type", c.Get("Content-Type", "application/json"))
	req.Header.Set("X-Internal-API-Key", h.internalAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Failed to proxy request to Central API", zap.String("endpoint", label), zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to proxy request to Central API",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("Failed to read response body", zap.String("endpoint", label), zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read response from Central API",
		})
	}

	h.logger.Info("Successfully proxied request",
		zap.String("endpoint", label), zap.Int("status_code", resp.StatusCode))

	// Forward response with same status code and content type
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	return c.Status(resp.StatusCode).Send(respBody)
}
