package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig holds rate limiter configuration
type RateLimitConfig struct {
	RedisClient    *redis.Client
	MaxAttempts    int
	WindowDuration time.Duration
	BlockDuration  time.Duration
	KeyPrefix      string
	SkipOnError    bool
}

// DefaultRateLimitConfig returns default rate limit settings
func DefaultRateLimitConfig(redisClient *redis.Client) RateLimitConfig {
	return RateLimitConfig{
		RedisClient:    redisClient,
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
		if cfg.RedisClient == nil {
			return c.Next()
		}
		ctx := context.Background()
		ip := realClientIP(c)
		path := c.Path()
		key := fmt.Sprintf("%s%s:%s", cfg.KeyPrefix, path, ip)
		blockKey := fmt.Sprintf("%sblock:%s:%s", cfg.KeyPrefix, path, ip)
		blocked, err := cfg.RedisClient.Exists(ctx, blockKey).Result()
		if err != nil && !cfg.SkipOnError {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "rate limit error"})
		}
		if blocked > 0 {
			ttl, _ := cfg.RedisClient.TTL(ctx, blockKey).Result()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too_many_requests",
				"message": "Too many failed attempts. Please try again later.",
				"retry_after": int(ttl.Seconds()),
			})
		}
		countStr, err := cfg.RedisClient.Get(ctx, key).Result()
		var count int64 = 0
		if err == nil {
			count, _ = strconv.ParseInt(countStr, 10, 64)
		}
		if int(count) >= cfg.MaxAttempts {
			cfg.RedisClient.Set(ctx, blockKey, "1", cfg.BlockDuration)
			cfg.RedisClient.Del(ctx, key)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too_many_requests",
				"message": "Rate limit exceeded. You have been temporarily blocked.",
				"retry_after": int(cfg.BlockDuration.Seconds()),
			})
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
	if !ok || cfg.RedisClient == nil {
		return
	}
	ctx := context.Background()
	pipe := cfg.RedisClient.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, cfg.WindowDuration)
	pipe.Exec(ctx)
}

// ResetRateLimitOnSuccess resets rate limit counter on successful attempt
func ResetRateLimitOnSuccess(c *fiber.Ctx) {
	key, ok := c.Locals("rate_limit_key").(string)
	if !ok || key == "" {
		return
	}
	cfg, ok := c.Locals("rate_limit_cfg").(RateLimitConfig)
	if !ok || cfg.RedisClient == nil {
		return
	}
	ctx := context.Background()
	cfg.RedisClient.Del(ctx, key)
}

// LoginRateLimit creates rate limiting specifically for login endpoints
func LoginRateLimit(redisClient *redis.Client) fiber.Handler {
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    5,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:login:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// RegisterRateLimit creates rate limiting for registration endpoints
func RegisterRateLimit(redisClient *redis.Client) fiber.Handler {
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    3,
		WindowDuration: time.Minute,
		BlockDuration:  30 * time.Minute,
		KeyPrefix:      "ratelimit:register:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// PasswordResetRateLimit creates rate limiting for password reset
func PasswordResetRateLimit(redisClient *redis.Client) fiber.Handler {
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    3,
		WindowDuration: time.Hour,
		BlockDuration:  time.Hour,
		KeyPrefix:      "ratelimit:pwreset:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}

// APIRateLimit creates general API rate limiting
func APIRateLimit(redisClient *redis.Client, maxRequests int, window time.Duration) fiber.Handler {
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    maxRequests,
		WindowDuration: window,
		BlockDuration:  window,
		KeyPrefix:      "ratelimit:api:",
		SkipOnError:    true,
	}
	return RateLimit(cfg)
}
