package middleware

import (
	"authway/apps/central/api/pkg/apierror"
	"errors"
	"strings"
	"time"

	"authway/apps/central/api/internal/hydra"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// JWTAuth middleware validates access tokens (both JWT and opaque) from Authorization header
// Uses Hydra token introspection for opaque tokens and JWT parsing for JWT tokens
func JWTAuth(logger *zap.Logger, hydraClient *hydra.Client, db ...*gorm.DB) fiber.Handler {
	var database *gorm.DB
	if len(db) > 0 {
		database = db[0]
	}
	return func(c *fiber.Ctx) error {
		// Get Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			logger.Warn("Missing Authorization header")
			return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Missing authorization header")
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Invalid Authorization header format")
			return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Invalid authorization header format")
		}

		token := parts[1]
		if token == "" {
			logger.Warn("Empty bearer token")
			return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Empty bearer token")
		}

		// Always validate via Hydra's introspection endpoint. It works for both
		// opaque (ory_at_*) and JWT-format access tokens, and — crucially —
		// verifies the signature, active state, and expiration. The previous
		// branch that decoded JWTs locally without checking the signature
		// accepted ANY base64-encoded payload as authentication, allowing
		// trivial token forgery for `sub` and `tenant_id` claims.
		found, err := introspectToken(token, hydraClient, logger)
		if err != nil {
			// A token that is expired, revoked or malformed is the caller's
			// to fix and is answered 401; only a failure to ask the
			// authorization server is this service's.
			var refused *fiber.Error
			if errors.As(err, &refused) && refused.Code == fiber.StatusUnauthorized {
				logger.Debug("Bearer token refused", zap.String("reason", refused.Message))
			} else {
				logger.Error("Token introspection failed", zap.Error(err))
			}
			c.Set(fiber.HeaderWWWAuthenticate, `Bearer error="invalid_token"`)
			return apierror.Refuse(c, fiber.StatusUnauthorized, "unauthorized", "Invalid token")
		}
		// These routes are the user's own account. A token a client was issued
		// to act for the user (it names the client in act, RFC 8693) is for
		// the client's work, not for managing the account: the client could
		// otherwise enrol its own second factor, or revoke what the user
		// granted.
		if found.delegated {
			c.Set(fiber.HeaderWWWAuthenticate, `Bearer error="insufficient_scope"`)
			return apierror.Refuse(c, fiber.StatusForbidden, "delegated_token", "A token a client holds to act for the user cannot manage the user's account.")
		}

		userID, tenantID, authTime, sessionID := found.userID, found.tenantID, found.authTime, found.sessionID
		// If tenant_id is not in token (nil), look it up from user
		if tenantID == uuid.Nil && database != nil {
			var user struct {
				TenantID uuid.UUID `gorm:"column:tenant_id"`
			}
			if err := database.Table("users").Select("tenant_id").Where("id = ?", userID).First(&user).Error; err == nil {
				tenantID = user.TenantID
				logger.Info("Resolved tenant_id from user", zap.String("tenant_id", tenantID.String()))
			} else {
				logger.Warn("Failed to resolve tenant_id from user", zap.Error(err))
			}
		}

		// Store user_id and tenant_id in context
		c.Locals("user_id", userID)
		c.Locals("tenant_id", tenantID)
		c.Locals("access_token", token)
		// The client the token was issued to.
		c.Locals("client_id", found.clientID)
		// When the user of the session this token came from last proved who
		// they are; absent when that is not known.
		if !authTime.IsZero() {
			c.Locals("auth_time", authTime)
		}
		// The login session the token was issued in; absent for a token issued
		// before sessions were named in tokens.
		if sessionID != "" {
			c.Locals("session_id", sessionID)
		}

		logger.Info("Token authenticated",
			zap.String("user_id", userID.String()),
			zap.String("tenant_id", tenantID.String()))

		return c.Next()
	}
}

// introspectToken validates a bearer token (opaque or JWT) via Hydra's
// /admin/oauth2/introspect endpoint and extracts user_id + tenant_id.
//
// Hydra's introspection performs full signature/expiration/active checks for
// both token formats, so this is the only safe validation path — local JWT
// decode-without-verify is NEVER acceptable in this codebase.
// introspected is what a user's access token says.
type introspected struct {
	userID, tenantID uuid.UUID
	authTime         time.Time
	sessionID        string
	clientID         string
	// delegated is true for a token a client was issued to act for the user
	// (it names the actor, act).
	delegated bool
}

func introspectToken(token string, hydraClient *hydra.Client, logger *zap.Logger) (*introspected, error) {
	introspectResp, err := hydraClient.IntrospectToken(token)
	if err != nil {
		return nil, err
	}

	if !introspectResp.Active {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Token is not active")
	}

	// Extract user_id from subject
	userID, err := uuid.Parse(introspectResp.Subject)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid user ID in token")
	}

	// Extract tenant_id from ext claims
	var tenantID uuid.UUID
	if introspectResp.Ext != nil {
		if tidStr, ok := introspectResp.Ext["tenant_id"].(string); ok {
			tenantID, err = uuid.Parse(tidStr)
			if err != nil {
				logger.Warn("Invalid tenant_id in introspection response", zap.Error(err))
				tenantID = uuid.Nil
			}
		}
	}

	// JSON numbers arrive as float64.
	var authTime time.Time
	if unix, ok := introspectResp.Ext["auth_time"].(float64); ok && unix > 0 {
		authTime = time.Unix(int64(unix), 0)
	}

	sessionID, _ := introspectResp.Ext["sid"].(string)

	// A token exchanged for a delegation names its actor (act).
	_, delegated := introspectResp.Ext["act"]

	return &introspected{
		userID: userID, tenantID: tenantID, authTime: authTime, sessionID: sessionID,
		clientID: introspectResp.ClientID, delegated: delegated,
	}, nil
}

// (Removed: extractClaimsFromToken — it decoded JWTs without verifying their
// signature, which let any caller forge `sub` and `tenant_id`. All token
// validation now goes through introspectToken → Hydra. See the security
// note above introspectToken.)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
