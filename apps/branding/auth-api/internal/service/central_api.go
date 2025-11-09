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
