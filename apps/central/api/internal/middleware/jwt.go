package middleware

import (
	"strings"

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
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("Invalid Authorization header format")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization header format",
			})
		}

		token := parts[1]
		if token == "" {
			logger.Warn("Empty bearer token")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Empty bearer token",
			})
		}

		// Always validate via Hydra's introspection endpoint. It works for both
		// opaque (ory_at_*) and JWT-format access tokens, and — crucially —
		// verifies the signature, active state, and expiration. The previous
		// branch that decoded JWTs locally without checking the signature
		// accepted ANY base64-encoded payload as authentication, allowing
		// trivial token forgery for `sub` and `tenant_id` claims.
		userID, tenantID, err := introspectToken(token, hydraClient, logger)
		if err != nil {
			logger.Error("Token introspection failed",
				zap.Error(err),
				zap.String("token_prefix", token[:min(20, len(token))]))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token",
			})
		}

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
func introspectToken(token string, hydraClient *hydra.Client, logger *zap.Logger) (uuid.UUID, uuid.UUID, error) {
	introspectResp, err := hydraClient.IntrospectToken(token)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if !introspectResp.Active {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Token is not active")
	}

	// Extract user_id from subject
	userID, err := uuid.Parse(introspectResp.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid user ID in token")
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

	return userID, tenantID, nil
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
