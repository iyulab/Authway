package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"authway/apps/branding/auth-api/internal/config"
	"go.uber.org/zap"
)

type HydraClient struct {
	adminURL   string
	logger     *zap.Logger
	httpClient *http.Client
}

// OAuth2Client represents a Hydra OAuth2 client (returned in LoginRequest)
type OAuth2Client struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectUris            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// LoginRequest represents Hydra login request details
type LoginRequest struct {
	Challenge         string        `json:"challenge"`
	RequestedScope    []string      `json:"requested_scope"`
	RequestedAudience []string      `json:"requested_audience"`
	Subject           string        `json:"subject"`
	Client            *OAuth2Client `json:"client"`
	RequestURL        string        `json:"request_url"`
	SessionID         string        `json:"session_id"`
	Skip              bool          `json:"skip"`
}

// AcceptLoginRequest is the body for accepting a login request
type AcceptLoginRequest struct {
	Subject     string                 `json:"subject"`
	Remember    bool                   `json:"remember"`
	RememberFor int                    `json:"remember_for"`
	ACR         string                 `json:"acr,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
}

// LoginResponse contains the redirect URL after accepting/rejecting login
type LoginResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func NewHydraClient(cfg *config.HydraConfig, logger *zap.Logger) *HydraClient {
	return &HydraClient{
		adminURL: cfg.AdminURL,
		logger:   logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - return error to stop redirect following
				// This prevents PUT requests from being converted to GET on redirect
				return http.ErrUseLastResponse
			},
		},
	}
}

// GetLoginRequest retrieves the login request details from Hydra
func (h *HydraClient) GetLoginRequest(challenge string) (*LoginRequest, error) {
	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/login?challenge=%s", h.adminURL, challenge)

	challengePrefix := challenge[:min(10, len(challenge))] + "..."
	h.logger.Info("Getting Hydra login request",
		zap.String("url", url),
		zap.String("challenge", challengePrefix))

	resp, err := h.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request Hydra: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginReq LoginRequest
	if err := json.Unmarshal(bodyBytes, &loginReq); err != nil {
		return nil, fmt.Errorf("failed to decode login request: %w", err)
	}

	return &loginReq, nil
}

// AcceptLoginRequest accepts the login request and returns redirect URL
func (h *HydraClient) AcceptLoginRequest(challenge string, body *AcceptLoginRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/login/accept?challenge=%s", h.adminURL, challenge)

	h.logger.Info("Accepting Hydra login request",
		zap.String("url", url),
		zap.String("subject", body.Subject))

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Hydra: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	h.logger.Info("Successfully accepted Hydra login",
		zap.String("redirect_to", loginResp.RedirectTo))

	return &loginResp, nil
}

// RejectLoginRequest rejects the login request
func (h *HydraClient) RejectLoginRequest(challenge string, errorCode, errorDescription string) (*LoginResponse, error) {
	body := map[string]string{
		"error":             errorCode,
		"error_description": errorDescription,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/login/reject?challenge=%s", h.adminURL, challenge)

	h.logger.Warn("Rejecting Hydra login request",
		zap.String("error_code", errorCode),
		zap.String("error_description", errorDescription))

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Hydra: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra reject login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &loginResp, nil
}

// RevokeUserSessions revokes every one of the subject's Hydra login and
// consent sessions (all=true — every client, matching the "Single Logout"
// name this revocation is wired for). Accepting a Hydra RP-initiated logout
// request only ends the browser's login session; it does not by itself
// invalidate previously issued access/refresh tokens. Hydra does invalidate
// the opaque tokens tied to a consent session when that session is deleted,
// which is what actually closes the gap this method exists for.
func (h *HydraClient) RevokeUserSessions(subject string) error {
	escapedSubject := url.QueryEscape(subject)

	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/oauth2/auth/sessions/login?subject=%s", h.adminURL, escapedSubject),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create revoke login sessions request: %w", err)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke login sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to revoke login sessions: status=%d body=%s", resp.StatusCode, string(body))
	}

	req, err = http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/oauth2/auth/sessions/consent?subject=%s&all=true", h.adminURL, escapedSubject),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create revoke consent sessions request: %w", err)
	}
	resp, err = h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke consent sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to revoke consent sessions: status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}
