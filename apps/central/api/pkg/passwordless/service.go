package passwordless

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"strings"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/maillink"
	"authway/apps/central/api/pkg/tokenhash"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EmailSender interface for sending emails
type EmailSender interface {
	// code is the link's code as the reader types it (XXXX-XXXX).
	SendMagicLinkEmail(toEmail, organization, linkURL, code string, isNewUser bool) error
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
	// SendMagicLink emails a link that signs email in to loginFlow. An
	// address that may not sign in gets the same answer and no link.
	SendMagicLink(tenantID uuid.UUID, email, loginFlow, ipAddress, userAgent string) (*MagicLinkResponse, error)
	// VerifyMagicLink consumes the token: it marks the link used and may
	// provision a user. It is not idempotent and must never back a GET.
	VerifyMagicLink(token string) (*MagicLink, *user.User, error)
	// VerifyMagicLinkCode does what VerifyMagicLink does, for the code the
	// email carries instead of the link — typed into the screen that started
	// loginFlow. Wrong codes are counted; after maxCodeAttempts the codes of
	// that sign-in's links stop working.
	VerifyMagicLinkCode(loginFlow, code string) (*MagicLink, *user.User, error)
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
	codes       crypto.Cipher
	tokenExpiry time.Duration
	// organization names a tenant in the mail ("" when it cannot).
	organization func(tenantID uuid.UUID) string
}

// NewService wires the passwordless flow. invitations must not be nil — a nil
// gate is treated as "provisioning denied" rather than "no policy", so a wiring
// mistake fails closed instead of silently restoring self-registration. codes
// seals each link's code at rest (the at-rest cipher).
func NewService(db *gorm.DB, userService user.Service, invitations InvitationGate, emailSender EmailSender, logger *zap.Logger, frontendURL string, codes crypto.Cipher, organization func(tenantID uuid.UUID) string) Service {
	return &service{
		db:          db,
		userService: userService,
		invitations: invitations,
		emailSender: emailSender,
		logger:      logger,
		frontendURL: frontendURL,
		codes:       codes,
		tokenExpiry: 15 * time.Minute,

		organization: organization,
	}
}

func generateToken() (string, error) {
	return tokenhash.Generate()
}

// A link's code: eight letters from an alphabet without vowels (no words, and
// nothing that reads as another letter or a digit) — the shape RFC 8628 §6.1
// recommends for codes people type, and the one the device sign-in uses.
const (
	codeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"
	codeLength   = 8
	// maxCodeAttempts wrong codes end the codes of a sign-in's links.
	maxCodeAttempts = 5
	// At most maxLinksPerMailbox links go to one address in mailboxWindow.
	maxLinksPerMailbox = 5
	mailboxWindow      = 15 * time.Minute
)

