package hydra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	AdminURL string
	client   *http.Client
}

func NewClient(adminURL string) *Client {
	return &Client{
		AdminURL: adminURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - return error to stop redirect following
				// This prevents PUT requests from being converted to GET on redirect
				return http.ErrUseLastResponse
			},
		},
	}
}

// OAuth2 Client Management
type OAuth2Client struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	RedirectUris            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

func (c *Client) CreateOAuth2Client(client *OAuth2Client) (*OAuth2Client, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}

	// DEBUG: Log the AdminURL and full URL being used
	fullURL := fmt.Sprintf("%s/admin/clients", c.AdminURL)
	fmt.Printf("🔍 DEBUG Hydra Client: AdminURL='%s', FullURL='%s'\n", c.AdminURL, fullURL)

	resp, err := c.client.Post(
		fullURL,
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		fmt.Printf("❌ DEBUG Hydra Client: POST request failed with error: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create client: %s", string(body))
	}

	var result OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) GetOAuth2Client(clientID string) (*OAuth2Client, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client not found")
	}

	var client OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&client); err != nil {
		return nil, err
	}

	return &client, nil
}

func (c *Client) UpdateOAuth2Client(clientID string, client *OAuth2Client) (*OAuth2Client, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update client: %s", string(body))
	}

	var result OAuth2Client
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) DeleteOAuth2Client(clientID string) error {
	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/clients/%s", c.AdminURL, clientID),
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete client: %s", string(body))
	}

	return nil
}

// Login and Consent Flow Management
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

