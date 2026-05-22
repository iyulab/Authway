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

type LogoutHandler struct {
	centralAPIURL string
	internalKey   string
	hydraAdminURL string
	logger        *zap.Logger
}

func NewLogoutHandler(centralAPIURL, internalKey, hydraAdminURL string, logger *zap.Logger) *LogoutHandler {
	return &LogoutHandler{
		centralAPIURL: centralAPIURL,
		internalKey:   internalKey,
		hydraAdminURL: hydraAdminURL,
		logger:        logger,
	}
}

// LogoutRequest represents Hydra's logout request
type LogoutRequest struct {
	Challenge        string `json:"challenge"`
	Subject          string `json:"subject"`
	SessionID        string `json:"sid"`
	RequestURL       string `json:"request_url"`
	RPInitiated      bool   `json:"rp_initiated"`
	RequestedAt      string `json:"requested_at"`
	Client           *LogoutClient `json:"client,omitempty"`
}

type LogoutClient struct {
	ClientID string `json:"client_id"`
}

// ClientConfig represents client configuration from Central API
type ClientConfig struct {
	ID                      string   `json:"id"`
	ClientID                string   `json:"client_id"`
	PostLogoutRedirectURIs  []string `json:"post_logout_redirect_uris"`
	LogoutRedirectPolicy    string   `json:"logout_redirect_policy"`
	DefaultLogoutURI        *string  `json:"default_logout_uri"`
	AllowWildcardLogout     bool     `json:"allow_wildcard_logout"`
	Website                 string   `json:"website"`
}

// HandleLogout processes logout requests with configurable policy validation
func (h *LogoutHandler) HandleLogout(c *fiber.Ctx) error {
	logoutChallenge := c.Query("logout_challenge")
	if logoutChallenge == "" {
		h.logger.Error("Logout challenge missing")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "logout_challenge parameter is required",
		})
	}

	h.logger.Info("Processing logout request", zap.String("challenge", logoutChallenge))

	// 1. Get logout request info from Hydra
	logoutReq, err := h.getLogoutRequest(logoutChallenge)
	if err != nil {
		h.logger.Error("Failed to get logout request", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": fmt.Sprintf("Failed to retrieve logout request: %v", err),
		})
	}

	// Extract client_id from logout request
	clientID := ""
	if logoutReq.Client != nil {
		clientID = logoutReq.Client.ClientID
	}

	if clientID == "" {
		h.logger.Warn("No client_id in logout request", zap.String("challenge", logoutChallenge))
		// Accept logout without redirect validation if no client
		return h.acceptLogout(c, logoutChallenge, "")
	}

	// 2. Get client configuration from Central API
	clientConfig, err := h.getClientConfig(clientID)
	if err != nil {
		h.logger.Error("Failed to get client config",
			zap.String("client_id", clientID),
			zap.Error(err))
		// If we can't get client config, accept logout without redirect
		return h.acceptLogout(c, logoutChallenge, "")
	}

	// 3. Parse post_logout_redirect_uri from request
	postLogoutRedirectURI := c.Query("post_logout_redirect_uri")

	h.logger.Info("Validating logout redirect",
		zap.String("policy", clientConfig.LogoutRedirectPolicy),
		zap.String("requested_uri", postLogoutRedirectURI),
		zap.Strings("whitelist", clientConfig.PostLogoutRedirectURIs))

	// 4. Validate based on logout_redirect_policy
	validatedURI, err := h.validateLogoutRedirect(clientConfig, postLogoutRedirectURI)
	if err != nil {
		h.logger.Error("Logout redirect validation failed",
			zap.String("client_id", clientID),
			zap.String("policy", clientConfig.LogoutRedirectPolicy),
			zap.Error(err))

		// Return error with fallback redirect info for graceful degradation
		// This allows the UI to redirect users back to their app even on error
		fallbackURI := h.determineFallbackURI(clientConfig, postLogoutRedirectURI)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": err.Error(),
			"fallback_redirect": fallbackURI,
			"client_id":         clientID,
		})
	}

	// 5. Accept logout with validated redirect URI
	return h.acceptLogout(c, logoutChallenge, validatedURI)
}

// determineFallbackURI determines the best fallback URI for error cases
func (h *LogoutHandler) determineFallbackURI(config *ClientConfig, requestedURI string) string {
	// Priority: default_logout_uri > first post_logout_redirect_uri > website > requested URI origin
	if config.DefaultLogoutURI != nil && *config.DefaultLogoutURI != "" {
		return *config.DefaultLogoutURI
	}
	if len(config.PostLogoutRedirectURIs) > 0 {
		return config.PostLogoutRedirectURIs[0]
	}
	if config.Website != "" {
		return config.Website
	}
	// Extract origin from requested URI as last resort
	if requestedURI != "" {
		return extractOrigin(requestedURI)
	}
	return ""
}

// extractOrigin extracts the origin (scheme + host) from a URL
func extractOrigin(uri string) string {
	// Simple extraction: find third slash or end
	slashCount := 0
	for i, ch := range uri {
		if ch == '/' {
			slashCount++
			if slashCount == 3 {
				return uri[:i]
			}
		}
	}
	return uri
}

