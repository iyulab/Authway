package passwordless

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EmailSender interface for sending emails
type EmailSender interface {
	SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error
}

// Service provides passwordless authentication functionality
type Service interface {
	SendMagicLink(tenantID uuid.UUID, req *SendMagicLinkRequest, ipAddress, userAgent string) (*MagicLinkResponse, error)
	VerifyMagicLink(token string) (*MagicLink, *user.User, error)
	CleanupExpired() (int64, error)
}

type service struct {
	db          *gorm.DB
	userService user.Service
	emailSender EmailSender
	logger      *zap.Logger
	baseURL     string
	tokenExpiry time.Duration
}

func NewService(db *gorm.DB, userService user.Service, emailSender EmailSender, logger *zap.Logger, baseURL string) Service {
	return &service{
		db:          db,
		userService: userService,
		emailSender: emailSender,
		logger:      logger,
		baseURL:     baseURL,
		tokenExpiry: 15 * time.Minute,
	}
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *service) SendMagicLink(tenantID uuid.UUID, req *SendMagicLinkRequest, ipAddress, userAgent string) (*MagicLinkResponse, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	expiresAt := time.Now().Add(s.tokenExpiry)
	tokenType := TokenTypeLogin
	_, err = s.userService.GetByEmailAndTenant(tenantID, req.Email)
	if err != nil {
		tokenType = TokenTypeRegister
	}
	magicLink := &MagicLink{
		TenantID:    tenantID,
		Email:       req.Email,
		Token:       token,
		TokenType:   tokenType,
		ClientID:    req.ClientID,
		RedirectURI: req.RedirectURI,
		State:       req.State,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		ExpiresAt:   expiresAt,
	}
	if err := s.db.Create(magicLink).Error; err != nil {
		return nil, fmt.Errorf("failed to create magic link: %w", err)
	}
	linkURL := fmt.Sprintf("%s/auth/magic-link/verify?token=%s", s.baseURL, url.QueryEscape(token))
	if req.State != "" {
		linkURL = fmt.Sprintf("%s&state=%s", linkURL, url.QueryEscape(req.State))
	}
	if s.emailSender != nil {
		isNewUser := tokenType == TokenTypeRegister
		if err := s.emailSender.SendMagicLinkEmail(req.Email, linkURL, isNewUser); err != nil {
			s.logger.Error("Failed to send magic link email", zap.Error(err), zap.String("email", req.Email))
			return nil, fmt.Errorf("failed to send magic link email: %w", err)
		}
	}
	s.logger.Info("Magic link sent", zap.String("email", req.Email), zap.String("token_type", string(tokenType)), zap.String("tenant_id", tenantID.String()))
	return &MagicLinkResponse{
		Message:   "Magic link sent to your email",
		ExpiresAt: expiresAt,
	}, nil
}

func (s *service) VerifyMagicLink(token string) (*MagicLink, *user.User, error) {
	var magicLink MagicLink
	if err := s.db.Where("token = ?", token).First(&magicLink).Error; err != nil {
		return nil, nil, fmt.Errorf("invalid or expired token")
	}
	if magicLink.IsExpired() {
		return nil, nil, fmt.Errorf("magic link has expired")
	}
	if magicLink.IsUsed() {
		return nil, nil, fmt.Errorf("magic link has already been used")
	}
	now := time.Now()
	if err := s.db.Model(&magicLink).Update("used_at", now).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to mark token as used: %w", err)
	}
	var u *user.User
	var err error
	u, err = s.userService.GetByEmailAndTenant(magicLink.TenantID, magicLink.Email)
	if err != nil {
		createReq := &user.CreateUserRequest{
			Email:    magicLink.Email,
			Password: "",
			Name:     "",
		}
		u, err = s.userService.Create(magicLink.TenantID, createReq)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create user: %w", err)
		}
		u.EmailVerified = true
		s.userService.Update(u.ID, &user.UpdateUserRequest{})
		s.logger.Info("Created user via magic link", zap.String("user_id", u.ID.String()), zap.String("email", u.Email))
	} else {
		if !u.EmailVerified {
			u.EmailVerified = true
			s.userService.Update(u.ID, &user.UpdateUserRequest{})
		}
		s.logger.Info("User authenticated via magic link", zap.String("user_id", u.ID.String()), zap.String("email", u.Email))
	}
	return &magicLink, u, nil
}

func (s *service) CleanupExpired() (int64, error) {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&MagicLink{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired magic links: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		s.logger.Info("Cleaned up expired magic links", zap.Int64("deleted", result.RowsAffected))
	}
	return result.RowsAffected, nil
}
