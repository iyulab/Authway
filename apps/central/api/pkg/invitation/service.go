package invitation

import (
	"fmt"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/maillink"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tokenhash"
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
	// Create issues an invitation. inviterID is nil when the caller is the
	// system actor (admin API key) rather than a signed-in user.
	Create(tenantID uuid.UUID, inviterID *uuid.UUID, req *CreateInvitationRequest) (*Invitation, error)
	// HasValidInvitation reports whether (tenantID, email) has a pending,
	// unexpired invitation. Auto-provisioning paths (magic link, social login)
	// use this to enforce the invitation-only onboarding policy, which would
	// otherwise exist only in comments.
	HasValidInvitation(tenantID uuid.UUID, email string) (bool, error)
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
	frontendURL   string
	expiry        time.Duration
}

func NewService(db *gorm.DB, userService user.Service, tenantService *tenant.Service, emailSender EmailSender, logger *zap.Logger, frontendURL string) Service {
	return &service{
		db:            db,
		userService:   userService,
		tenantService: tenantService,
		emailSender:   emailSender,
		logger:        logger,
		frontendURL:   frontendURL,
		expiry:        7 * 24 * time.Hour,
	}
}

// generateToken defers to the shared primitive, which emits unpadded base64url.
// The old local version used padded encoding, so every token ended in "=" — a
// character that has to be escaped in the URL path the accept page reads it
// from. See handler.GetInvitationByToken.
func generateToken() (string, error) {
	return tokenhash.Generate()
}

func (s *service) Create(tenantID uuid.UUID, inviterID *uuid.UUID, req *CreateInvitationRequest) (*Invitation, error) {
	t, err := s.tenantService.GetTenantByID(tenantID)
	if err != nil {
		return nil, apierror.NewPublic("tenant not found")
	}
	// A nil inviter is the system actor (admin API key): there is no user row to
	// look up, and requiring one is what made a fresh instance un-bootstrappable.
	inviterName := SystemInviterName
	if inviterID != nil {
		inviter, err := s.userService.GetByID(*inviterID)
		if err != nil {
			return nil, apierror.NewPublic("inviter not found")
		}
		if inviter.TenantID != tenantID {
			return nil, apierror.NewPublic("inviter belongs to a different organization")
		}
		inviterName = displayName(inviter)
	}
	existingUser, _ := s.userService.GetByEmailAndTenant(tenantID, req.Email)
	if existingUser != nil {
		return nil, apierror.NewPublic("user already exists in this organization")
	}
	var existing Invitation
	if err := s.db.Where("tenant_id = ? AND email = ? AND status = ?", tenantID, req.Email, StatusPending).First(&existing).Error; err == nil {
		return nil, apierror.NewPublic("pending invitation already exists for this email")
	}
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	invitation := &Invitation{
		TenantID:  tenantID,
		InviterID: inviterID,
		Email:     req.Email,
		Role:      role,
		Token:     token,
		Status:    StatusPending,
		Message:   req.Message,
		ExpiresAt: time.Now().Add(s.expiry),
	}
	if err := s.db.Create(invitation).Error; err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}
	invitation.TenantName = t.Name
	invitation.InviterName = inviterName
	if s.emailSender != nil {
		inviteURL := maillink.InvitationAccept(s.frontendURL, invitation.Token)
		if err := s.emailSender.SendInvitationEmail(req.Email, inviterName, t.Name, req.Message, inviteURL); err != nil {
			s.logger.Error("Failed to send invitation email", zap.Error(err), zap.String("email", req.Email))
		}
	}
	s.logger.Info("Invitation created", zap.String("invitation_id", invitation.ID.String()), zap.String("email", req.Email), zap.String("tenant_id", tenantID.String()))
	return invitation, nil
}

// displayName prefers the user's name and falls back to their email, which is
// the only field guaranteed to be present.
func displayName(u *user.User) string {
	if u.Name != nil && *u.Name != "" {
		return *u.Name
	}
	return u.Email
}

