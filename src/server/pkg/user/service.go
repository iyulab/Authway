package user

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service interface {
	Create(tenantID uuid.UUID, req *CreateUserRequest) (*User, error)
	GetByID(id uuid.UUID) (*User, error)
	GetByEmail(email string) (*User, error) // Deprecated: Use GetByEmailAndTenant
	GetByEmailAndTenant(tenantID uuid.UUID, email string) (*User, error)
	GetByTenant(tenantID uuid.UUID, limit, offset int) ([]*User, int64, error)
	Update(id uuid.UUID, req *UpdateUserRequest) (*User, error)
	Delete(id uuid.UUID) error
	List(limit, offset int) ([]*User, int64, error)
	VerifyPassword(user *User, password string) bool
	ChangePassword(userID uuid.UUID, req *ChangePasswordRequest) error
	UpdateLastLogin(userID uuid.UUID) error
	UpdateEmailVerified(userID uuid.UUID, verified bool) error
	UpdatePassword(userID uuid.UUID, newPassword string) error
}

type service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB, logger *zap.Logger) Service {
	return &service{
		db:     db,
		logger: logger,
	}
}

func (s *service) Create(tenantID uuid.UUID, req *CreateUserRequest) (*User, error) {
	// Check if user already exists in this tenant
	var existingUser User
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&existingUser).Error; err == nil {
		return nil, fmt.Errorf("user with email %s already exists in this tenant", req.Email)
	}

	user := &User{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Email:         req.Email,
		Name:          &req.Name,
		EmailVerified: false,
		Active:        true,
	}

	// Hash password if provided (not required for social login)
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = string(hashedPassword)
	}

	if err := s.db.Create(user).Error; err != nil {
		s.logger.Error("Failed to create user", zap.Error(err), zap.String("email", req.Email), zap.String("tenant_id", tenantID.String()))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("User created successfully", zap.String("id", user.ID.String()), zap.String("email", user.Email), zap.String("tenant_id", tenantID.String()))
	return user, nil
}

func (s *service) GetByID(id uuid.UUID) (*User, error) {
	var user User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

func (s *service) GetByEmail(email string) (*User, error) {
	var user User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

func (s *service) Update(id uuid.UUID, req *UpdateUserRequest) (*User, error) {
	var user User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Update fields
	if req.Name != "" {
		user.Name = &req.Name
	}
	if req.AvatarURL != "" {
		user.AvatarURL = &req.AvatarURL
	}

	if err := s.db.Save(&user).Error; err != nil {
		s.logger.Error("Failed to update user", zap.Error(err), zap.String("id", id.String()))
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.logger.Info("User updated successfully", zap.String("id", user.ID.String()))
	return &user, nil
}

func (s *service) Delete(id uuid.UUID) error {
	result := s.db.Delete(&User{}, id)
	if result.Error != nil {
		s.logger.Error("Failed to delete user", zap.Error(result.Error), zap.String("id", id.String()))
		return fmt.Errorf("failed to delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
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
	if err := s.db.Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
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

	// Verify current password
	if !s.VerifyPassword(user, req.CurrentPassword) {
		return fmt.Errorf("current password is incorrect")
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
	if err := s.db.Where("tenant_id = ? AND email = ?", tenantID, email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
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
	if err := s.db.Where("tenant_id = ?", tenantID).Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	return users, total, nil
}
