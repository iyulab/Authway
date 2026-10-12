package claims

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service defines the interface for claims business logic
type Service interface {
	// System claims (require re-authentication)
	UpdateClaims(ctx context.Context, userID, tenantID uuid.UUID, req *UpdateClaimsRequest) (*UpdateClaimsResponse, error)
	GetClaims(ctx context.Context, userID, tenantID uuid.UUID) (*GetClaimsResponse, error)
	DeleteClaim(ctx context.Context, userID, tenantID uuid.UUID, claimKey string) (*DeleteClaimResponse, error)

	// User claims (no re-authentication required)
	UpdateUserClaims(ctx context.Context, userID, tenantID uuid.UUID, req *UpdateUserClaimsRequest) (*UpdateUserClaimsResponse, error)
	GetUserClaims(ctx context.Context, userID, tenantID uuid.UUID) (*GetUserClaimsResponse, error)

	// Internal operations for OAuth flow
	GetClaimsForLogin(ctx context.Context, userID, tenantID uuid.UUID, loginChallenge string) (ClaimMap, error)
	// GetClaimsForConsent returns the stored claims for the token being
	// issued, without any reserved claim, and where they came from. The caller
	// puts the issuer's claims on top (Issued).
	GetClaimsForConsent(ctx context.Context, loginChallenge string, userID, tenantID uuid.UUID) (ClaimMap, ClaimsSource, error)
}

type service struct {
	repo           Repository
	hydraPublicURL string
	logger         *zap.Logger
}

// NewService creates a new claims service
func NewService(repo Repository, hydraPublicURL string, logger *zap.Logger) Service {
	return &service{
		repo:           repo,
		hydraPublicURL: hydraPublicURL,
		logger:         logger,
	}
}