// hydrate fills the derived TenantName/InviterName fields, which are not
// columns. Lookup failures are non-fatal: a missing tenant or a deleted inviter
// must not turn a readable invitation into an error, so the field is simply
// left at its zero value (inviter falls back to "system", matching a NULL
// inviter_id).
func (s *service) hydrate(invs ...*Invitation) {
	tenantNames := map[uuid.UUID]string{}
	inviterNames := map[uuid.UUID]string{}
	for _, inv := range invs {
		if _, ok := tenantNames[inv.TenantID]; !ok {
			if t, err := s.tenantService.GetTenantByID(inv.TenantID); err == nil {
				tenantNames[inv.TenantID] = t.Name
			} else {
				tenantNames[inv.TenantID] = ""
			}
		}
		inv.TenantName = tenantNames[inv.TenantID]

		inv.InviterName = SystemInviterName
		if inv.InviterID == nil {
			continue
		}
		if _, ok := inviterNames[*inv.InviterID]; !ok {
			if u, err := s.userService.GetByID(*inv.InviterID); err == nil {
				inviterNames[*inv.InviterID] = displayName(u)
			} else {
				inviterNames[*inv.InviterID] = SystemInviterName
			}
		}
		inv.InviterName = inviterNames[*inv.InviterID]
	}
}

// HasValidInvitation delegates to Gate so the policy has exactly one
// implementation, whichever way a caller reaches it.
func (s *service) HasValidInvitation(tenantID uuid.UUID, email string) (bool, error) {
	return NewGate(s.db).HasValidInvitation(tenantID, email)
}

func (s *service) GetByToken(token string) (*Invitation, error) {
	var inv Invitation
	if err := s.db.Where("token = ?", token).First(&inv).Error; err != nil {
		return nil, apierror.NewPublic("invitation not found")
	}
	s.hydrate(&inv)
	return &inv, nil
}

func (s *service) GetByID(id uuid.UUID) (*Invitation, error) {
	var inv Invitation
	if err := s.db.Where("id = ?", id).First(&inv).Error; err != nil {
		return nil, apierror.NewPublic("invitation not found")
	}
	s.hydrate(&inv)
	return &inv, nil
}

func (s *service) ListByTenant(tenantID uuid.UUID) ([]Invitation, error) {
	var invitations []Invitation
	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&invitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	s.hydrateAll(invitations)
	return invitations, nil
}

func (s *service) ListPendingByEmail(email string) ([]Invitation, error) {
	var invitations []Invitation
	if err := s.db.Where("email = ? AND status = ? AND expires_at > ?", email, StatusPending, time.Now()).Find(&invitations).Error; err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	s.hydrateAll(invitations)
	return invitations, nil
}

// hydrateAll hydrates a slice in place (the elements, not copies).
func (s *service) hydrateAll(invs []Invitation) {
	ptrs := make([]*Invitation, len(invs))
	for i := range invs {
		ptrs[i] = &invs[i]
	}
	s.hydrate(ptrs...)
}

func (s *service) Accept(token string, userID *uuid.UUID, name, password string) (*user.User, error) {
	inv, err := s.GetByToken(token)
	if err != nil {
		return nil, err
	}
	if !inv.CanBeAccepted() {
		if inv.IsExpired() {
			return nil, apierror.NewPublic("invitation has expired")
		}
		return nil, apierror.NewPublic(fmt.Sprintf("invitation cannot be accepted (status: %s)", inv.Status))
	}
	var u *user.User
	if userID != nil {
		u, err = s.userService.GetByID(*userID)
		if err != nil {
			return nil, apierror.NewPublic("user not found")
		}
		if u.TenantID != inv.TenantID {
			return nil, apierror.NewPublic("user belongs to a different organization")
		}
	} else {
		existingUser, _ := s.userService.GetByEmailAndTenant(inv.TenantID, inv.Email)
		if existingUser != nil {
			return nil, apierror.NewPublic("user already exists, please log in to accept")
		}
		if password == "" {
			return nil, apierror.NewPublic("password is required for new users")
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
		return apierror.NewPublic(fmt.Sprintf("invitation cannot be declined (status: %s)", inv.Status))
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
		return apierror.NewPublic("only pending invitations can be revoked")
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
		return apierror.NewPublic("only pending invitations can be resent")
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
		inviteURL := maillink.InvitationAccept(s.frontendURL, inv.Token)
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
