package middleware

import (
	"authway/apps/central/api/pkg/apierror"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"authway/apps/central/api/pkg/ephemeral"
)

// RateLimitConfig holds rate limiter configuration
type RateLimitConfig struct {
	Store          ephemeral.Store
	MaxAttempts    int
	WindowDuration time.Duration
	BlockDuration  time.Duration
	KeyPrefix      string
	SkipOnError    bool
}

// DefaultRateLimitConfig returns default rate limit settings
func DefaultRateLimitConfig(store ephemeral.Store) RateLimitConfig {
	return RateLimitConfig{
		Store:          store,
		MaxAttempts:    5,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:",
		SkipOnError:    true,
	}
}

// realClientIP resolves the actual client IP behind Azure Container Apps' ingress.
//
// ACA's edge proxy is the only path into the container (no VNet, no direct-connect
// bypass), so the immediate TCP peer is always trustworthy. But per Azure's own docs,
// only the RIGHTMOST entry of X-Forwarded-For is appended by ACA itself — anything to
// its left arrives verbatim from the client and is trivially spoofable. Fiber's built-in
// c.IP()/ProxyHeader picks the LEFTMOST entry, which would let a caller bypass rate
// limiting by sending its own X-Forwarded-For header. Falls back to c.IP() (RemoteAddr)
// when the header is absent, e.g. local dev without a proxy in front.
func realClientIP(c *fiber.Ctx) string {
	ips := c.IPs()
	if len(ips) == 0 {
		return c.IP()
	}
	return ips[len(ips)-1]
}

// RateLimit creates a rate limiting middleware
func RateLimit(cfg RateLimitConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.Next()
		}
		ctx := context.Background()
		ip := realClientIP(c)
		// Count per route, not per request path: on a route such as
		// /api/v1/login-flows/:flow/password every new flow id would otherwise
		// start a fresh counter, and starting a new flow costs an attacker one
		// request.
		path := c.Route().Path
		key := fmt.Sprintf("%s%s:%s", cfg.KeyPrefix, path, ip)
		blockKey := fmt.Sprintf("%sblock:%s:%s", cfg.KeyPrefix, path, ip)
		ttl, blocked, err := cfg.Store.Remaining(ctx, blockKey)
		if err != nil && !cfg.SkipOnError {
			return apierror.Refuse(c, fiber.StatusInternalServerError, apierror.CodeForStatus(fiber.StatusInternalServerError), "rate limit error")
		}
		if blocked {
			return apierror.RefuseWith(c, fiber.StatusTooManyRequests, "too_many_requests", "Too many attempts. Try again later.", fiber.Map{"retry_after": int(ttl.Seconds())})
		}
		countStr, _, _ := cfg.Store.Value(ctx, key)
		count, _ := strconv.ParseInt(countStr, 10, 64)
		if int(count) >= cfg.MaxAttempts {
			cfg.Store.SetValue(ctx, blockKey, "1", cfg.BlockDuration)
			cfg.Store.Remove(ctx, key)
			return apierror.RefuseWith(c, fiber.StatusTooManyRequests, "too_many_requests", "Too many attempts. Try again later.", fiber.Map{"retry_after": int(cfg.BlockDuration.Seconds())})
		}
		c.Locals("rate_limit_key", key)
		c.Locals("rate_limit_cfg", cfg)
		return c.Next()
	}
}

// IncrementRateLimitOnFailure increments rate limit counter on failed attempt
func IncrementRateLimitOnFailure(c *fiber.Ctx) {
	key, ok := c.Locals("rate_limit_key").(string)
	if !ok || key == "" {
		return
	}
	cfg, ok := c.Locals("rate_limit_cfg").(RateLimitConfig)
	if !ok || cfg.Store == nil {
		return
	}
	cfg.Store.Count(context.Background(), key, cfg.WindowDuration)
}

// ResetRateLimitOnSuccess resets rate limit counter on successful attempt
func ResetRateLimitOnSuccess(c *fiber.Ctx) {
	key, ok := c.Locals("rate_limit_key").(string)
	if !ok || key == "" {
		return
	}
	cfg, ok := c.Locals("rate_limit_cfg").(RateLimitConfig)
	if !ok || cfg.Store == nil {
		return
	}
	cfg.Store.Remove(context.Background(), key)
}

// LoginRateLimit creates rate limiting specifically for login endpoints
func LoginRateLimit(store ephemeral.Store) fiber.Handler {
	cfg := RateLimitConfig{
		Store:          store,
		MaxAttempts:    5,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:login:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// MagicLinkRateLimit limits sign-in link emails. The handler counts every
// request (each one may send an email), not just failures.
// MagicLinkRateLimit bounds the sign-in emails one network address asks for.
// It is a ceiling for an address many people share (an office, a mobile
// carrier); how many links one mailbox receives is limited per mailbox, by
// the passwordless service.
func MagicLinkRateLimit(store ephemeral.Store) fiber.Handler {
	return RateLimit(RateLimitConfig{
		Store:          store,
		MaxAttempts:    20,
		WindowDuration: 15 * time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:magiclink:",
		SkipOnError:    true,
	})
}

// RegisterRateLimit creates rate limiting for registration endpoints
func RegisterRateLimit(store ephemeral.Store) fiber.Handler {
	cfg := RateLimitConfig{
		Store:          store,
		MaxAttempts:    3,
		WindowDuration: time.Minute,
		BlockDuration:  30 * time.Minute,
		KeyPrefix:      "ratelimit:register:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// PasswordResetRateLimit creates rate limiting for password reset
func PasswordResetRateLimit(store ephemeral.Store) fiber.Handler {
	cfg := RateLimitConfig{
		Store:          store,
		MaxAttempts:    3,
		WindowDuration: time.Hour,
		BlockDuration:  time.Hour,
		KeyPrefix:      "ratelimit:pwreset:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// APIRateLimit creates general API rate limiting
func APIRateLimit(store ephemeral.Store, maxRequests int, window time.Duration) fiber.Handler {
	cfg := RateLimitConfig{
		Store:          store,
		MaxAttempts:    maxRequests,
		WindowDuration: window,
		BlockDuration:  window,
		KeyPrefix:      "ratelimit:api:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}
