package middleware

import (
	"authway/apps/central/api/internal/hydra"
	"encoding/base64"
	"encoding/json"
	"strings"

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

		var userID uuid.UUID
		var tenantID uuid.UUID
		var err error

		// Check if token is opaque (starts with "ory_at_") or JWT (starts with "eyJ")
		if strings.HasPrefix(token, "ory_at_") || strings.HasPrefix(token, "ory_rt_") {
			// Opaque token - use Hydra token introspection
			logger.Info("Using token introspection for opaque token")
			userID, tenantID, err = introspectOpaqueToken(token, hydraClient, logger)
			if err != nil {
				logger.Error("Token introspection failed",
					zap.Error(err),
					zap.String("token_prefix", token[:min(20, len(token))]))
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "Invalid token",
				})
			}
		} else {
			// JWT token - parse claims directly (without signature validation for now)
			logger.Info("Using JWT parsing")
			userID, tenantID, err = extractClaimsFromToken(token)
			if err != nil {
				logger.Error("Failed to extract claims from JWT",
					zap.Error(err),
					zap.String("token_prefix", token[:min(20, len(token))]))
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "Invalid token",
				})
			}
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

// introspectOpaqueToken uses Hydra's token introspection endpoint to validate and extract claims
func introspectOpaqueToken(token string, hydraClient *hydra.Client, logger *zap.Logger) (uuid.UUID, uuid.UUID, error) {
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

// extractClaimsFromToken extracts user_id and tenant_id from JWT token
// Using JWT decoding without signature verification for development
// In production, should use Hydra's token introspection endpoint or JWKS validation
func extractClaimsFromToken(token string) (userID uuid.UUID, tenantID uuid.UUID, err error) {
	// Split JWT into parts (header.payload.signature)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid JWT format")
	}

	// Decode payload (base64url)
	payload := parts[1]

	// Add padding if needed
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	// Decode base64
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Failed to decode JWT payload")
		}
	}

	// Parse JSON claims
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Failed to parse JWT claims")
	}

	// Extract user_id from "sub" claim
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Missing sub claim")
	}

	userID, err = uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid user ID in token")
	}

	// Extract tenant_id claim (optional - may not be in Hydra tokens)
	// Hydra puts custom claims in "ext" object
	tenantIDStr := ""

	// Try to get from top-level first (backward compatibility)
	if tid, ok := claims["tenant_id"].(string); ok {
		tenantIDStr = tid
	} else if ext, ok := claims["ext"].(map[string]interface{}); ok {
		// Try to get from ext object (Hydra custom claims)
		if tid, ok := ext["tenant_id"].(string); ok {
			tenantIDStr = tid
		}
	}

	if tenantIDStr != "" {
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid tenant ID in token")
		}
	} else {
		// If tenant_id is not in token, use nil (will be resolved later from user's tenant)
		tenantID = uuid.Nil
	}

	return userID, tenantID, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
