package social

import (
	"context"
	"encoding/json"
	"errors"
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

type GoogleService struct {
	config        *config.GoogleOAuthConfig
	userService   user.Service
	invitations   InvitationGate
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

// Google's own endpoints, used unless the configuration names others.
const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

func endpoint(configured, google string) string {
	if configured != "" {
		return configured
	}
	return google
}

func NewGoogleService(cfg *config.GoogleOAuthConfig, userService user.Service, invitations InvitationGate, clientService client.Service, logger *zap.Logger) *GoogleService {
	if cfg.EndpointsOverridden() {
		logger.Warn("Google sign-in is pointed at configured endpoints, not at Google: whatever answers there decides who signs in",
			zap.String("auth_url", endpoint(cfg.AuthURL, googleAuthURL)),
			zap.String("token_url", endpoint(cfg.TokenURL, googleTokenURL)),
			zap.String("userinfo_url", endpoint(cfg.UserInfoURL, googleUserInfoURL)))
	}
	return &GoogleService{
		config:        cfg,
		userService:   userService,
		invitations:   invitations,
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
// ConfiguredFor reports whether sign-in with this provider can start for
// clientID — the client has its own credentials or the deployment does. With
// an empty clientID it reports the deployment's own configuration.
func (g *GoogleService) ConfiguredFor(clientID string) bool {
	cfg, err := g.GetOAuthConfig(clientID)
	return err == nil && cfg.ClientID != ""
}

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

	if clientData.UsesOwnGoogle() {
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

	// Debug log to verify Google OAuth config
	g.logger.Info("Building Google OAuth URL",
		zap.String("google_client_id", oauthConfig.ClientID),
		zap.String("redirect_url", oauthConfig.RedirectURL),
		zap.String("oauth_client_id", clientID))

	baseURL := endpoint(g.config.AuthURL, googleAuthURL)
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

	tokenURL := endpoint(g.config.TokenURL, googleTokenURL)

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
	userInfoURL := endpoint(g.config.UserInfoURL, googleUserInfoURL)

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
	tenantID, identity, err := g.identityFromCallback(ctx, code, state, clientID)
	if err != nil {
		return nil, err
	}
	return resolveAccount(g.userService, g.invitations, g.logger, tenantID, identity)
}

// ErrInvitationAddressMismatch refuses a sign-in that was started to accept an
// invitation but came back with a different address (or one Google has not
// verified): nothing is created or linked and the invitation stays as it was.
var ErrInvitationAddressMismatch = errors.New("the Google account is not the address the invitation was sent to")

// ErrInvitationAccountSuspended refuses accepting an invitation into an
// account an administrator has suspended; the account is left untouched.
var ErrInvitationAccountSuspended = errors.New("the account for the invited address is suspended")

// HandleCallbackForInvitation is HandleCallbackForClient for a sign-in that
// accepts the invitation sent to invitedEmail. Only a Google account that has
// verified that very address gets through — and it is checked before anything
// is created or linked, so any other account changes nothing.
func (g *GoogleService) HandleCallbackForInvitation(ctx context.Context, code, state, clientID, invitedEmail string) (*user.User, error) {
	tenantID, identity, err := g.identityFromCallback(ctx, code, state, clientID)
	if err != nil {
		return nil, err
	}
	if !identity.EmailVerified || !strings.EqualFold(strings.TrimSpace(identity.Email), strings.TrimSpace(invitedEmail)) {
		g.logger.Warn("Invitation sign-in refused: the Google address is not the invited one",
			zap.Bool("verified", identity.EmailVerified), zap.String("client_id", clientID))
		return nil, ErrInvitationAddressMismatch
	}
	// The account is found or made under the address as the invitation wrote
	// it, not as Google spells it: the lookups are exact, and a differently
	// cased spelling would miss the account and the invitation both.
	identity.Email = strings.TrimSpace(invitedEmail)
	// A suspended account is refused before anything is linked to it.
	if account, err := g.userService.GetByEmailAndTenant(tenantID, identity.Email); err == nil && !account.Active {
		return nil, ErrInvitationAccountSuspended
	}
	return resolveAccount(g.userService, g.invitations, g.logger, tenantID, identity)
}

// identityFromCallback exchanges the code and reads who Google says signed in,
// with the tenant of the client the sign-in was started for.
func (g *GoogleService) identityFromCallback(ctx context.Context, code, state, clientID string) (uuid.UUID, Identity, error) {
	g.logger.Info("Processing Google OAuth callback",
		zap.String("state", state),
		zap.String("code_length", fmt.Sprintf("%d", len(code))),
		zap.String("client_id", clientID))

	// The client decides the tenant.
	if clientID == "" {
		g.logger.Warn("No client ID provided for a Google sign-in")
		return uuid.Nil, Identity{}, fmt.Errorf("client_id required for tenant determination")
	}
	client, err := g.clientService.GetByClientID(clientID)
	if err != nil {
		g.logger.Error("Failed to get client for tenant determination", zap.Error(err))
		return uuid.Nil, Identity{}, fmt.Errorf("failed to get client: %w", err)
	}

	// Exchange authorization code for access token using client-specific or central config
	tokenResp, err := g.ExchangeCodeForClient(ctx, code, clientID)
	if err != nil {
		g.logger.Error("Failed to exchange authorization code", zap.Error(err))
		return uuid.Nil, Identity{}, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user information from Google
	googleUser, err := g.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		g.logger.Error("Failed to get user info from Google", zap.Error(err))
		return uuid.Nil, Identity{}, fmt.Errorf("failed to get user info: %w", err)
	}

	return client.TenantID, Identity{
		Provider:      "google",
		Subject:       googleUser.ID,
		Email:         googleUser.Email,
		EmailVerified: googleUser.VerifiedEmail,
		Name:          strings.TrimSpace(googleUser.GivenName + " " + googleUser.FamilyName),
		Picture:       googleUser.Picture,
	}, nil
}
