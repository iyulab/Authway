package social

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AppleService struct {
	config        *config.AppleOAuthConfig
	userService   user.Service
	clientService client.Service
	logger        *zap.Logger
	httpClient    *http.Client
}

type AppleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

type AppleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
}

func NewAppleService(cfg *config.AppleOAuthConfig, userService user.Service, clientService client.Service, logger *zap.Logger) *AppleService {
	return &AppleService{
		config:        cfg,
		userService:   userService,
		clientService: clientService,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

type AppleOAuthConfig struct {
	ClientID    string
	TeamID      string
	KeyID       string
	PrivateKey  string
	RedirectURL string
}

func (a *AppleService) GetOAuthConfig(clientID string) (*AppleOAuthConfig, error) {
	return &AppleOAuthConfig{
		ClientID:    a.config.ClientID,
		TeamID:      a.config.TeamID,
		KeyID:       a.config.KeyID,
		PrivateKey:  a.config.PrivateKey,
		RedirectURL: a.config.RedirectURL,
	}, nil
}

func (a *AppleService) generateClientSecret(cfg *AppleOAuthConfig) (string, error) {
	block, _ := pem.Decode([]byte(cfg.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to parse private key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not ECDSA")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": cfg.TeamID,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour * 24 * 180).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": cfg.ClientID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = cfg.KeyID
	return token.SignedString(ecdsaKey)
}

func (a *AppleService) GetAuthURL(state string) string {
	return a.GetAuthURLForClient(state, "")
}

func (a *AppleService) GetAuthURLForClient(state string, clientID string) string {
	oauthConfig, _ := a.GetOAuthConfig(clientID)
	a.logger.Info("Building Apple OAuth URL", zap.String("apple_client_id", oauthConfig.ClientID))
	baseURL := "https://appleid.apple.com/auth/authorize"
	params := url.Values{}
	params.Add("client_id", oauthConfig.ClientID)
	params.Add("redirect_uri", oauthConfig.RedirectURL)
	params.Add("response_type", "code")
	params.Add("scope", "name email")
	params.Add("state", state)
	params.Add("response_mode", "form_post")
	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

func (a *AppleService) ExchangeCode(ctx context.Context, code string) (*AppleTokenResponse, error) {
	return a.ExchangeCodeForClient(ctx, code, "")
}

func (a *AppleService) ExchangeCodeForClient(ctx context.Context, code string, clientID string) (*AppleTokenResponse, error) {
	oauthConfig, err := a.GetOAuthConfig(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth config: %w", err)
	}
	clientSecret, err := a.generateClientSecret(oauthConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate client secret: %w", err)
	}
	tokenURL := "https://appleid.apple.com/auth/token"
	data := url.Values{}
	data.Set("client_id", oauthConfig.ClientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", oauthConfig.RedirectURL)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}
	var tokenResp AppleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tokenResp, nil
}

func (a *AppleService) parseIDToken(idToken string) (*AppleUserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}
	payload, err := jwt.NewParser().DecodeSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode ID token payload: %w", err)
	}
	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}
	return &AppleUserInfo{Sub: claims.Sub, Email: claims.Email}, nil
}

func (a *AppleService) HandleCallback(ctx context.Context, code, state string) (*user.User, error) {
	return a.HandleCallbackForClient(ctx, code, state, "")
}

func (a *AppleService) HandleCallbackForClient(ctx context.Context, code, state string, clientID string) (*user.User, error) {
	a.logger.Info("Processing Apple OAuth callback", zap.String("state", state), zap.String("code_length", fmt.Sprintf("%d", len(code))), zap.String("client_id", clientID))
	var clientTenantID uuid.UUID
	if clientID != "" {
		clientData, err := a.clientService.GetByClientID(clientID)
		if err != nil {
			a.logger.Error("Failed to get client for tenant determination", zap.Error(err))
			return nil, fmt.Errorf("failed to get client: %w", err)
		}
		clientTenantID = clientData.TenantID
	} else {
		a.logger.Warn("No client ID provided, using default tenant")
		return nil, fmt.Errorf("client_id required for tenant determination")
	}
	tokenResp, err := a.ExchangeCodeForClient(ctx, code, clientID)
	if err != nil {
		a.logger.Error("Failed to exchange authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	appleUser, err := a.parseIDToken(tokenResp.IDToken)
	if err != nil {
		a.logger.Error("Failed to parse ID token", zap.Error(err))
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}
	if appleUser.Email == "" {
		return nil, fmt.Errorf("email not available from Apple account")
	}
	existingUser, err := a.userService.GetByEmailAndTenant(clientTenantID, appleUser.Email)
	if err == nil {
		existingUser.AppleID = &appleUser.Sub
		updateReq := &user.UpdateUserRequest{}
		if _, err := a.userService.Update(existingUser.ID, updateReq); err != nil {
			a.logger.Error("Failed to update existing user", zap.Error(err))
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		a.logger.Info("Updated existing user with Apple account", zap.String("user_id", existingUser.ID.String()), zap.String("email", existingUser.Email))
		return existingUser, nil
	}
	createReq := &user.CreateUserRequest{Email: appleUser.Email, Password: "", Name: ""}
	newUser, err := a.userService.Create(clientTenantID, createReq)
	if err != nil {
		a.logger.Error("Failed to create new user from Apple", zap.Error(err))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	newUser.AppleID = &appleUser.Sub
	newUser.EmailVerified = true
	updateReq := &user.UpdateUserRequest{}
	if _, err := a.userService.Update(newUser.ID, updateReq); err != nil {
		a.logger.Warn("Failed to update new user with Apple fields", zap.Error(err))
	}
	a.logger.Info("Created new user from Apple account", zap.String("user_id", newUser.ID.String()), zap.String("email", newUser.Email))
	return newUser, nil
}
