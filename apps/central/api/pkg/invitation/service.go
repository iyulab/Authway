package invitation

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EmailSender interface for sending emails
type EmailSender interface {
	SendInvitationEmail(toEmail, inviterName, tenantName, message, inviteURL string) error
}

// Service provides organization invitation functionality
type Service interface {
	Create(tenantID, inviterID uuid.UUID, req *CreateInvitationRequest) (*Invitation, error)
	GetByToken(token string) (*Invitation, error)
	GetByID(id uuid.UUID) (*Invitation, error)
	ListByTenant(tenantID uuid.UUID) ([]Invitation, error)
	ListPendingByEmail(email string) ([]Invitation, error)
	Accept(token string, userID *uuid.UUID, name, password string) (*user.User, error)
	Decline(token string) error
	Revoke(id uuid.UUID) error
	Resend(id uuid.UUID) error
	CleanupExpired() (int64, error)
}

type service struct {
	db            *gorm.DB
	userService   user.Service
	tenantService *tenant.Service
	emailSender   EmailSender
	logger        *zap.Logger
	baseURL       string
	expiry        time.Duration
}

func NewService(db *gorm.DB, userService user.Service, tenantService *tenant.Service, emailSender EmailSender, logger *zap.Logger, baseURL string) Service {
	return &service{
		db:            db,
		userService:   userService,
		tenantService: tenantService,
		emailSender:   emailSender,
		logger:        logger,
		baseURL:       baseURL,
		expiry:        7 * 24 * time.Hour,
	}
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *service) Create(tenantID, inviterID uuid.UUID, req *CreateInvitationRequest) (*Invitation, error) {
	t, err := s.tenantService.GetTenantByID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}
	inviter, err := s.userService.GetByID(inviterID)
	if err != nil {
		return nil, fmt.Errorf("inviter not found: %w", err)
	}
	existingUser, _ := s.userService.GetByEmailAndTenant(tenantID, req.Email)
	if existingUser != nil {
		return nil, fmt.Errorf("user already exists in this organization")
	}
	var existing Invitation
	if err := s.db.Where("tenant_id = ? AND email = ? AND status = ?", tenantID, req.Email, StatusPending).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("pending invitation already exists for this email")
	}
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	inviterName := inviter.Email
	if inviter.Name != nil && *inviter.Name != "" {
		inviterName = *inviter.Name
	}
	invitation := &Invitation{
		TenantID:    tenantID,
		TenantName:  t.Name,
		InviterID:   inviterID,
		InviterName: inviterName,
		Email:       req.Email,
		Role:        role,
		Token:       token,
		Status:      StatusPending,
		Message:     req.Message,
		ExpiresAt:   time.Now().Add(s.expiry),
	}
	if err := s.db.Create(invitation).Error; err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}
	if s.emailSender != nil {
		inviteURL := fmt.Sprintf("%s/invitation/accept?token=%s", s.baseURL, invitation.Token)
		if err := s.emailSender.SendInvitationEmail(req.Email, inviterName, t.Name, req.Message, inviteURL); err != nil {
			s.logger.Error("Failed to send invitation email", zap.Error(err), zap.String("email", req.Email))
		}
	}
	s.logger.Info("Invitation created", zap.String("invitation_id", invitation.ID.String()), zap.String("email", req.Email), zap.String("tenant_id", tenantID.String()))
	return invitation, nil
}

func (s *service) GetByToken(token string) (*Invitation, error) {
	var inv Invitation
	if err := s.db.Where("token = ?", token).First(&inv).Error; err != nil {
		return nil, fmt.Errorf("invitation not found")
	}
	return &inv, nil
}

func (s *service) GetByID(id uuid.UUID) (*Invitation, error) {
	var inv Invitation
	if err := s.db.Where("id = ?", id).First(&inv).Error; err != nil {
		return nil, fmt.Errorf("invitation not found")
	}
	return &inv, nil
}

func (s *service) ListByTenant(tenantID uuid.UUID) ([]Invitation, error) {
	var invitations []Invitation
	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&invitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	return invitations, nil
}

