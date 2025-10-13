package shared

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// OAuthConfig holds OAuth 2.0 configuration
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}

// StateStore provides server-side OAuth state management
// IMPORTANT: Use server-side state storage instead of cookies to avoid SameSite issues
// In production, replace with Redis or similar distributed storage
type StateStore struct {
	mu     sync.RWMutex
	states map[string]*StateData
}

// StateData holds OAuth state information with expiration
type StateData struct {
	Value     string
	CreatedAt time.Time
	ExpiresAt time.Time
	Metadata  map[string]string // Store login_challenge, client_id, etc.
}

// NewStateStore creates a new server-side state store
func NewStateStore() *StateStore {
	return &StateStore{
		states: make(map[string]*StateData),
	}
}

// Store saves state data with automatic expiration (default: 15 minutes)
func (s *StateStore) Store(state string, metadata map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.states[state] = &StateData{
		Value:     state,
		CreatedAt: now,
		ExpiresAt: now.Add(15 * time.Minute),
		Metadata:  metadata,
	}
}

// Retrieve fetches and deletes state data (one-time use)
func (s *StateStore) Retrieve(state string) (*StateData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, exists := s.states[state]
	if !exists {
		return nil, false
	}

	// Check expiration
	if time.Now().After(data.ExpiresAt) {
		delete(s.states, state)
		return nil, false
	}

	// One-time use: delete after retrieval
	delete(s.states, state)
	return data, true
}

// CleanExpired removes expired states (call periodically)
func (s *StateStore) CleanExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, data := range s.states {
		if now.After(data.ExpiresAt) {
			delete(s.states, key)
		}
	}
}

// UserInfo represents user profile information
type UserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// Session represents user session data
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	UserInfo     UserInfo
}

// GenerateState generates a random state parameter for OAuth
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetOAuth2Config creates an oauth2.Config from OAuthConfig
func (c *OAuthConfig) GetOAuth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes:       c.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURL,
			TokenURL: c.TokenURL,
		},
	}
}

// GetAuthURL generates the authorization URL
func (c *OAuthConfig) GetAuthURL(state string) string {
	config := c.GetOAuth2Config()
	return config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges authorization code for tokens
func (c *OAuthConfig) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	config := c.GetOAuth2Config()
	return config.Exchange(ctx, code)
}

// GetUserInfo fetches user information using access token
func (c *OAuthConfig) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch user info: status %d, body: %s", resp.StatusCode, string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &userInfo, nil
}

// RefreshAccessToken refreshes the access token using refresh token
func (c *OAuthConfig) RefreshAccessToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	config := c.GetOAuth2Config()

	tokenSource := config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
	})

	return tokenSource.Token()
}

// RevokeToken revokes an access or refresh token
func (c *OAuthConfig) RevokeToken(ctx context.Context, token string) error {
	// Authway uses standard OAuth 2.0 token revocation
	revokeURL := strings.Replace(c.TokenURL, "/token", "/revoke", 1)

	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", c.ClientID)
	data.Set("client_secret", c.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", revokeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to revoke token: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
