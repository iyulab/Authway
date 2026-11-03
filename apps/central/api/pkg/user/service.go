package user

import (
	"errors"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/emailaddr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service interface {
	Create(tenantID uuid.UUID, req *CreateUserRequest) (*User, error)
	GetByID(id uuid.UUID) (*User, error)
	// GetSoleAccountByEmail returns the account with this address when there
	// is exactly one, across every tenant. The same address may have an
	// account in more than one tenant (uq_users_tenant_lower_email allows it);
	// then there is no telling which one is meant, and it answers not found.
	// For a request with no tenant to scope by — everywhere else, use
	// GetByEmailAndTenant.
	GetSoleAccountByEmail(email string) (*User, error)
	GetByEmailAndTenant(tenantID uuid.UUID, email string) (*User, error)
	// FindByEmail lists the tenant's accounts with this address, compared
	// without regard to case — what someone answering a user looks up by.
	// Usually one or none; addresses differing only in case are separate
	// accounts, and all of them are returned.
	FindByEmail(tenantID uuid.UUID, email string) ([]*User, error)
	// GetByProviderSubject finds the account in a tenant that signs in with a
	// provider's account (subject is the provider's own identifier for it).
	// ErrUserNotFound when no account in the tenant is linked to it.
	GetByProviderSubject(tenantID uuid.UUID, provider, subject string) (*User, error)
	// LinkProvider records that the account signs in with a provider's
	// account. An account may sign in with several, at one provider or at
	// several. ErrProviderSubjectTaken when another account in the tenant
	// already signs in with it; linking it to the same account again changes
	// nothing.
	LinkProvider(userID uuid.UUID, link ProviderLink) error
	// UnlinkProvider ends the account's link to the one provider account it
	// has at provider. ErrNotLinked when there is none; ErrSeveralLinked when
	// there are several (use UnlinkIdentity); ErrLastSignInMethod when the
	// account would be left with no way to sign in (no password, no other
	// link).
	UnlinkProvider(userID uuid.UUID, provider string) error
	// UnlinkIdentity ends the account's link to one provider account, named by
	// its identity id. ErrNotLinked when the account has no such identity;
	// ErrLastSignInMethod as for UnlinkProvider.
	UnlinkIdentity(userID, identityID uuid.UUID) error
	GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*User, int64, error)
	Update(id uuid.UUID, req *UpdateUserRequest) (*User, error)
	Delete(id uuid.UUID) error
	List(limit, offset int) ([]*User, int64, error)
	VerifyPassword(user *User, password string) bool
	ChangePassword(userID uuid.UUID, req *ChangePasswordRequest) error
	UpdateLastLogin(userID uuid.UUID) error
	UpdateEmailVerified(userID uuid.UUID, verified bool) error
	UpdatePassword(userID uuid.UUID, newPassword string) error
	// LeastPasswordLength is the least length of a password an account of
	// the tenant may be given — what a screen about to ask for one says.
	LeastPasswordLength(tenantID uuid.UUID) int
}

// ErrUserNotFound reports that no account matches a lookup. Its text is safe
// to show a caller.
var ErrUserNotFound = apierror.NewPublic("user not found")

// ErrProviderSubjectTaken refuses a link to a sign-in provider account that
// another account in the same tenant already signs in with.
var ErrProviderSubjectTaken = errors.New("another account in this tenant already signs in with this provider account")

// uniqueViolation is the SQLSTATE of a row refused by a unique index.
const uniqueViolation = "23505"

// providerColumns names the column holding each sign-in provider's
// identifier for the account a user signs in with.
var providerColumns = map[string]string{
	"google":    "google_id",
	"github":    "github_id",
	"microsoft": "microsoft_id",
	"apple":     "apple_id",
}

type service struct {
	db        *gorm.DB
	logger    *zap.Logger
	onCreated []func(*User)

	// passwordMinLength answers a tenant's least password length; see
	// PasswordMinLength.
	passwordMinLength func(tenantID uuid.UUID) int
}

// Option configures a user service.
type Option func(*service)

// OnCreated has fn called with every account the service creates. Accounts
// come into being on several paths — an accepted invitation, a first sign-in
// link, a first social sign-in — and all of them create the account here, so
// this is the one place that sees each of them.
func OnCreated(fn func(*User)) Option {
	return func(s *service) { s.onCreated = append(s.onCreated, fn) }
}