// UpdateClaims updates claims for a user and returns a re-authentication URL
func (s *service) UpdateClaims(ctx context.Context, userID, tenantID uuid.UUID, req *UpdateClaimsRequest) (*UpdateClaimsResponse, error) {
	s.logger.Info("Updating claims for user",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
		zap.Int("claim_count", len(req.Claims)),
		zap.Bool("permanent", req.Permanent),
	)
	if err := checkWritable(req.Claims); err != nil {
		return nil, err
	}

	// 1. Store pending claims in Redis (will be applied on next authentication)
	err := s.repo.SetPendingClaims(ctx, userID, req.Claims, 5*time.Minute)
	if err != nil {
		s.logger.Error("Failed to set pending claims", zap.Error(err))
		return nil, fmt.Errorf("failed to set pending claims: %w", err)
	}

	// 2. If permanent, also save to database
	if req.Permanent {
		for key, value := range req.Claims {
			claim := &UserClaim{
				ID:          uuid.New(),
				UserID:      userID,
				TenantID:    tenantID,
				ClaimKey:    key,
				ClaimValue:  map[string]any{"value": value},
				IsPermanent: true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			err := s.repo.SaveClaim(ctx, claim)
			if err != nil {
				s.logger.Error("Failed to save permanent claim",
					zap.String("claim_key", key),
					zap.Error(err),
				)
				return nil, fmt.Errorf("failed to save permanent claim: %w", err)
			}
		}
	}

	// 3. Build re-authentication URL with prompt=none
	authURL := s.buildReauthURL(req.ClientID, req.RedirectURI)

	s.logger.Info("Claims updated successfully",
		zap.String("user_id", userID.String()),
		zap.String("auth_url", authURL),
	)

	return &UpdateClaimsResponse{
		Success: true,
		AuthURL: authURL,
		Message: "Claims updated. Re-authenticate to receive new token with updated claims.",
	}, nil
}

// GetClaims retrieves all claims for a user
func (s *service) GetClaims(ctx context.Context, userID, tenantID uuid.UUID) (*GetClaimsResponse, error) {
	s.logger.Info("Getting claims for user",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
	)

	// 1. Get permanent claims from database
	dbClaims, err := s.repo.GetUserClaims(ctx, userID, tenantID)
	if err != nil {
		s.logger.Error("Failed to get database claims", zap.Error(err))
		return nil, fmt.Errorf("failed to get database claims: %w", err)
	}

	// 2. Get pending session claims from Redis
	pendingClaims, err := s.repo.GetPendingClaims(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to get pending claims", zap.Error(err))
		// Continue even if Redis fails
		pendingClaims = nil
	}

	// 3. Merge claims
	allClaims := make(ClaimMap)
	permanentKeys := []string{}
	sessionKeys := []string{}

	for _, claim := range dbClaims {
		if IsReserved(claim.ClaimKey) {
			continue
		}
		allClaims[claim.ClaimKey] = claim.value()
		if claim.IsPermanent {
			permanentKeys = append(permanentKeys, claim.ClaimKey)
		}
	}

	// Pending claims override database claims
	for key, value := range withoutReserved(pendingClaims) {
		allClaims[key] = value
		sessionKeys = append(sessionKeys, key)
	}

	return &GetClaimsResponse{
		UserID:          userID,
		TenantID:        tenantID,
		Claims:          allClaims,
		PermanentClaims: permanentKeys,
		SessionClaims:   sessionKeys,
	}, nil
}

// DeleteClaim deletes a specific claim for a user
func (s *service) DeleteClaim(ctx context.Context, userID, tenantID uuid.UUID, claimKey string) (*DeleteClaimResponse, error) {
	s.logger.Info("Deleting claim",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
		zap.String("claim_key", claimKey),
	)

	err := s.repo.DeleteClaim(ctx, userID, tenantID, claimKey)
	if err != nil {
		s.logger.Error("Failed to delete claim", zap.Error(err))
		return nil, fmt.Errorf("failed to delete claim: %w", err)
	}

	s.logger.Info("Claim deleted successfully", zap.String("claim_key", claimKey))

	return &DeleteClaimResponse{
		Success: true,
		Message: fmt.Sprintf("Claim '%s' deleted successfully", claimKey),
	}, nil
}

// UpdateUserClaims updates user-level claims (no re-authentication required)
func (s *service) UpdateUserClaims(ctx context.Context, userID, tenantID uuid.UUID, req *UpdateUserClaimsRequest) (*UpdateUserClaimsResponse, error) {
	s.logger.Info("Updating user claims (no re-auth)",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
		zap.Int("claim_count", len(req.Claims)),
	)
	if err := checkWritable(req.Claims); err != nil {
		return nil, err
	}

	// Save user claims directly to database with claim_type='user'
	for key, value := range req.Claims {
		claim := &UserClaim{
			ID:          uuid.New(),
			UserID:      userID,
			TenantID:    tenantID,
			ClaimKey:    key,
			ClaimValue:  map[string]any{"value": value},
			ClaimType:   "user", // User claims don't require re-auth
			IsPermanent: true,   // User claims are always permanent
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		err := s.repo.SaveClaim(ctx, claim)
		if err != nil {
			s.logger.Error("Failed to save user claim",
				zap.String("claim_key", key),
				zap.Error(err),
			)
			return nil, fmt.Errorf("failed to save user claim: %w", err)
		}
	}

	s.logger.Info("User claims updated successfully",
		zap.String("user_id", userID.String()),
		zap.Int("claims_added", len(req.Claims)),
	)

	return &UpdateUserClaimsResponse{
		Success: true,
		Message: "User claims updated successfully. No re-authentication required.",
	}, nil
}

// GetUserClaims retrieves user-level claims (claim_type='user')
func (s *service) GetUserClaims(ctx context.Context, userID, tenantID uuid.UUID) (*GetUserClaimsResponse, error) {
	s.logger.Info("Getting user claims",
		zap.String("user_id", userID.String()),
		zap.String("tenant_id", tenantID.String()),
	)

	// Get only user-type claims from database
	dbClaims, err := s.repo.GetUserClaimsByType(ctx, userID, tenantID, "user")
	if err != nil {
		s.logger.Error("Failed to get user claims", zap.Error(err))
		return nil, fmt.Errorf("failed to get user claims: %w", err)
	}

	// Convert to ClaimMap
	return &GetUserClaimsResponse{
		UserID:   userID,
		TenantID: tenantID,
		Claims:   storedValues(dbClaims),
	}, nil
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

// getClaimKeys returns all keys from a ClaimMap for logging purposes
func getClaimKeys(claims ClaimMap) []string {
	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	return keys
}

// buildReauthURL constructs an OAuth authorization URL for silent re-authentication
// Note: We don't use prompt=none because it requires pre-existing consent in Hydra
// Instead, we rely on the existing session - if the user is already logged in,
// Hydra will automatically redirect without prompting
func (s *service) buildReauthURL(clientID, redirectURI string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	// Don't set prompt=none to avoid consent_required errors
	// Hydra will automatically use existing session if available
	params.Set("scope", "openid profile email")
	params.Set("state", uuid.New().String())

	return fmt.Sprintf("%s/oauth2/auth?%s", s.hydraPublicURL, params.Encode())
}
