package accountlink

import (
	"encoding/json"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SocialUserInfo represents user info from social provider
type SocialUserInfo struct {
	ProviderID   string
	Email        string
	Name         string
	AvatarURL    string
	AccessToken  string
	RefreshToken string
	TokenExpiry  *time.Time
	Metadata     map[string]any
}

// SocialAuthProvider interface for social authentication providers
type SocialAuthProvider interface {
	GetProviderName() Provider
	ExchangeCode(code, redirectURI string) (*SocialUserInfo, error)
}

// Service provides account linking functionality
type Service interface {
	LinkAccount(userID, tenantID uuid.UUID, provider Provider, info *SocialUserInfo) (*LinkedAccount, error)
	UnlinkAccount(userID uuid.UUID, provider Provider) error
	GetLinkedAccounts(userID uuid.UUID) ([]LinkedAccount, error)
	GetLinkedAccountByProvider(userID uuid.UUID, provider Provider) (*LinkedAccount, error)
	FindUserByProviderID(tenantID uuid.UUID, provider Provider, providerID string) (*user.User, error)
	UpdateLastUsed(id uuid.UUID) error
	UpdateTokens(id uuid.UUID, accessToken, refreshToken string, expiry *time.Time) error
	GetAvailableProviders(userID uuid.UUID) ([]AvailableProvidersResponse, error)
}

type service struct {
	db          *gorm.DB
	userService user.Service
	providers   map[Provider]SocialAuthProvider
	logger      *zap.Logger
}

func NewService(db *gorm.DB, userService user.Service, logger *zap.Logger) Service {
	return &service{
		db:          db,
		userService: userService,
		providers:   make(map[Provider]SocialAuthProvider),
		logger:      logger,
	}
}

// RegisterProvider registers a social auth provider
func (s *service) RegisterProvider(provider SocialAuthProvider) {
	s.providers[provider.GetProviderName()] = provider
}

func (s *service) LinkAccount(userID, tenantID uuid.UUID, provider Provider, info *SocialUserInfo) (*LinkedAccount, error) {
	// Check if this provider account is already linked to another user
	var existing LinkedAccount
	err := s.db.Where("tenant_id = ? AND provider = ? AND provider_id = ?", tenantID, provider, info.ProviderID).First(&existing).Error
	if err == nil {
		if existing.UserID != userID {
			return nil, fmt.Errorf("this %s account is already linked to another user", provider)
		}
		// Update existing link
		existing.Email = info.Email
		existing.Name = info.Name
		existing.AvatarURL = info.AvatarURL
		existing.AccessToken = info.AccessToken
		existing.RefreshToken = info.RefreshToken
		existing.TokenExpiry = info.TokenExpiry
		if info.Metadata != nil {
			metadataJSON, _ := json.Marshal(info.Metadata)
			existing.Metadata = string(metadataJSON)
		}
		now := time.Now()
		existing.LastUsedAt = &now
		if err := s.db.Save(&existing).Error; err != nil {
			return nil, fmt.Errorf("failed to update linked account: %w", err)
		}
		s.logger.Info("Linked account updated", zap.String("user_id", userID.String()), zap.String("provider", string(provider)))
		return &existing, nil
	}

	// Check if user already has this provider linked
	err = s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("you already have a %s account linked", provider)
	}

	// Create new link
	var metadataJSON string
	if info.Metadata != nil {
		data, _ := json.Marshal(info.Metadata)
		metadataJSON = string(data)
	}
	linkedAccount := &LinkedAccount{
		UserID:       userID,
		TenantID:     tenantID,
		Provider:     provider,
		ProviderID:   info.ProviderID,
		Email:        info.Email,
		Name:         info.Name,
		AvatarURL:    info.AvatarURL,
		AccessToken:  info.AccessToken,
		RefreshToken: info.RefreshToken,
		TokenExpiry:  info.TokenExpiry,
		Metadata:     metadataJSON,
		LinkedAt:     time.Now(),
	}
	if err := s.db.Create(linkedAccount).Error; err != nil {
		return nil, fmt.Errorf("failed to link account: %w", err)
	}
	s.logger.Info("Account linked successfully", zap.String("user_id", userID.String()), zap.String("provider", string(provider)), zap.String("email", info.Email))
	return linkedAccount, nil
}

func (s *service) UnlinkAccount(userID uuid.UUID, provider Provider) error {
	// Check how many linked accounts user has
	var count int64
	s.db.Model(&LinkedAccount{}).Where("user_id = ?", userID).Count(&count)

	// Get the user to check if they have a password
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Prevent unlinking if this is the only way to log in
	hasPassword := u.PasswordHash != ""
	if count <= 1 && !hasPassword {
		return fmt.Errorf("cannot unlink: this is your only way to sign in. Please set a password first")
	}

	result := s.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&LinkedAccount{})
	if result.Error != nil {
		return fmt.Errorf("failed to unlink account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no linked %s account found", provider)
	}
	s.logger.Info("Account unlinked", zap.String("user_id", userID.String()), zap.String("provider", string(provider)))
	return nil
}

func (s *service) GetLinkedAccounts(userID uuid.UUID) ([]LinkedAccount, error) {
	var accounts []LinkedAccount
	if err := s.db.Where("user_id = ?", userID).Order("linked_at DESC").Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("failed to get linked accounts: %w", err)
	}
	return accounts, nil
}

func (s *service) GetLinkedAccountByProvider(userID uuid.UUID, provider Provider) (*LinkedAccount, error) {
	var account LinkedAccount
	if err := s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&account).Error; err != nil {
		return nil, fmt.Errorf("linked account not found")
	}
	return &account, nil
}

func (s *service) FindUserByProviderID(tenantID uuid.UUID, provider Provider, providerID string) (*user.User, error) {
	var linkedAccount LinkedAccount
	if err := s.db.Where("tenant_id = ? AND provider = ? AND provider_id = ?", tenantID, provider, providerID).First(&linkedAccount).Error; err != nil {
		return nil, fmt.Errorf("no user found with this %s account", provider)
	}
	return s.userService.GetByID(linkedAccount.UserID)
}

func (s *service) UpdateLastUsed(id uuid.UUID) error {
	now := time.Now()
	return s.db.Model(&LinkedAccount{}).Where("id = ?", id).Update("last_used_at", now).Error
}

func (s *service) UpdateTokens(id uuid.UUID, accessToken, refreshToken string, expiry *time.Time) error {
	updates := map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_expiry":  expiry,
	}
	return s.db.Model(&LinkedAccount{}).Where("id = ?", id).Updates(updates).Error
}

func (s *service) GetAvailableProviders(userID uuid.UUID) ([]AvailableProvidersResponse, error) {
	linkedAccounts, err := s.GetLinkedAccounts(userID)
	if err != nil {
		return nil, err
	}
	linkedProviders := make(map[Provider]bool)
	for _, acc := range linkedAccounts {
		linkedProviders[acc.Provider] = true
	}

	allProviders := []struct {
		Provider    Provider
		DisplayName string
	}{
		{ProviderGoogle, "Google"},
		{ProviderGitHub, "GitHub"},
		{ProviderMicrosoft, "Microsoft"},
		{ProviderApple, "Apple"},
	}

	result := make([]AvailableProvidersResponse, 0, len(allProviders))
	for _, p := range allProviders {
		result = append(result, AvailableProvidersResponse{
			Provider:    p.Provider,
			DisplayName: p.DisplayName,
			IsLinked:    linkedProviders[p.Provider],
		})
	}
	return result, nil
}
