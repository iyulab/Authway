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
