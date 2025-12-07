package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"authway/apps/branding/auth-api/internal/config"
	"go.uber.org/zap"
)

type CentralAPIClient struct {
	baseURL    string
	apiKey     string
	logger     *zap.Logger
	httpClient *http.Client
}

type AuthenticateGoogleUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	GoogleID string `json:"google_id"`
	Picture  string `json:"picture"`
	ClientID string `json:"client_id"`
}

type AuthenticateGoogleUserResponse struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
}

// ClientAuthConfig represents the authentication provider configuration for a client
type ClientAuthConfig struct {
	EnabledAuthProviders []string `json:"enabled_auth_providers"`
	AllowEmailSignup     bool     `json:"allow_email_signup"`
	AllowEmailLogin      bool     `json:"allow_email_login"`
	GoogleOAuthEnabled   bool     `json:"google_oauth_enabled"`
	GithubOAuthEnabled   bool     `json:"github_oauth_enabled"`
	MicrosoftOAuthEnabled bool    `json:"microsoft_oauth_enabled"`
	AppleOAuthEnabled    bool     `json:"apple_oauth_enabled"`
}

// GetClientResponse represents the response from Central API for client lookup
type GetClientResponse struct {
	Client struct {
		ClientID              string   `json:"client_id"`
		Name                  string   `json:"name"`
		EnabledAuthProviders  []string `json:"enabled_auth_providers"`
		AllowEmailSignup      bool     `json:"allow_email_signup"`
		AllowEmailLogin       bool     `json:"allow_email_login"`
		GoogleOAuthEnabled    bool     `json:"google_oauth_enabled"`
		GithubOAuthEnabled    bool     `json:"github_oauth_enabled"`
		MicrosoftOAuthEnabled bool     `json:"microsoft_oauth_enabled"`
		AppleOAuthEnabled     bool     `json:"apple_oauth_enabled"`
	} `json:"client"`
}

func NewCentralAPIClient(cfg *config.CentralAPIConfig, logger *zap.Logger) *CentralAPIClient {
	return &CentralAPIClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.InternalKey,
		logger:  logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AuthenticateGoogleUser calls central API to authenticate or create user
func (c *CentralAPIClient) AuthenticateGoogleUser(ctx context.Context, req *AuthenticateGoogleUserRequest) (*AuthenticateGoogleUserResponse, error) {
	url := fmt.Sprintf("%s/internal/auth/google", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)

	c.logger.Info("Calling central API for Google auth",
		zap.String("url", url),
		zap.String("email", req.Email),
		zap.String("client_id", req.ClientID))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call central API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("central API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp AuthenticateGoogleUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Info("Successfully authenticated user with central API",
		zap.String("user_id", authResp.UserID),
		zap.String("tenant_id", authResp.TenantID),
		zap.String("email", authResp.Email))

	return &authResp, nil
}

// GetClientByClientID retrieves client information from Central API by client_id
func (c *CentralAPIClient) GetClientByClientID(ctx context.Context, clientID string) (*ClientAuthConfig, error) {
	url := fmt.Sprintf("%s/api/v1/clients/by-client-id/%s", c.baseURL, clientID)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("X-API-Key", c.apiKey)

	c.logger.Debug("Fetching client from central API",
		zap.String("url", url),
		zap.String("client_id", clientID))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call central API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("central API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var clientResp GetClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&clientResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	config := &ClientAuthConfig{
		EnabledAuthProviders:  clientResp.Client.EnabledAuthProviders,
		AllowEmailSignup:      clientResp.Client.AllowEmailSignup,
		AllowEmailLogin:       clientResp.Client.AllowEmailLogin,
		GoogleOAuthEnabled:    clientResp.Client.GoogleOAuthEnabled,
		GithubOAuthEnabled:    clientResp.Client.GithubOAuthEnabled,
		MicrosoftOAuthEnabled: clientResp.Client.MicrosoftOAuthEnabled,
		AppleOAuthEnabled:     clientResp.Client.AppleOAuthEnabled,
	}

	// Set defaults if enabled_auth_providers is empty
	if len(config.EnabledAuthProviders) == 0 {
		config.EnabledAuthProviders = []string{"email", "google"}
	}

	c.logger.Debug("Successfully fetched client auth config",
		zap.String("client_id", clientID),
		zap.Strings("enabled_providers", config.EnabledAuthProviders))

	return config, nil
}
