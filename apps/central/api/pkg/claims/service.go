package claims

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service defines the interface for claims business logic.
//
// The claims stored for a user go into the tokens issued to them, where a
// relying party may decide what the user is allowed to do from them. So only
// a trusted party writes them — an administrator, or a tenant's service
// credential — never the user.
type Service interface {
	// List returns the claims stored for a user, without any reserved claim.
	// They go into the user's tokens under the issuer's own (Issued).
	List(ctx context.Context, userID, tenantID uuid.UUID) (ClaimMap, error)
	// Patch applies a JSON merge patch to a user's stored claims — a value sets
	// the claim, nil removes it — and returns the claims stored afterwards.
	Patch(ctx context.Context, userID, tenantID uuid.UUID, patch map[string]any) (ClaimMap, error)
}

type service struct {
	repo   Repository
	logger *zap.Logger
}

// NewService creates a new claims service
func NewService(repo Repository, logger *zap.Logger) Service {
	return &service{repo: repo, logger: logger}
}

// List returns the claims stored for a user, without reserved ones.
func (s *service) List(ctx context.Context, userID, tenantID uuid.UUID) (ClaimMap, error) {
	stored, err := s.repo.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	return storedValues(stored), nil
}

// Patch applies patch to the user's stored claims. A refused patch changes
// nothing.
func (s *service) Patch(ctx context.Context, userID, tenantID uuid.UUID, patch map[string]any) (ClaimMap, error) {
	set := ClaimMap{}
	var remove []string
	for key, value := range patch {
		if err := checkKey(key); err != nil {
			return nil, err
		}
		if value == nil {
			remove = append(remove, key)
			continue
		}
		set[key] = value
	}
	if err := checkWritable(set); err != nil {
		return nil, err
	}

	current, err := s.List(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	for _, key := range remove {
		delete(current, key)
	}
	for key, value := range set {
		current[key] = value
	}
	if err := checkSize(current); err != nil {
		return nil, err
	}

	if err := s.repo.ApplyPatch(ctx, userID, tenantID, set, remove); err != nil {
		return nil, err
	}
	return current, nil
}

// storedValues maps stored claims to their values, leaving reserved keys out.
func storedValues(stored []UserClaim) ClaimMap {
	out := make(ClaimMap, len(stored))
	for _, claim := range stored {
		if !IsReserved(claim.ClaimKey) {
			out[claim.ClaimKey] = claim.value()
		}
	}
	return out
}
