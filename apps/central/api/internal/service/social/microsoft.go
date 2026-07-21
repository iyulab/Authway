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

type MicrosoftService struct {
	config        *config.MicrosoftOAuthConfig
	userService   user.Service
	invitations   InvitationGate
	clientService client.Service
	logger        *zap.Logger
	httpClient    *http.Client
}

type MicrosoftUserInfo struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	GivenName         string `json:"givenName"`
	Surname           string `json:"surname"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

type MicrosoftTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

func NewMicrosoftService(cfg *config.MicrosoftOAuthConfig, userService user.Service, invitations InvitationGate, clientService client.Service, logger *zap.Logger) *MicrosoftService {
	return &MicrosoftService{
		config:        cfg,
		userService:   userService,
		invitations:   invitations,
		clientService: clientService,
		logger:        logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type MicrosoftOAuthConfig struct {
	ClientID     string
	ClientSecret string
	TenantID     string
	RedirectURL  string
}

func (m *MicrosoftService) GetOAuthConfig(clientID string) (*MicrosoftOAuthConfig, error) {
	return &MicrosoftOAuthConfig{ClientID: m.config.ClientID, ClientSecret: m.config.ClientSecret, TenantID: m.config.TenantID, RedirectURL: m.config.RedirectURL}, nil
}

func (m *MicrosoftService) GetAuthURL(state string) string {
	return m.GetAuthURLForClient(state, "")
}

func (m *MicrosoftService) GetAuthURLForClient(state string, clientID string) string {
	oauthConfig, err := m.GetOAuthConfig(clientID)
	if err != nil {
		m.logger.Error("Failed to get OAuth config", zap.Error(err))
		oauthConfig = &MicrosoftOAuthConfig{ClientID: m.config.ClientID, ClientSecret: m.config.ClientSecret, TenantID: m.config.TenantID, RedirectURL: m.config.RedirectURL}
	}
	tenantID := oauthConfig.TenantID
	if tenantID == "" {
		tenantID = "common"
	}
	m.logger.Info("Building Microsoft OAuth URL", zap.String("microsoft_client_id", oauthConfig.ClientID), zap.String("tenant_id", tenantID))
	baseURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenantID)
	params := url.Values{}
	params.Add("client_id", oauthConfig.ClientID)
	params.Add("redirect_uri", oauthConfig.RedirectURL)
	params.Add("response_type", "code")
	params.Add("scope", "openid email profile User.Read")
	params.Add("state", state)
	params.Add("response_mode", "query")
	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

func (m *MicrosoftService) ExchangeCode(ctx context.Context, code string) (*MicrosoftTokenResponse, error) {
	return m.ExchangeCodeForClient(ctx, code, "")
}

func (m *MicrosoftService) ExchangeCodeForClient(ctx context.Context, code string, clientID string) (*MicrosoftTokenResponse, error) {
	oauthConfig, err := m.GetOAuthConfig(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth config: %w", err)
	}
	tenantID := oauthConfig.TenantID
	if tenantID == "" {
		tenantID = "common"
	}
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	data := url.Values{}
	data.Set("client_id", oauthConfig.ClientID)
	data.Set("client_secret", oauthConfig.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", oauthConfig.RedirectURL)
	data.Set("scope", "openid email profile User.Read")
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}
	var tokenResp MicrosoftTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tokenResp, nil
}

func (m *MicrosoftService) GetUserInfo(ctx context.Context, accessToken string) (*MicrosoftUserInfo, error) {
	userInfoURL := "https://graph.microsoft.com/v1.0/me"
	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(body))
	}
	var userInfo MicrosoftUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}
	return &userInfo, nil
}

func (m *MicrosoftService) HandleCallback(ctx context.Context, code, state string) (*user.User, error) {
	return m.HandleCallbackForClient(ctx, code, state, "")
}

func (m *MicrosoftService) HandleCallbackForClient(ctx context.Context, code, state string, clientID string) (*user.User, error) {
	m.logger.Info("Processing Microsoft OAuth callback", zap.String("state", state), zap.String("code_length", fmt.Sprintf("%d", len(code))), zap.String("client_id", clientID))
	var clientTenantID uuid.UUID
	if clientID != "" {
		clientData, err := m.clientService.GetByClientID(clientID)
		if err != nil {
			m.logger.Error("Failed to get client for tenant determination", zap.Error(err))
			return nil, fmt.Errorf("failed to get client: %w", err)
		}
		clientTenantID = clientData.TenantID
	} else {
		m.logger.Warn("No client ID provided, using default tenant")
		return nil, fmt.Errorf("client_id required for tenant determination")
	}
	tokenResp, err := m.ExchangeCodeForClient(ctx, code, clientID)
	if err != nil {
		m.logger.Error("Failed to exchange authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	msUser, err := m.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		m.logger.Error("Failed to get user info from Microsoft", zap.Error(err))
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	email := msUser.Mail
	if email == "" {
		email = msUser.UserPrincipalName
	}
	if email == "" {
		return nil, fmt.Errorf("email not available from Microsoft account")
	}
	existingUser, err := m.userService.GetByEmailAndTenant(clientTenantID, email)
	if err == nil {
		existingUser.MicrosoftID = &msUser.ID
		updateReq := &user.UpdateUserRequest{}
		if _, err := m.userService.Update(existingUser.ID, updateReq); err != nil {
			m.logger.Error("Failed to update existing user", zap.Error(err))
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		m.logger.Info("Updated existing user with Microsoft account", zap.String("user_id", existingUser.ID.String()), zap.String("email", existingUser.Email))
		return existingUser, nil
	}
	// Onboarding is invitation-only: a first-time sign-in may create an account
	// only for an address that was invited into this tenant.
	if !mayProvision(m.invitations, m.logger, clientTenantID, email) {
		m.logger.Warn("Social sign-in denied for uninvited address", zap.String("email", email))
		return nil, fmt.Errorf("%s", ErrNotInvited)
	}
	fullName := msUser.DisplayName
	if fullName == "" {
		fullName = strings.TrimSpace(msUser.GivenName + " " + msUser.Surname)
	}
	createReq := &user.CreateUserRequest{Email: email, Password: "", Name: fullName}
	newUser, err := m.userService.Create(clientTenantID, createReq)
	if err != nil {
		m.logger.Error("Failed to create new user from Microsoft", zap.Error(err))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	newUser.MicrosoftID = &msUser.ID
	newUser.EmailVerified = true
	updateReq := &user.UpdateUserRequest{}
	if _, err := m.userService.Update(newUser.ID, updateReq); err != nil {
		m.logger.Warn("Failed to update new user with Microsoft fields", zap.Error(err))
	}
	m.logger.Info("Created new user from Microsoft account", zap.String("user_id", newUser.ID.String()), zap.String("email", newUser.Email))
	return newUser, nil
}
