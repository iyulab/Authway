package claims

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository stores the claims kept for users.
type Repository interface {
	GetUserClaims(ctx context.Context, userID, tenantID uuid.UUID) ([]UserClaim, error)
	// ApplyPatch stores set and deletes remove, in one transaction.
	ApplyPatch(ctx context.Context, userID, tenantID uuid.UUID, set ClaimMap, remove []string) error
}

type repository struct {
	db *gorm.DB
}

// NewRepository creates a new claims repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// GetUserClaims retrieves all claims for a user in a tenant
func (r *repository) GetUserClaims(ctx context.Context, userID, tenantID uuid.UUID) ([]UserClaim, error) {
	var claims []UserClaim
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Find(&claims).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get user claims: %w", err)
	}

	return claims, nil
}

// ApplyPatch stores set and deletes remove for a user, in one transaction.
func (r *repository) ApplyPatch(ctx context.Context, userID, tenantID uuid.UUID, set ClaimMap, remove []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(remove) > 0 {
			if err := tx.Where("user_id = ? AND tenant_id = ? AND claim_key IN ?", userID, tenantID, remove).
				Delete(&UserClaim{}).Error; err != nil {
				return fmt.Errorf("failed to delete claims: %w", err)
			}
		}
		now := time.Now()
		for key, value := range set {
			claim := &UserClaim{
				ID: uuid.New(), UserID: userID, TenantID: tenantID, ClaimKey: key,
				ClaimValue: map[string]any{"value": value},
				CreatedAt:  now, UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "tenant_id"}, {Name: "claim_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"claim_value", "updated_at"}),
			}).Create(claim).Error; err != nil {
				return fmt.Errorf("failed to save claim: %w", err)
			}
		}
		return nil
	})
}
