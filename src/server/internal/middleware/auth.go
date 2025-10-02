package middleware

import (
	// "strings"  // Currently unused

	// "authway/src/server/pkg/token"  // Package not yet implemented
	"github.com/gofiber/fiber/v2"
)

// RequireAuth middleware validates JWT tokens - Package not yet implemented
// func RequireAuth(tokenService token.Service) fiber.Handler {
//	return func(c *fiber.Ctx) error {
//		authHeader := c.Get("Authorization")
//		if authHeader == "" {
//			return fiber.NewError(fiber.StatusUnauthorized, "Authorization header required")
//		}
//
//		// Extract token from "Bearer <token>" format
//		parts := strings.Split(authHeader, " ")
//		if len(parts) != 2 || parts[0] != "Bearer" {
//			return fiber.NewError(fiber.StatusUnauthorized, "Invalid authorization header format")
//		}
//
//		tokenString := parts[1]
//		claims, err := tokenService.ValidateAccessToken(tokenString)
//		if err != nil {
//			return fiber.NewError(fiber.StatusUnauthorized, "Invalid or expired token")
//		}
//
//		// Store user information in context
//		c.Locals("userID", claims.UserID)
//		c.Locals("clientID", claims.ClientID)
//		c.Locals("scopes", claims.Scopes)
//
//		return c.Next()
//	}
// }

// RequireAdmin middleware checks if user has admin role
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		scopes, ok := c.Locals("scopes").([]string)
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "Access denied")
		}

		// Check if user has admin scope
		for _, scope := range scopes {
			if scope == "admin" {
				return c.Next()
			}
		}

		return fiber.NewError(fiber.StatusForbidden, "Admin access required")
	}
}

// RequireScope middleware checks if user has required scope
func RequireScope(requiredScope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scopes, ok := c.Locals("scopes").([]string)
		if !ok {
			return fiber.NewError(fiber.StatusForbidden, "Access denied")
		}

		// Check if user has required scope
		for _, scope := range scopes {
			if scope == requiredScope {
				return c.Next()
			}
		}

		return fiber.NewError(fiber.StatusForbidden, "Insufficient scope")
	}
}
