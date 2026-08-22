package passwordless

import (
	"authway/apps/central/api/pkg/apierror"
	"fmt"
	"net/url"
	"time"

	"authway/apps/central/api/pkg/maillink"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EmailSender interface for sending emails
type EmailSender interface {
	SendMagicLinkEmail(toEmail, linkURL string, isNewUser bool) error
}

// InvitationGate reports whether a magic-link sign-in may create a new
// account for (tenantID, email) — invited, or the tenant allows open signup.
//
// Onboarding is invitation-only by default (decision D-a/B), but this
// endpoint is public and unauthenticated: without a gate, anyone who knows a
// tenant id could send themselves a magic link and be provisioned into that
// tenant on verify — i.e. public self-registration under another name. A
// tenant may opt into open signup (tenant.SignupModeOpen); every other
// tenant keeps the invite-only default, and because invitations are keyed on
// (tenant_id, email), that default also closes the arbitrary-tenant_id hole:
// an attacker-chosen tenant has no matching invitation.
type InvitationGate interface {
	MayProvision(tenantID uuid.UUID, email string) (bool, error)
}

// Service provides passwordless authentication functionality
type Service interface {
	SendMagicLink(tenantID uuid.UUID, req *SendMagicLinkRequest, ipAddress, userAgent string) (*MagicLinkResponse, error)
	// VerifyMagicLink consumes the token: it marks the link used and may
	// provision a user. It is not idempotent and must never back a GET.
	VerifyMagicLink(token string) (*MagicLink, *user.User, error)
	// InspectMagicLink reports whether a token is currently redeemable without
	// consuming it or touching any user state.
	InspectMagicLink(token string) (*MagicLink, error)
	CleanupExpired() (int64, error)
}

type service struct {
	db          *gorm.DB
	userService user.Service
	invitations InvitationGate
	emailSender EmailSender
	logger      *zap.Logger
	frontendURL string
	tokenExpiry time.Duration
}

// NewService wires the passwordless flow. invitations must not be nil — a nil
// gate is treated as "provisioning denied" rather than "no policy", so a wiring
// mistake fails closed instead of silently restoring self-registration.
func NewService(db *gorm.DB, userService user.Service, invitations InvitationGate, emailSender EmailSender, logger *zap.Logger, frontendURL string) Service {
	return &service{
		db:          db,
		userService: userService,
		invitations: invitations,
		emailSender: emailSender,
		logger:      logger,
		frontendURL: frontendURL,
		tokenExpiry: 15 * time.Minute,
	}
}

func generateToken() (string, error) {
	return tokenhash.Generate()
}

// mayProvision reports whether a user may be created for this address. It fails
// closed on a missing gate or a lookup error: the safe answer to "is this
// address allowed in?" is no.
func (s *service) mayProvision(tenantID uuid.UUID, email string) bool {
	if s.invitations == nil {
		s.logger.Error("Invitation gate not wired; denying magic-link provisioning")
		return false
	}
	allowed, err := s.invitations.MayProvision(tenantID, email)
	if err != nil {
		s.logger.Error("Provisioning check failed; denying provisioning", zap.Error(err))
		return false
	}
	return allowed
}

func (s *service) SendMagicLink(tenantID uuid.UUID, req *SendMagicLinkRequest, ipAddress, userAgent string) (*MagicLinkResponse, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	expiresAt := time.Now().Add(s.tokenExpiry)
	tokenType := TokenTypeLogin
	if _, err := s.userService.GetByEmailAndTenant(tenantID, req.Email); err != nil {
		// No user yet — this link would provision one, which the
		// invitation-only policy allows solely for an invited address.
		if !s.mayProvision(tenantID, req.Email) {
			// Deliberately indistinguishable from success: a differing response
			// would turn this public endpoint into a membership oracle. No link
			// is created, so there is nothing to verify later.
			s.logger.Warn("Magic link suppressed for uninvited address",
				zap.String("email", req.Email), zap.String("tenant_id", tenantID.String()))
			return &MagicLinkResponse{
				Message:   "Magic link sent to your email",
				ExpiresAt: expiresAt,
			}, nil
		}
		tokenType = TokenTypeRegister
	}
	magicLink := &MagicLink{
		TenantID:    tenantID,
		Email:       req.Email,
		TokenHash:   tokenhash.Hash(token),
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
	// frontendURL is the auth UI, not the API — this link is opened by a human, and
	// the page then POSTs the token to /auth/magic-link/verify. It previously
	// pointed at /auth/magic-link/verify on the UI, a route that does not exist
	// there (the page is mounted at /magic-link), so every emailed link 404'd.
	linkURL := maillink.MagicLink(s.frontendURL, token)
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

func (s *service) InspectMagicLink(token string) (*MagicLink, error) {
	var magicLink MagicLink
	if err := s.db.Where("token_hash = ?", tokenhash.Hash(token)).First(&magicLink).Error; err != nil {
		return nil, apierror.NewPublic("invalid or expired token")
	}
	if magicLink.IsExpired() {
		return nil, apierror.NewPublic("magic link has expired")
	}
	if magicLink.IsUsed() {
		return nil, apierror.NewPublic("magic link has already been used")
	}
	return &magicLink, nil
}

func (s *service) VerifyMagicLink(token string) (*MagicLink, *user.User, error) {
	// Claim the token with a single conditional UPDATE. Read-then-write let two
	// concurrent requests both pass the "not used" check and both redeem the
	// same link; here exactly one UPDATE can match, and the loser sees the same
	// error as any stale token.
	now := time.Now()
	claim := s.db.Model(&MagicLink{}).
		Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenhash.Hash(token), now).
		Update("used_at", now)
	if claim.Error != nil {
		return nil, nil, fmt.Errorf("failed to mark token as used: %w", claim.Error)
	}
	if claim.RowsAffected == 0 {
		return nil, nil, apierror.NewPublic("invalid, expired or already used token")
	}

	var magicLink MagicLink
	if err := s.db.Where("token_hash = ?", tokenhash.Hash(token)).First(&magicLink).Error; err != nil {
		return nil, nil, apierror.NewPublic("invalid or expired token")
	}
	var u *user.User
	var err error
	u, err = s.userService.GetByEmailAndTenant(magicLink.TenantID, magicLink.Email)
	if err != nil {
		// Re-check at the moment of creation, not just at send time: the
		// invitation may have been revoked or expired in between, and the link
		// itself is not proof of eligibility.
		if !s.mayProvision(magicLink.TenantID, magicLink.Email) {
			return nil, nil, apierror.NewPublic("no account for this address; ask an administrator for an invitation")
		}
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
