package claims

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository defines the interface for claims data access
type Repository interface {
	// Database operations
	SaveClaim(ctx context.Context, claim *UserClaim) error
	GetUserClaims(ctx context.Context, userID, tenantID uuid.UUID) ([]UserClaim, error)
	// ApplyPatch stores set and deletes remove, in one transaction.
	ApplyPatch(ctx context.Context, userID, tenantID uuid.UUID, set ClaimMap, remove []string) error

	// Redis operations for pending claims
	SetPendingClaims(ctx context.Context, userID uuid.UUID, claims ClaimMap, ttl time.Duration) error
	GetPendingClaims(ctx context.Context, userID uuid.UUID) (ClaimMap, error)
	DeletePendingClaims(ctx context.Context, userID uuid.UUID) error

	// Redis operations for login claims
	SetLoginClaims(ctx context.Context, loginChallenge string, claims ClaimMap, ttl time.Duration) error
	GetLoginClaims(ctx context.Context, loginChallenge string) (ClaimMap, error)
	DeleteLoginClaims(ctx context.Context, loginChallenge string) error
}

type repository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewRepository creates a new claims repository
func NewRepository(db *gorm.DB, redis *redis.Client) Repository {
	return &repository{
		db:    db,
		redis: redis,
	}
}

// SaveClaim saves or updates a claim in the database
func (r *repository) SaveClaim(ctx context.Context, claim *UserClaim) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "tenant_id"},
				{Name: "claim_key"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"claim_value", "claim_type", "is_permanent", "updated_at"}),
		}).
		Create(claim).Error
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
				ClaimValue: map[string]any{"value": value}, IsPermanent: true,
				CreatedAt: now, UpdatedAt: now,
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

// SetPendingClaims stores pending claims in Redis for a user session
func (r *repository) SetPendingClaims(ctx context.Context, userID uuid.UUID, claims ClaimMap, ttl time.Duration) error {
	key := fmt.Sprintf("session:%s:pending_claims", userID.String())
	data, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("failed to marshal pending claims: %w", err)
	}

	err = r.redis.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set pending claims in redis: %w", err)
	}

	return nil
}

// GetPendingClaims retrieves pending claims from Redis for a user session
func (r *repository) GetPendingClaims(ctx context.Context, userID uuid.UUID) (ClaimMap, error) {
	key := fmt.Sprintf("session:%s:pending_claims", userID.String())
	data, err := r.redis.Get(ctx, key).Result()

	if err == redis.Nil {
		// No pending claims found
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get pending claims from redis: %w", err)
	}

	var claims ClaimMap
	err = json.Unmarshal([]byte(data), &claims)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pending claims: %w", err)
	}

	return claims, nil
}

// DeletePendingClaims removes pending claims from Redis
func (r *repository) DeletePendingClaims(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("session:%s:pending_claims", userID.String())
	err := r.redis.Del(ctx, key).Err()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to delete pending claims from redis: %w", err)
	}
	return nil
}

// SetLoginClaims stores claims associated with a login challenge in Redis
func (r *repository) SetLoginClaims(ctx context.Context, loginChallenge string, claims ClaimMap, ttl time.Duration) error {
	key := fmt.Sprintf("login:%s:claims", loginChallenge)
	data, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("failed to marshal login claims: %w", err)
	}

	err = r.redis.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set login claims in redis: %w", err)
	}

	return nil
}

// GetLoginClaims retrieves claims associated with a login challenge from Redis
func (r *repository) GetLoginClaims(ctx context.Context, loginChallenge string) (ClaimMap, error) {
	key := fmt.Sprintf("login:%s:claims", loginChallenge)
	data, err := r.redis.Get(ctx, key).Result()

	if err == redis.Nil {
		// No login claims found
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get login claims from redis: %w", err)
	}

	var claims ClaimMap
	err = json.Unmarshal([]byte(data), &claims)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal login claims: %w", err)
	}

	return claims, nil
}

// DeleteLoginClaims removes claims associated with a login challenge from Redis
func (r *repository) DeleteLoginClaims(ctx context.Context, loginChallenge string) error {
	key := fmt.Sprintf("login:%s:claims", loginChallenge)
	err := r.redis.Del(ctx, key).Err()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to delete login claims from redis: %w", err)
	}
	return nil
}