func NewService(db *gorm.DB, logger *zap.Logger, opts ...Option) Service {
	s := &service{
		db:     db,
		logger: logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *service) Create(tenantID uuid.UUID, req *CreateUserRequest) (*User, error) {
	email := emailaddr.Normalize(req.Email)

	// Check if user already exists in this tenant
	var existingUser User
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, email).First(&existingUser).Error; err == nil {
		return nil, fmt.Errorf("user with email %s already exists in this tenant", email)
	}

	user := &User{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Email:         email,
		Name:          &req.Name,
		EmailVerified: false,
		Active:        true,
	}

	// Hash password if provided (not required for social login)
	if req.Password != "" {
		if err := s.checkPassword(tenantID, req.Password); err != nil {
			return nil, err
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = string(hashedPassword)
	}

	if err := s.db.Create(user).Error; err != nil {
		s.logger.Error("Failed to create user", zap.Error(err), zap.String("email", email), zap.String("tenant_id", tenantID.String()))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("User created successfully", zap.String("id", user.ID.String()), zap.String("email", user.Email), zap.String("tenant_id", tenantID.String()))
	for _, fn := range s.onCreated {
		fn(user)
	}
	return user, nil
}

func (s *service) GetByID(id uuid.UUID) (*User, error) {
	var user User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apierror.NewPublic("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return s.loaded(&user)
}

func (s *service) GetSoleAccountByEmail(email string) (*User, error) {
	var users []User
	if err := s.db.Where("email = ?", emailaddr.Normalize(email)).Limit(2).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if len(users) != 1 {
		return nil, apierror.NewPublic("user not found")
	}
	return s.loaded(&users[0])
}

func (s *service) Update(id uuid.UUID, req *UpdateUserRequest) (*User, error) {
	var user User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apierror.NewPublic("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// A field left out stays; an empty one is cleared.
	clearable := func(v *string) *string {
		if *v == "" {
			return nil
		}
		return v
	}
	if req.Name != nil {
		user.Name = clearable(req.Name)
	}
	if req.AvatarURL != nil {
		user.AvatarURL = clearable(req.AvatarURL)
	}
	if req.Active != nil {
		user.Active = *req.Active
	}

	if err := s.db.Save(&user).Error; err != nil {
		s.logger.Error("Failed to update user", zap.Error(err), zap.String("id", id.String()))
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.logger.Info("User updated successfully", zap.String("id", user.ID.String()))
	return s.loaded(&user)
}

// Delete removes the account and, through the schema's cascades, what hangs
// on it (claims, terms acceptances, delegations, approvals, console
// memberships, unused email links); invitations keep their row with the
// account's id cleared. A deleted account is gone, not hidden: nothing of it
// stays to be restored or to answer for the person who asked to leave.
func (s *service) Delete(id uuid.UUID) error {
	result := s.db.Delete(&User{}, id)
	if result.Error != nil {
		s.logger.Error("Failed to delete user", zap.Error(result.Error), zap.String("id", id.String()))
		return fmt.Errorf("failed to delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return apierror.NewPublic("user not found")
	}

	s.logger.Info("User deleted successfully", zap.String("id", id.String()))
	return nil
}

func (s *service) List(limit, offset int) ([]*User, int64, error) {
	var users []*User
	var total int64

	// Get total count
	if err := s.db.Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Get users with pagination
	if err := s.db.Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	if err := s.withIdentities(users...); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (s *service) VerifyPassword(user *User, password string) bool {
	if user.PasswordHash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}

func (s *service) ChangePassword(userID uuid.UUID, req *ChangePasswordRequest) error {
	// Get user
	user, err := s.GetByID(userID)
	if err != nil {
		return err
	}

	if user.PasswordHash != "" && !s.VerifyPassword(user, req.CurrentPassword) {
		return ErrWrongPassword
	}

	if err := s.checkPassword(user.TenantID, req.NewPassword); err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Update password
	if err := s.db.Model(user).Update("password_hash", string(hashedPassword)).Error; err != nil {
		s.logger.Error("Failed to update password", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to update password: %w", err)
	}

	s.logger.Info("Password changed successfully", zap.String("user_id", userID.String()))
	return nil
}

func (s *service) UpdateLastLogin(userID uuid.UUID) error {
	now := time.Now()
	if err := s.db.Model(&User{}).Where("id = ?", userID).Update("last_login_at", now).Error; err != nil {
		s.logger.Error("Failed to update last login", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

// UpdateEmailVerified updates the email verification status
func (s *service) UpdateEmailVerified(userID uuid.UUID, verified bool) error {
	if err := s.db.Model(&User{}).Where("id = ?", userID).Update("email_verified", verified).Error; err != nil {
		s.logger.Error("Failed to update email verified status", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to update email verified status: %w", err)
	}
	s.logger.Info("Email verified status updated", zap.String("user_id", userID.String()), zap.Bool("verified", verified))
	return nil
}

// UpdatePassword updates user password (for password reset)
func (s *service) UpdatePassword(userID uuid.UUID, newPassword string) error {
	account, err := s.GetByID(userID)
	if err != nil {
		return err
	}
	if err := s.checkPassword(account.TenantID, newPassword); err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.db.Model(&User{}).Where("id = ?", userID).Update("password_hash", string(hashedPassword)).Error; err != nil {
		s.logger.Error("Failed to update password", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to update password: %w", err)
	}

	s.logger.Info("Password updated successfully", zap.String("user_id", userID.String()))
	return nil
}

// GetByEmailAndTenant retrieves a user by email and tenant ID
func (s *service) GetByEmailAndTenant(tenantID uuid.UUID, email string) (*User, error) {
	var user User
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, emailaddr.Normalize(email)).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return s.loaded(&user)
}

func (s *service) FindByEmail(tenantID uuid.UUID, email string) ([]*User, error) {
	var users []*User
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, emailaddr.Normalize(email)).
		Order("created_at").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to find users: %w", err)
	}
	if err := s.withIdentities(users...); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *service) GetByProviderSubject(tenantID uuid.UUID, provider, subject string) (*User, error) {
	if !knownProvider(provider) {
		return nil, fmt.Errorf("unknown sign-in provider %q", provider)
	}
	if subject == "" {
		return nil, ErrUserNotFound
	}
	var identity LinkedIdentity
	if err := s.db.Where("tenant_id = ? AND provider = ? AND subject = ?", tenantID, provider, subject).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	var user User
	if err := s.db.Where("id = ?", identity.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return s.loaded(&user)
}

func knownProvider(provider string) bool {
	_, ok := providerColumns[provider]
	return ok
}

func (s *service) LinkProvider(userID uuid.UUID, link ProviderLink) error {
	if !knownProvider(link.Provider) || link.Subject == "" {
		return fmt.Errorf("cannot link sign-in provider %q", link.Provider)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var u User
		if err := tx.Where("id = ?", userID).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var existing LinkedIdentity
		err := tx.Where("tenant_id = ? AND provider = ? AND subject = ?", u.TenantID, link.Provider, link.Subject).First(&existing).Error
		switch {
		case err == nil && existing.UserID != userID:
			return ErrProviderSubjectTaken
		case err == nil:
			// Already linked to this account.
		case errors.Is(err, gorm.ErrRecordNotFound):
			address := link.Email
			if address == "" {
				address = u.Email
			}
			if err := tx.Create(&LinkedIdentity{
				ID: uuid.New(), TenantID: u.TenantID, UserID: userID,
				Provider: link.Provider, Subject: link.Subject, EmailAtLink: address, LinkedAt: time.Now(),
			}).Error; err != nil {
				return err
			}
		default:
			return err
		}
		if link.VerifiesAddress {
			if err := tx.Model(&User{}).Where("id = ?", userID).Update("email_verified", true).Error; err != nil {
				return err
			}
		}
		return syncProviderColumn(tx, userID, link.Provider)
	})
	var refused *pgconn.PgError
	if errors.As(err, &refused) && refused.Code == uniqueViolation {
		return ErrProviderSubjectTaken
	}
	if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrProviderSubjectTaken) {
		return err
	}
	if err != nil {
		return fmt.Errorf("failed to link sign-in provider: %w", err)
	}
	s.logger.Info("Sign-in provider linked", zap.String("user_id", userID.String()), zap.String("provider", link.Provider))
	return nil
}

func (s *service) UnlinkProvider(userID uuid.UUID, provider string) error {
	u, err := s.GetByID(userID)
	if err != nil {
		return err
	}
	var at []LinkedIdentity
	for _, identity := range u.Identities {
		if identity.Provider == provider {
			at = append(at, identity)
		}
	}
	switch len(at) {
	case 0:
		return ErrNotLinked
	case 1:
		return s.unlink(u, at[0])
	default:
		return ErrSeveralLinked
	}
}

func (s *service) UnlinkIdentity(userID, identityID uuid.UUID) error {
	u, err := s.GetByID(userID)
	if err != nil {
		return err
	}
	for _, identity := range u.Identities {
		if identity.ID == identityID {
			return s.unlink(u, identity)
		}
	}
	return ErrNotLinked
}

// unlink removes one identity of u, which has just been loaded with all of its
// identities, unless it is the account's only way to sign in.
func (s *service) unlink(u *User, identity LinkedIdentity) error {
	if u.PasswordHash == "" && len(u.Identities) == 1 {
		return ErrLastSignInMethod
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", identity.ID, u.ID).Delete(&LinkedIdentity{}).Error; err != nil {
			return err
		}
		return syncProviderColumn(tx, u.ID, identity.Provider)
	})
	if err != nil {
		return fmt.Errorf("failed to unlink sign-in provider: %w", err)
	}
	s.logger.Info("Sign-in provider unlinked", zap.String("user_id", u.ID.String()), zap.String("provider", identity.Provider))
	return nil
}

// GetByTenant retrieves all users for a specific tenant with pagination
func (s *service) GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*User, int64, error) {
	var users []*User
	var total int64

	// Get total count for this tenant
	if err := s.db.Model(&User{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Get users with pagination
	if err := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC, id").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	if err := s.withIdentities(users...); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
