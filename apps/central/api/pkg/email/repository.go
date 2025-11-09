package email

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository handles email verification and password reset database operations
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new email repository
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// === Email Verification Methods ===

// CreateVerification creates a new email verification record
func (r *Repository) CreateVerification(userID uuid.UUID) (*EmailVerification, error) {
	verification := &EmailVerification{
		UserID: userID,
	}

	if err := r.db.Create(verification).Error; err != nil {
		return nil, fmt.Errorf("failed to create email verification: %w", err)
	}

	return verification, nil
}

// GetVerificationByToken retrieves a verification by token
func (r *Repository) GetVerificationByToken(token string) (*EmailVerification, error) {
	var verification EmailVerification
	if err := r.db.Where("token = ? AND verified = ? AND deleted_at IS NULL", token, false).First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("verification token not found")
		}
		return nil, fmt.Errorf("failed to get verification: %w", err)
	}

	return &verification, nil
}

// MarkVerificationAsVerified marks a verification as verified
func (r *Repository) MarkVerificationAsVerified(id uuid.UUID) error {
	if err := r.db.Model(&EmailVerification{}).Where("id = ?", id).Update("verified", true).Error; err != nil {
		return fmt.Errorf("failed to mark verification as verified: %w", err)
	}
	return nil
}

// DeleteVerificationsByUserID deletes all verifications for a user
func (r *Repository) DeleteVerificationsByUserID(userID uuid.UUID) error {
	if err := r.db.Where("user_id = ?", userID).Delete(&EmailVerification{}).Error; err != nil {
		return fmt.Errorf("failed to delete verifications: %w", err)
	}
	return nil
}

// GetPendingVerificationByUserID gets pending verification for a user
func (r *Repository) GetPendingVerificationByUserID(userID uuid.UUID) (*EmailVerification, error) {
	var verification EmailVerification
	if err := r.db.Where("user_id = ? AND verified = ? AND deleted_at IS NULL", userID, false).
		Order("created_at DESC").
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get pending verification: %w", err)
	}

	return &verification, nil
}

// === Password Reset Methods ===

// CreatePasswordReset creates a new password reset record
func (r *Repository) CreatePasswordReset(userID uuid.UUID) (*PasswordReset, error) {
	// First, invalidate any existing unused reset tokens for this user
	if err := r.db.Model(&PasswordReset{}).
		Where("user_id = ? AND used = ?", userID, false).
		Update("used", true).Error; err != nil {
		return nil, fmt.Errorf("failed to invalidate old reset tokens: %w", err)
	}

	// Create new reset token
	reset := &PasswordReset{
		UserID: userID,
	}

	if err := r.db.Create(reset).Error; err != nil {
		return nil, fmt.Errorf("failed to create password reset: %w", err)
	}

	return reset, nil
}

// GetPasswordResetByToken retrieves a password reset by token
func (r *Repository) GetPasswordResetByToken(token string) (*PasswordReset, error) {
	var reset PasswordReset
	if err := r.db.Where("token = ? AND used = ? AND deleted_at IS NULL", token, false).First(&reset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("reset token not found or already used")
		}
		return nil, fmt.Errorf("failed to get password reset: %w", err)
	}

	return &reset, nil
}

// MarkPasswordResetAsUsed marks a password reset as used
func (r *Repository) MarkPasswordResetAsUsed(id uuid.UUID) error {
	reset := &PasswordReset{}
	if err := r.db.First(reset, id).Error; err != nil {
		return fmt.Errorf("failed to find password reset: %w", err)
	}

	reset.MarkAsUsed()
	if err := r.db.Save(reset).Error; err != nil {
		return fmt.Errorf("failed to mark reset as used: %w", err)
	}

	return nil
}

// DeletePasswordResetsByUserID deletes all password resets for a user
func (r *Repository) DeletePasswordResetsByUserID(userID uuid.UUID) error {
	if err := r.db.Where("user_id = ?", userID).Delete(&PasswordReset{}).Error; err != nil {
		return fmt.Errorf("failed to delete password resets: %w", err)
	}
	return nil
}

// CleanupExpiredTokens removes expired tokens (can be run periodically)
func (r *Repository) CleanupExpiredTokens() error {
	// Delete expired email verifications
	if err := r.db.Unscoped().Where("expires_at < NOW() AND deleted_at IS NULL").Delete(&EmailVerification{}).Error; err != nil {
		return fmt.Errorf("failed to cleanup expired verifications: %w", err)
	}

	// Delete expired password resets
	if err := r.db.Unscoped().Where("expires_at < NOW() AND deleted_at IS NULL").Delete(&PasswordReset{}).Error; err != nil {
		return fmt.Errorf("failed to cleanup expired resets: %w", err)
	}

	return nil
}
