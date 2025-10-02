package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"authway/src/server/internal/config"
	"authway/src/server/pkg/client"
	"authway/src/server/pkg/user"
	"go.uber.org/zap"
)

type GoogleService struct {
	config        *config.GoogleOAuthConfig
	userService   user.Service
	clientService client.Service
	logger        *zap.Logger
	httpClient    *http.Client
}

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}

type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

func NewGoogleService(cfg *config.GoogleOAuthConfig, userService user.Service, clientService client.Service, logger *zap.Logger) *GoogleService {
	return &GoogleService{
		config:        cfg,
		userService:   userService,
		clientService: clientService,
		logger:        logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GoogleOAuthConfig represents OAuth configuration for a specific client or central config
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// GetOAuthConfig returns the appropriate Google OAuth config (client-specific or central)
func (g *GoogleService) GetOAuthConfig(clientID string) (*GoogleOAuthConfig, error) {
	if clientID == "" {
		// No client specified, use central Authway config
		return &GoogleOAuthConfig{
			ClientID:     g.config.ClientID,
			ClientSecret: g.config.ClientSecret,
			RedirectURL:  g.config.RedirectURL,
		}, nil
	}

	// Check if client has its own Google OAuth configuration
	clientData, err := g.clientService.GetByClientID(clientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	if clientData.GoogleOAuthEnabled && clientData.GoogleClientID != nil && clientData.GoogleClientSecret != nil {
		// Use client-specific Google OAuth config
		redirectURI := g.config.RedirectURL // Default fallback
		if clientData.GoogleRedirectURI != nil && *clientData.GoogleRedirectURI != "" {
			redirectURI = *clientData.GoogleRedirectURI
		}

		return &GoogleOAuthConfig{
			ClientID:     *clientData.GoogleClientID,
			ClientSecret: *clientData.GoogleClientSecret,
			RedirectURL:  redirectURI,
		}, nil
	}

	// Fallback to central Authway config
	return &GoogleOAuthConfig{
		ClientID:     g.config.ClientID,
		ClientSecret: g.config.ClientSecret,
		RedirectURL:  g.config.RedirectURL,
	}, nil
}

// GetAuthURL returns the Google OAuth authorization URL
func (g *GoogleService) GetAuthURL(state string) string {
	return g.GetAuthURLForClient(state, "")
}

// GetAuthURLForClient returns the Google OAuth authorization URL for a specific client
func (g *GoogleService) GetAuthURLForClient(state string, clientID string) string {
	oauthConfig, err := g.GetOAuthConfig(clientID)
	if err != nil {
		g.logger.Error("Failed to get OAuth config, using central config", zap.Error(err))
		oauthConfig = &GoogleOAuthConfig{
			ClientID:     g.config.ClientID,
			ClientSecret: g.config.ClientSecret,
			RedirectURL:  g.config.RedirectURL,
		}
	}

	baseURL := "https://accounts.google.com/o/oauth2/v2/auth"
	params := url.Values{}
	params.Add("client_id", oauthConfig.ClientID)
	params.Add("redirect_uri", oauthConfig.RedirectURL)
	params.Add("response_type", "code")
	params.Add("scope", "openid email profile")
	params.Add("state", state)
	params.Add("access_type", "offline")
	params.Add("prompt", "consent")

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCode exchanges authorization code for access token
func (g *GoogleService) ExchangeCode(ctx context.Context, code string) (*GoogleTokenResponse, error) {
	return g.ExchangeCodeForClient(ctx, code, "")
}

// ExchangeCodeForClient exchanges authorization code for access token using client-specific or central config
func (g *GoogleService) ExchangeCodeForClient(ctx context.Context, code string, clientID string) (*GoogleTokenResponse, error) {
	oauthConfig, err := g.GetOAuthConfig(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth config: %w", err)
	}

	tokenURL := "https://oauth2.googleapis.com/token"

	data := url.Values{}
	data.Set("client_id", oauthConfig.ClientID)
	data.Set("client_secret", oauthConfig.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", oauthConfig.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp GoogleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetUserInfo retrieves user information from Google
func (g *GoogleService) GetUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	userInfoURL := "https://www.googleapis.com/oauth2/v2/userinfo"

	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &userInfo, nil
}

// HandleCallback processes the Google OAuth callback
func (g *GoogleService) HandleCallback(ctx context.Context, code, state string) (*user.User, error) {
	return g.HandleCallbackForClient(ctx, code, state, "")
}

// HandleCallbackForClient processes the Google OAuth callback for a specific client
func (g *GoogleService) HandleCallbackForClient(ctx context.Context, code, state string, clientID string) (*user.User, error) {
	g.logger.Info("Processing Google OAuth callback",
		zap.String("state", state),
		zap.String("code_length", fmt.Sprintf("%d", len(code))),
		zap.String("client_id", clientID))

	// Exchange authorization code for access token using client-specific or central config
	tokenResp, err := g.ExchangeCodeForClient(ctx, code, clientID)
	if err != nil {
		g.logger.Error("Failed to exchange authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user information from Google
	googleUser, err := g.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		g.logger.Error("Failed to get user info from Google", zap.Error(err))
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Check if user already exists
	existingUser, err := g.userService.GetByEmail(googleUser.Email)
	if err == nil {
		// User exists, update Google-specific fields
		existingUser.GoogleID = &googleUser.ID
		existingUser.Picture = &googleUser.Picture
		existingUser.EmailVerified = googleUser.VerifiedEmail

		updateReq := &user.UpdateUserRequest{
			AvatarURL: googleUser.Picture,
		}
		if _, err := g.userService.Update(existingUser.ID, updateReq); err != nil {
			g.logger.Error("Failed to update existing user", zap.Error(err))
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		g.logger.Info("Updated existing user with Google account",
			zap.String("user_id", existingUser.ID.String()),
			zap.String("email", existingUser.Email),
			zap.String("client_id", clientID))

		return existingUser, nil
	}

	// Create new user account
	fullName := strings.TrimSpace(googleUser.GivenName + " " + googleUser.FamilyName)
	createReq := &user.CreateUserRequest{
		Email:    googleUser.Email,
		Password: "", // Social login users don't need a password
		Name:     fullName,
	}

	newUser, err := g.userService.Create(createReq)
	if err != nil {
		g.logger.Error("Failed to create new user from Google", zap.Error(err))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	g.logger.Info("Created new user from Google account",
		zap.String("user_id", newUser.ID.String()),
		zap.String("email", newUser.Email),
		zap.String("client_id", clientID))

	return newUser, nil
}