type AcceptLoginRequest struct {
	Subject     string                 `json:"subject"`
	Remember    bool                   `json:"remember"`
	RememberFor int                    `json:"remember_for"`
	ACR         string                 `json:"acr,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

type LoginResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func (c *Client) GetLoginRequest(challenge string) (*LoginRequest, error) {
	url := fmt.Sprintf("%s/admin/oauth2/auth/requests/login?challenge=%s", c.AdminURL, challenge)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request Hydra at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra returned status %d: %s", resp.StatusCode, string(body))
	}

	var loginReq LoginRequest
	if err := json.NewDecoder(resp.Body).Decode(&loginReq); err != nil {
		return nil, fmt.Errorf("failed to decode login request: %w", err)
	}

	return &loginReq, nil
}

func (c *Client) AcceptLoginRequest(challenge string, body *AcceptLoginRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// Debug: log the request
	fmt.Printf("🔍 Hydra AcceptLoginRequest body: %s\n", string(data))
	fmt.Printf("🔍 Hydra AcceptLoginRequest challenge (first 50 chars): %.50s\n", challenge)

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login/accept?challenge=%s", c.AdminURL, challenge),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: log the response
	fmt.Printf("🔍 Hydra AcceptLoginRequest response status: %d\n", resp.StatusCode)
	fmt.Printf("🔍 Hydra AcceptLoginRequest response body: %s\n", string(bodyBytes))

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w, body: %s", err, string(bodyBytes))
	}

	return &loginResp, nil
}

func (c *Client) RejectLoginRequest(challenge string, error_code, error_description string) (*LoginResponse, error) {
	body := map[string]string{
		"error":             error_code,
		"error_description": error_description,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login/reject?challenge=%s", c.AdminURL, challenge),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra reject login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w, body: %s", err, string(bodyBytes))
	}

	return &loginResp, nil
}

// Consent Flow
type ConsentRequest struct {
	Challenge         string                 `json:"challenge"`
	RequestedScope    []string               `json:"requested_scope"`
	RequestedAudience []string               `json:"requested_audience"`
	Subject           string                 `json:"subject"`
	Client            *OAuth2Client          `json:"client"`
	LoginChallenge    string                 `json:"login_challenge"`
	LoginSessionID    string                 `json:"login_session_id"`
	ACR               string                 `json:"acr"`
	Context           map[string]interface{} `json:"context"`
	Skip              bool                   `json:"skip"`
}

type AcceptConsentRequest struct {
	GrantScope               []string        `json:"grant_scope"`
	GrantAccessTokenAudience []string        `json:"grant_access_token_audience,omitempty"`
	Remember                 bool            `json:"remember"`
	RememberFor              int             `json:"remember_for"`
	Session                  *ConsentSession `json:"session,omitempty"`
}

type ConsentSession struct {
	AccessToken map[string]interface{} `json:"access_token,omitempty"`
	IDToken     map[string]interface{} `json:"id_token,omitempty"`
}

func (c *Client) GetConsentRequest(challenge string) (*ConsentRequest, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent?challenge=%s", c.AdminURL, challenge),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra get consent failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var consentReq ConsentRequest
	if err := json.Unmarshal(bodyBytes, &consentReq); err != nil {
		return nil, fmt.Errorf("failed to decode consent request: %w, body: %s", err, string(bodyBytes))
	}

	return &consentReq, nil
}

func (c *Client) AcceptConsentRequest(challenge string, body *AcceptConsentRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// Debug: log the request body
	fmt.Printf("🔍 Hydra AcceptConsentRequest body: %s\n", string(data))

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent/accept?challenge=%s", c.AdminURL, challenge),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: log the response
	fmt.Printf("🔍 Hydra response status: %d, body: %s\n", resp.StatusCode, string(bodyBytes))

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept consent failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var consentResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &consentResp); err != nil {
		return nil, fmt.Errorf("failed to decode consent response: %w, body: %s", err, string(bodyBytes))
	}

	return &consentResp, nil
}

func (c *Client) RejectConsentRequest(challenge string, error_code, error_description string) (*LoginResponse, error) {
	body := map[string]string{
		"error":             error_code,
		"error_description": error_description,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent/reject?challenge=%s", c.AdminURL, challenge),
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body for proper error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra reject consent failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var consentResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &consentResp); err != nil {
		return nil, fmt.Errorf("failed to decode consent response: %w, body: %s", err, string(bodyBytes))
	}

	return &consentResp, nil
}

// Session Management
// RevokeUserSessions revokes all OAuth2 sessions for a specific user
func (c *Client) RevokeUserSessions(subject string) error {
	// Revoke login sessions
	req, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/oauth2/auth/sessions/login?subject=%s", c.AdminURL, subject),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create revoke login sessions request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke login sessions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to revoke login sessions: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Revoke consent sessions
	req, err = http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/admin/oauth2/auth/sessions/consent?subject=%s&all=true", c.AdminURL, subject),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create revoke consent sessions request: %w", err)
	}

	resp, err = c.client.Do(req)
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

// Logout Flow
type LogoutRequest struct {
	Challenge             string `json:"challenge"`
	Subject               string `json:"subject"`
	SessionID             string `json:"sid"`
	RequestURL            string `json:"request_url"`
	RPInitiated           bool   `json:"rp_initiated"`
	PostLogoutRedirectURI string `json:"post_logout_redirect_uri,omitempty"`
}

type LogoutResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func (c *Client) GetLogoutRequest(challenge string) (*LogoutRequest, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/logout?challenge=%s", c.AdminURL, challenge),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra get logout failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var logoutReq LogoutRequest
	if err := json.Unmarshal(bodyBytes, &logoutReq); err != nil {
		return nil, fmt.Errorf("failed to decode logout request: %w, body: %s", err, string(bodyBytes))
	}

	return &logoutReq, nil
}

func (c *Client) AcceptLogoutRequest(challenge string) (*LogoutResponse, error) {
	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/admin/oauth2/auth/requests/logout/accept?challenge=%s", c.AdminURL, challenge),
		nil,
	)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hydra accept logout failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var logoutResp LogoutResponse
	if err := json.Unmarshal(bodyBytes, &logoutResp); err != nil {
		return nil, fmt.Errorf("failed to decode logout response: %w, body: %s", err, string(bodyBytes))
	}

	return &logoutResp, nil
}

// Token Introspection
type IntrospectRequest struct {
	Token string `json:"token"`
	Scope string `json:"scope,omitempty"`
}

type IntrospectResponse struct {
	Active    bool                   `json:"active"`
	Subject   string                 `json:"sub,omitempty"`
	ClientID  string                 `json:"client_id,omitempty"`
	Scope     string                 `json:"scope,omitempty"`
	TokenType string                 `json:"token_type,omitempty"`
	Exp       int64                  `json:"exp,omitempty"`
	Iat       int64                  `json:"iat,omitempty"`
	Ext       map[string]interface{} `json:"ext,omitempty"`
}

func (c *Client) IntrospectToken(token string) (*IntrospectResponse, error) {
	// OAuth 2.0 Token Introspection requires form-encoded data
	formData := url.Values{}
	formData.Set("token", token)

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/admin/oauth2/introspect", c.AdminURL),
		strings.NewReader(formData.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var introspectResp IntrospectResponse
	if err := json.Unmarshal(bodyBytes, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to decode introspection response: %w, body: %s", err, string(bodyBytes))
	}

	return &introspectResp, nil
}