// getLogoutRequest retrieves logout request info from Hydra
func (h *LogoutHandler) getLogoutRequest(challenge string) (*LogoutRequest, error) {
	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/logout?logout_challenge=%s",
		h.hydraAdminURL, challenge)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Hydra: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Hydra returned status %d: %s", resp.StatusCode, string(body))
	}

	var logoutReq LogoutRequest
	if err := json.NewDecoder(resp.Body).Decode(&logoutReq); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &logoutReq, nil
}

// getClientConfig retrieves client configuration from Central API
func (h *LogoutHandler) getClientConfig(clientID string) (*ClientConfig, error) {
	url := fmt.Sprintf("%s/api/v1/clients/by-client-id/%s", h.centralAPIURL, clientID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Internal-Key", h.internalKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Central API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Central API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Client ClientConfig `json:"client"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result.Client, nil
}

// validateLogoutRedirect validates post_logout_redirect_uri based on policy
func (h *LogoutHandler) validateLogoutRedirect(config *ClientConfig, requestedURI string) (string, error) {
	policy := config.LogoutRedirectPolicy
	if policy == "" {
		policy = "strict" // Default to strict
	}

	switch policy {
	case "disabled":
		// No validation, return requested URI or empty
		return requestedURI, nil

	case "lenient":
		// URI is optional, validated if provided, falls back to default
		if requestedURI == "" {
			// Use default_logout_uri or website as fallback
			if config.DefaultLogoutURI != nil && *config.DefaultLogoutURI != "" {
				return *config.DefaultLogoutURI, nil
			}
			if config.Website != "" {
				return config.Website, nil
			}
			return "", nil
		}

		// Validate if provided
		if h.isWhitelisted(requestedURI, config.PostLogoutRedirectURIs, config.AllowWildcardLogout) {
			return requestedURI, nil
		}

		// Validation failed, use default fallback
		h.logger.Warn("Logout URI not whitelisted, using default",
			zap.String("requested", requestedURI),
			zap.String("policy", "lenient"))

		if config.DefaultLogoutURI != nil && *config.DefaultLogoutURI != "" {
			return *config.DefaultLogoutURI, nil
		}
		if config.Website != "" {
			return config.Website, nil
		}
		return "", nil

	case "strict":
		// URI is required and must be whitelisted
		if requestedURI == "" {
			return "", fmt.Errorf("post_logout_redirect_uri is required (strict policy)")
		}

		if !h.isWhitelisted(requestedURI, config.PostLogoutRedirectURIs, config.AllowWildcardLogout) {
			return "", fmt.Errorf("post_logout_redirect_uri is not whitelisted")
		}

		return requestedURI, nil

	default:
		return "", fmt.Errorf("unknown logout_redirect_policy: %s", policy)
	}
}

// isWhitelisted checks if URI matches whitelist with optional wildcard support
func (h *LogoutHandler) isWhitelisted(uri string, whitelist []string, allowWildcard bool) bool {
	for _, whitelisted := range whitelist {
		if allowWildcard && h.matchesWildcard(uri, whitelisted) {
			return true
		}
		if uri == whitelisted {
			return true
		}
	}
	return false
}

// matchesWildcard checks if URI matches wildcard pattern
func (h *LogoutHandler) matchesWildcard(uri, pattern string) bool {
	// Simple wildcard matching for localhost:* and *.domain.com patterns
	if len(pattern) == 0 {
		return false
	}

	// localhost:* pattern
	if pattern == "http://localhost:*" || pattern == "https://localhost:*" {
		return len(uri) > len("http://localhost:") &&
			(uri[:17] == "http://localhost:" || uri[:18] == "https://localhost:")
	}

	// *.domain.com pattern
	if pattern[0] == '*' && len(pattern) > 1 {
		suffix := pattern[1:] // Remove leading *
		return len(uri) >= len(suffix) && uri[len(uri)-len(suffix):] == suffix
	}

	return false
}

// acceptLogout accepts the logout request with Hydra, forwarding the validated redirect URI.
func (h *LogoutHandler) acceptLogout(c *fiber.Ctx, challenge, postLogoutRedirectURI string) error {
	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/logout/accept?logout_challenge=%s",
		h.hydraAdminURL, challenge)

	var bodyBytes []byte
	if postLogoutRedirectURI != "" {
		body := map[string]string{"post_logout_redirect_uri": postLogoutRedirectURI}
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to marshal accept request",
			})
		}
	} else {
		bodyBytes = []byte("{}")
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create accept request",
		})
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.logger.Error("Failed to accept logout", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to accept logout",
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		h.logger.Error("Hydra accept logout failed",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(bodyBytes)))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Hydra logout accept failed",
		})
	}

	var result struct {
		RedirectTo string `json:"redirect_to"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.logger.Error("Failed to decode Hydra response", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to parse Hydra response",
		})
	}

	h.logger.Info("Logout accepted successfully",
		zap.String("challenge", challenge),
		zap.String("redirect_to", result.RedirectTo))

	// Redirect to Hydra logout completion URL
	return c.Redirect(result.RedirectTo, fiber.StatusFound)
}
