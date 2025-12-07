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

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type GitHubService struct {
	config        *config.GitHubOAuthConfig
	userService   user.Service
	clientService client.Service
	logger        *zap.Logger
	httpClient    *http.Client
}

type GitHubUserInfo struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type GitHubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

type GitHubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

func NewGitHubService(cfg *config.GitHubOAuthConfig, userService user.Service, clientService client.Service, logger *zap.Logger) *GitHubService {
	return &GitHubService{
		config:        cfg,
		userService:   userService,
		clientService: clientService,
		logger:        logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func (g *GitHubService) GetOAuthConfig(clientID string) (*GitHubOAuthConfig, error) {
	if clientID == "" {
		return &GitHubOAuthConfig{ClientID: g.config.ClientID, ClientSecret: g.config.ClientSecret, RedirectURL: g.config.RedirectURL}, nil
	}
	clientData, err := g.clientService.GetByClientID(clientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}
	if clientData.GithubOAuthEnabled && clientData.GithubClientID != nil && clientData.GithubClientSecret != nil {
		return &GitHubOAuthConfig{ClientID: *clientData.GithubClientID, ClientSecret: *clientData.GithubClientSecret, RedirectURL: g.config.RedirectURL}, nil
	}
	return &GitHubOAuthConfig{ClientID: g.config.ClientID, ClientSecret: g.config.ClientSecret, RedirectURL: g.config.RedirectURL}, nil
}

func (g *GitHubService) GetAuthURL(state string) string {
	return g.GetAuthURLForClient(state, "")
}

func (g *GitHubService) GetAuthURLForClient(state string, clientID string) string {
	oauthConfig, err := g.GetOAuthConfig(clientID)
	if err != nil {
		g.logger.Error("Failed to get OAuth config", zap.Error(err))
		oauthConfig = &GitHubOAuthConfig{ClientID: g.config.ClientID, ClientSecret: g.config.ClientSecret, RedirectURL: g.config.RedirectURL}
	}
	g.logger.Info("Building GitHub OAuth URL", zap.String("github_client_id", oauthConfig.ClientID), zap.String("redirect_url", oauthConfig.RedirectURL))
	baseURL := "https://github.com/login/oauth/authorize"
	params := url.Values{}
	params.Add("client_id", oauthConfig.ClientID)
	params.Add("redirect_uri", oauthConfig.RedirectURL)
	params.Add("scope", "user:email read:user")
	params.Add("state", state)
	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

func (g *GitHubService) ExchangeCode(ctx context.Context, code string) (*GitHubTokenResponse, error) {
	return g.ExchangeCodeForClient(ctx, code, "")
}

func (g *GitHubService) ExchangeCodeForClient(ctx context.Context, code string, clientID string) (*GitHubTokenResponse, error) {
	oauthConfig, err := g.GetOAuthConfig(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth config: %w", err)
	}
	tokenURL := "https://github.com/login/oauth/access_token"
	data := url.Values{}
	data.Set("client_id", oauthConfig.ClientID)
	data.Set("client_secret", oauthConfig.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", oauthConfig.RedirectURL)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}
	var tokenResp GitHubTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tokenResp, nil
}

func (g *GitHubService) GetUserInfo(ctx context.Context, accessToken string) (*GitHubUserInfo, error) {
	userInfoURL := "https://api.github.com/user"
	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body))
	}
	var userInfo GitHubUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}
	if userInfo.Email == "" {
		email, err := g.getPrimaryEmail(ctx, accessToken)
		if err != nil {
			g.logger.Warn("Failed to get primary email", zap.Error(err))
		} else {
			userInfo.Email = email
		}
	}
	return &userInfo, nil
}

func (g *GitHubService) getPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	emailsURL := "https://api.github.com/user/emails"
	req, err := http.NewRequestWithContext(ctx, "GET", emailsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create emails request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get emails: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("emails request failed with status %d: %s", resp.StatusCode, string(body))
	}
	var emails []GitHubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("failed to decode emails: %w", err)
	}
	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}
	for _, email := range emails {
		if email.Verified {
			return email.Email, nil
		}
	}
	return "", fmt.Errorf("no verified email found")
}

func (g *GitHubService) HandleCallback(ctx context.Context, code, state string) (*user.User, error) {
	return g.HandleCallbackForClient(ctx, code, state, "")
}

func (g *GitHubService) HandleCallbackForClient(ctx context.Context, code, state string, clientID string) (*user.User, error) {
	g.logger.Info("Processing GitHub OAuth callback", zap.String("state", state), zap.String("code_length", fmt.Sprintf("%d", len(code))), zap.String("client_id", clientID))
	var clientTenantID uuid.UUID
	if clientID != "" {
		clientData, err := g.clientService.GetByClientID(clientID)
		if err != nil {
			g.logger.Error("Failed to get client for tenant determination", zap.Error(err))
			return nil, fmt.Errorf("failed to get client: %w", err)
		}
		clientTenantID = clientData.TenantID
	} else {
		g.logger.Warn("No client ID provided, using default tenant")
		return nil, fmt.Errorf("client_id required for tenant determination")
	}
	tokenResp, err := g.ExchangeCodeForClient(ctx, code, clientID)
	if err != nil {
		g.logger.Error("Failed to exchange authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	githubUser, err := g.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		g.logger.Error("Failed to get user info from GitHub", zap.Error(err))
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	if githubUser.Email == "" {
		return nil, fmt.Errorf("email not available from GitHub account")
	}
	githubIDStr := fmt.Sprintf("%d", githubUser.ID)
	existingUser, err := g.userService.GetByEmailAndTenant(clientTenantID, githubUser.Email)
	if err == nil {
		existingUser.GithubID = &githubIDStr
		if githubUser.AvatarURL != "" {
			existingUser.Picture = &githubUser.AvatarURL
		}
		updateReq := &user.UpdateUserRequest{AvatarURL: githubUser.AvatarURL}
		if _, err := g.userService.Update(existingUser.ID, updateReq); err != nil {
			g.logger.Error("Failed to update existing user", zap.Error(err))
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		g.logger.Info("Updated existing user with GitHub account", zap.String("user_id", existingUser.ID.String()), zap.String("email", existingUser.Email))
		return existingUser, nil
	}
	fullName := githubUser.Name
	if fullName == "" {
		fullName = githubUser.Login
	}
	createReq := &user.CreateUserRequest{Email: githubUser.Email, Password: "", Name: fullName}
	newUser, err := g.userService.Create(clientTenantID, createReq)
	if err != nil {
		g.logger.Error("Failed to create new user from GitHub", zap.Error(err))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	newUser.GithubID = &githubIDStr
	if githubUser.AvatarURL != "" {
		newUser.Picture = &githubUser.AvatarURL
	}
	newUser.EmailVerified = true
	updateReq := &user.UpdateUserRequest{AvatarURL: githubUser.AvatarURL}
	if _, err := g.userService.Update(newUser.ID, updateReq); err != nil {
		g.logger.Warn("Failed to update new user with GitHub fields", zap.Error(err))
	}
	g.logger.Info("Created new user from GitHub account", zap.String("user_id", newUser.ID.String()), zap.String("email", newUser.Email))
	return newUser, nil
}
