package hydra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		},
	}
}

// OAuth2 Client Management
type OAuth2Client struct {
	ClientID      string   `json:"client_id"`
	ClientName    string   `json:"client_name"`
	ClientSecret  string   `json:"client_secret,omitempty"`
	RedirectUris  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	Scope         string   `json:"scope"`
	Public        bool     `json:"token_endpoint_auth_method"`
}

func (c *Client) CreateOAuth2Client(client *OAuth2Client) (*OAuth2Client, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Post(
		fmt.Sprintf("%s/admin/clients", c.AdminURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
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
	ACR         string                 `json:"acr"`
	Context     map[string]interface{} `json:"context"`
}

type LoginResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func (c *Client) GetLoginRequest(challenge string) (*LoginRequest, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/login?challenge=%s", c.AdminURL, challenge),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var loginReq LoginRequest
	if err := json.NewDecoder(resp.Body).Decode(&loginReq); err != nil {
		return nil, err
	}

	return &loginReq, nil
}

func (c *Client) AcceptLoginRequest(challenge string, body *AcceptLoginRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

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

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, err
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

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, err
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
}

type AcceptConsentRequest struct {
	GrantScope    []string               `json:"grant_scope"`
	GrantAudience []string               `json:"grant_audience"`
	Remember      bool                   `json:"remember"`
	RememberFor   int                    `json:"remember_for"`
	HandledAt     time.Time              `json:"handled_at"`
	Session       map[string]interface{} `json:"session"`
}

func (c *Client) GetConsentRequest(challenge string) (*ConsentRequest, error) {
	resp, err := c.client.Get(
		fmt.Sprintf("%s/admin/oauth2/auth/requests/consent?challenge=%s", c.AdminURL, challenge),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var consentReq ConsentRequest
	if err := json.NewDecoder(resp.Body).Decode(&consentReq); err != nil {
		return nil, err
	}

	return &consentReq, nil
}

func (c *Client) AcceptConsentRequest(challenge string, body *AcceptConsentRequest) (*LoginResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

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

	var consentResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&consentResp); err != nil {
		return nil, err
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

	var consentResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&consentResp); err != nil {
		return nil, err
	}

	return &consentResp, nil
}
