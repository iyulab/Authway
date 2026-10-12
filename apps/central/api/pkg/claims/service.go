package claims

import (
	"context"
	"fmt"
	"time"

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
	// List returns the claims stored for a user.
	List(ctx context.Context, userID, tenantID uuid.UUID) (ClaimMap, error)
	// Patch applies a JSON merge patch to a user's stored claims — a value sets
	// the claim, nil removes it — and returns the claims stored afterwards.
	Patch(ctx context.Context, userID, tenantID uuid.UUID, patch map[string]any) (ClaimMap, error)

	// Internal operations for OAuth flow
	GetClaimsForLogin(ctx context.Context, userID, tenantID uuid.UUID, loginChallenge string) (ClaimMap, error)
	// GetClaimsForConsent returns the stored claims for the token being
	// issued, without any reserved claim, and where they came from. The caller
	// puts the issuer's claims on top (Issued).
	GetClaimsForConsent(ctx context.Context, loginChallenge string, userID, tenantID uuid.UUID) (ClaimMap, ClaimsSource, error)
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

// GetClaimsForLogin is called during login flow to prepare claims for this session
func (s *service) GetClaimsForLogin(ctx context.Context, userID, tenantID uuid.UUID, loginChallenge string) (ClaimMap, error) {
	s.logger.Info("Preparing claims for login",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
		zap.String("login_challenge", loginChallenge),
	)

	// 1. Get pending claims from Redis (if user just updated claims)
	pendingClaims, err := s.repo.GetPendingClaims(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get pending claims", zap.Error(err))
		pendingClaims = nil
	}

	// 2. Get permanent claims from database
	dbClaims, err := s.repo.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get database claims", zap.Error(err))
		dbClaims = nil
	}

	// 3. Merge claims (pending claims override database claims)
	allClaims := storedValues(dbClaims)
	for key, value := range withoutReserved(pendingClaims) {
		allClaims[key] = value
	}

	// 4. Store merged claims associated with login challenge
	if len(allClaims) > 0 {
		err = s.repo.SetLoginClaims(ctx, loginChallenge, allClaims, 10*time.Minute)
		if err != nil {
			s.logger.Error("Failed to set login claims", zap.Error(err))
			return nil, fmt.Errorf("failed to set login claims: %w", err)
		}
	}

	// 5. DON'T delete pending claims yet - consent might need them as fallback
	// They will expire automatically via Redis TTL (5 minutes)
	// This handles the case where Hydra reuses an old consent challenge

	s.logger.Info("Claims prepared for login",
		zap.Int("claim_count", len(allClaims)),
		zap.String("login_challenge", loginChallenge),
	)

	return allClaims, nil
}

// ClaimsSource indicates where the claims for a consent came from.
type ClaimsSource string

const (
	ClaimsSourceLoginChallenge ClaimsSource = "login_challenge"
	ClaimsSourcePending        ClaimsSource = "pending"  // Claims update scenario
	ClaimsSourceDatabase       ClaimsSource = "database" // Initial login scenario
	ClaimsSourceNone           ClaimsSource = "none"
)

// GetClaimsForConsent is called during the consent flow. It prefers the claims
// prepared for the login challenge, then falls back to the permanent claims
// with any pending claims on top (a claims update whose login challenge is
// gone). Reserved claims never come out of it.
func (s *service) GetClaimsForConsent(ctx context.Context, loginChallenge string, userID, tenantID uuid.UUID) (ClaimMap, ClaimsSource, error) {
	s.logger.Info("Getting claims for consent",
		zap.String("login_challenge", loginChallenge),
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
	)

	// Strategy 1: claims associated with this login challenge.
	loginClaims, err := s.repo.GetLoginClaims(ctx, loginChallenge)
	if err != nil {
		s.logger.Error("Failed to get login claims", zap.Error(err))
		return nil, ClaimsSourceNone, fmt.Errorf("failed to get login claims: %w", err)
	}
	if len(loginClaims) > 0 {
		return withoutReserved(loginClaims), ClaimsSourceLoginChallenge, nil
	}

	// Strategies 2 and 3: permanent claims, with pending claims on top.
	pendingClaims, err := s.repo.GetPendingClaims(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get pending claims", zap.Error(err))
	}
	dbClaims, err := s.repo.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		s.logger.Warn("Failed to get database claims", zap.Error(err))
	}

	claims := storedValues(dbClaims)
	if len(pendingClaims) > 0 {
		for key, value := range withoutReserved(pendingClaims) {
			claims[key] = value
		}
		// The pending claims are used now; a later consent must not apply them again.
		if err := s.repo.DeletePendingClaims(ctx, userID); err != nil {
			s.logger.Warn("Failed to delete pending claims after use", zap.Error(err))
		}
		return claims, ClaimsSourcePending, nil
	}
	if len(dbClaims) > 0 {
		return claims, ClaimsSourceDatabase, nil
	}
	return claims, ClaimsSourceNone, nil
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