func generateCode() (string, error) {
	b := make([]byte, codeLength)
	limit := big.NewInt(int64(len(codeAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// normalizeCode reads a code as typed: any case, with or without the hyphen
// or spaces.
func normalizeCode(typed string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		if r != '-' && r != ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// displayCode is how the email shows a code: XXXX-XXXX.
func displayCode(code string) string {
	return code[:4] + "-" + code[4:]
}

var errInvalidCode = apierror.NewPublic("this code is not valid; check it, or ask for a new email")

// mayProvision reports whether a user may be created for this address. It fails
// closed on a missing gate or a lookup error: the safe answer to "is this
// address allowed in?" is no.
// organizationOf is the tenant's name for a mail, or "" without one.
func (s *service) organizationOf(tenantID uuid.UUID) string {
	if s.organization == nil {
		return ""
	}
	return s.organization(tenantID)
}

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

func (s *service) SendMagicLink(tenantID uuid.UUID, email, loginFlow, ipAddress, userAgent string) (*MagicLinkResponse, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	code, err := generateCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code: %w", err)
	}
	sealedCode, err := s.codes.Encrypt(code)
	if err != nil {
		return nil, fmt.Errorf("failed to seal code: %w", err)
	}
	expiresAt := time.Now().Add(s.tokenExpiry)
	tokenType := TokenTypeLogin
	if existing, err := s.userService.GetByEmailAndTenant(tenantID, email); err == nil && !existing.Active {
		// A suspended account could not sign in with the link; it is sent
		// nothing, and the answer is the one every address gets.
		s.logger.Warn("Magic link suppressed for a suspended account",
			zap.String("user_id", existing.ID.String()), zap.String("tenant_id", tenantID.String()))
		return &MagicLinkResponse{
			Message:   "Magic link sent to your email",
			ExpiresAt: expiresAt,
		}, nil
	} else if err != nil {
		// No user yet — this link would provision one, which the
		// invitation-only policy allows solely for an invited address.
		if !s.mayProvision(tenantID, email) {
			// Deliberately indistinguishable from success: a differing response
			// would turn this public endpoint into a membership oracle. No link
			// is created, so there is nothing to verify later.
			s.logger.Warn("Magic link suppressed for uninvited address",
				zap.String("email", email), zap.String("tenant_id", tenantID.String()))
			return &MagicLinkResponse{
				Message:   "Magic link sent to your email",
				ExpiresAt: expiresAt,
			}, nil
		}
		tokenType = TokenTypeRegister
	}
	// A mailbox gets a few links at a time, however many network addresses ask
	// for them. Past that no link is made and the answer stays the same — a
	// refusal would tell that the address can sign in, since an address that
	// cannot never has links to count.
	var recent int64
	if err := s.db.Model(&MagicLink{}).
		Where("tenant_id = ? AND lower(email) = lower(?) AND created_at > ?", tenantID, email, time.Now().Add(-mailboxWindow)).
		Count(&recent).Error; err != nil {
		return nil, fmt.Errorf("failed to count recent links: %w", err)
	}
	if recent >= maxLinksPerMailbox {
		s.logger.Warn("Magic link suppressed: too many for this address",
			zap.String("email", email), zap.String("tenant_id", tenantID.String()))
		return &MagicLinkResponse{
			Message:   "Magic link sent to your email",
			ExpiresAt: expiresAt,
		}, nil
	}
	magicLink := &MagicLink{
		TenantID:   tenantID,
		Email:      email,
		TokenHash:  tokenhash.Hash(token),
		TokenType:  tokenType,
		LoginFlow:  loginFlow,
		CodeSealed: sealedCode,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		ExpiresAt:  expiresAt,
	}
	if err := s.db.Create(magicLink).Error; err != nil {
		return nil, fmt.Errorf("failed to create magic link: %w", err)
	}
	// frontendURL is the auth UI, not the API — this link is opened by a
	// human, and the page redeems the token only when they confirm.
	linkURL := maillink.MagicLink(s.frontendURL, token, displayCode(code))
	if s.emailSender != nil {
		isNewUser := tokenType == TokenTypeRegister
		// A link that could not be handed to the sender gets the same answer
		// as one that was: the answer must not depend on the address.
		if err := s.emailSender.SendMagicLinkEmail(email, s.organizationOf(tenantID), linkURL, displayCode(code), isNewUser); err != nil {
			s.logger.Error("Failed to send magic link email", zap.Error(err), zap.String("email", email))
		}
	}
	s.logger.Info("Magic link sent", zap.String("email", email), zap.String("token_type", string(tokenType)), zap.String("tenant_id", tenantID.String()))
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
	return s.signIn(&magicLink)
}

func (s *service) VerifyMagicLinkCode(loginFlow, code string) (*MagicLink, *user.User, error) {
	typed := normalizeCode(code)
	now := time.Now()
	live := func() *gorm.DB {
		return s.db.Model(&MagicLink{}).
			Where("login_flow = ? AND used_at IS NULL AND expires_at > ? AND code_sealed <> ''", loginFlow, now)
	}

	if len(typed) == codeLength {
		var candidates []MagicLink
		if err := live().Where("code_attempts < ?", maxCodeAttempts).Find(&candidates).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to look up the sign-in's links: %w", err)
		}
		for i := range candidates {
			stored, err := s.codes.Decrypt(candidates[i].CodeSealed)
			if err != nil || subtle.ConstantTimeCompare([]byte(stored), []byte(typed)) != 1 {
				continue
			}
			// Claimed by a single conditional UPDATE, like the link: of two
			// requests with the right code exactly one signs in.
			claim := s.db.Model(&MagicLink{}).
				Where("id = ? AND used_at IS NULL AND expires_at > ? AND code_attempts < ?", candidates[i].ID, now, maxCodeAttempts).
				Update("used_at", now)
			if claim.Error != nil {
				return nil, nil, fmt.Errorf("failed to mark the link as used: %w", claim.Error)
			}
			if claim.RowsAffected == 1 {
				return s.signIn(&candidates[i])
			}
			break
		}
	}

	if err := live().Update("code_attempts", gorm.Expr("code_attempts + 1")).Error; err != nil {
		s.logger.Error("Failed to count a wrong sign-in code", zap.Error(err))
	}
	return nil, nil, errInvalidCode
}

// signIn finds — or, for an invited address, creates — the user a redeemed
// link signs in.
func (s *service) signIn(magicLink *MagicLink) (*MagicLink, *user.User, error) {
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
		s.markEmailVerified(u)
		s.logger.Info("Created user via magic link", zap.String("user_id", u.ID.String()), zap.String("email", u.Email))
	} else {
		if !u.EmailVerified {
			s.markEmailVerified(u)
		}
		s.logger.Info("User authenticated via magic link", zap.String("user_id", u.ID.String()), zap.String("email", u.Email))
	}
	return magicLink, u, nil
}

// markEmailVerified records that the user proved control of the address by
// following the link. A failure is logged, not fatal: the sign-in itself is
// valid either way.
func (s *service) markEmailVerified(u *user.User) {
	if err := s.userService.UpdateEmailVerified(u.ID, true); err != nil {
		s.logger.Error("Failed to mark email verified after magic link", zap.String("user_id", u.ID.String()), zap.Error(err))
		return
	}
	u.EmailVerified = true
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