func (s *service) ListPendingByEmail(email string) ([]Invitation, error) {
	var invitations []Invitation
	if err := s.db.Where("email = ? AND status = ? AND expires_at > ?", email, StatusPending, time.Now()).Find(&invitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	return invitations, nil
}

func (s *service) Accept(token string, userID *uuid.UUID, name, password string) (*user.User, error) {
	inv, err := s.GetByToken(token)
	if err != nil {
		return nil, err
	}
	if !inv.CanBeAccepted() {
		if inv.IsExpired() {
			return nil, fmt.Errorf("invitation has expired")
		}
		return nil, fmt.Errorf("invitation cannot be accepted (status: %s)", inv.Status)
	}
	var u *user.User
	if userID != nil {
		u, err = s.userService.GetByID(*userID)
		if err != nil {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		if u.TenantID != inv.TenantID {
			return nil, fmt.Errorf("user belongs to a different organization")
		}
	} else {
		existingUser, _ := s.userService.GetByEmailAndTenant(inv.TenantID, inv.Email)
		if existingUser != nil {
			return nil, fmt.Errorf("user already exists, please log in to accept")
		}
		if password == "" {
			return nil, fmt.Errorf("password is required for new users")
		}
		userName := name
		if userName == "" {
			userName = inv.Email
		}
		createReq := &user.CreateUserRequest{
			Email:    inv.Email,
			Password: password,
			Name:     userName,
		}
		u, err = s.userService.Create(inv.TenantID, createReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		s.logger.Info("User created via invitation", zap.String("user_id", u.ID.String()), zap.String("email", u.Email))
	}
	now := time.Now()
	inv.Status = StatusAccepted
	inv.AcceptedAt = &now
	inv.AcceptedBy = &u.ID
	if err := s.db.Save(inv).Error; err != nil {
		return nil, fmt.Errorf("failed to update invitation: %w", err)
	}
	s.logger.Info("Invitation accepted", zap.String("invitation_id", inv.ID.String()), zap.String("user_id", u.ID.String()))
	return u, nil
}

func (s *service) Decline(token string) error {
	inv, err := s.GetByToken(token)
	if err != nil {
		return err
	}
	if inv.Status != StatusPending {
		return fmt.Errorf("invitation cannot be declined (status: %s)", inv.Status)
	}
	inv.Status = StatusDeclined
	if err := s.db.Save(inv).Error; err != nil {
		return fmt.Errorf("failed to decline invitation: %w", err)
	}
	s.logger.Info("Invitation declined", zap.String("invitation_id", inv.ID.String()))
	return nil
}

func (s *service) Revoke(id uuid.UUID) error {
	inv, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if inv.Status != StatusPending {
		return fmt.Errorf("only pending invitations can be revoked")
	}
	inv.Status = StatusRevoked
	if err := s.db.Save(inv).Error; err != nil {
		return fmt.Errorf("failed to revoke invitation: %w", err)
	}
	s.logger.Info("Invitation revoked", zap.String("invitation_id", inv.ID.String()))
	return nil
}

func (s *service) Resend(id uuid.UUID) error {
	inv, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if inv.Status != StatusPending {
		return fmt.Errorf("only pending invitations can be resent")
	}
	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("failed to generate new token: %w", err)
	}
	inv.Token = token
	inv.ExpiresAt = time.Now().Add(s.expiry)
	if err := s.db.Save(inv).Error; err != nil {
		return fmt.Errorf("failed to update invitation: %w", err)
	}
	if s.emailSender != nil {
		inviteURL := fmt.Sprintf("%s/invitation/accept?token=%s", s.baseURL, inv.Token)
		if err := s.emailSender.SendInvitationEmail(inv.Email, inv.InviterName, inv.TenantName, inv.Message, inviteURL); err != nil {
			s.logger.Error("Failed to resend invitation email", zap.Error(err), zap.String("email", inv.Email))
		}
	}
	s.logger.Info("Invitation resent", zap.String("invitation_id", inv.ID.String()))
	return nil
}

func (s *service) CleanupExpired() (int64, error) {
	result := s.db.Model(&Invitation{}).Where("status = ? AND expires_at < ?", StatusPending, time.Now()).Update("status", StatusExpired)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired invitations: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		s.logger.Info("Marked expired invitations", zap.Int64("count", result.RowsAffected))
	}
	return result.RowsAffected, nil
}
