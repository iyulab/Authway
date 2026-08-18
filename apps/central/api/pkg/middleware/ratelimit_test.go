package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// TestRealClientIP_TrustsRightmostForwardedFor pins the ACA-specific fix:
// Azure Container Apps' edge proxy appends only the rightmost X-Forwarded-For
// entry itself — anything to its left comes from the client verbatim and is
// spoofable. Fiber's own c.IP()/ProxyHeader would pick the leftmost entry,
// letting a caller bypass rate limiting by sending its own header.
func TestRealClientIP_TrustsRightmostForwardedFor(t *testing.T) {
	tests := []struct {
		name       string
		forwardFor string
		want       string // "" means "whatever c.IP() (RemoteAddr) resolves to" — fiber's
		// in-memory test transport doesn't honor httptest.Request.RemoteAddr, so the
		// no-header case asserts the fallback invariant (realClientIP == c.IP()) instead
		// of a literal address.
	}{
		{
			name:       "no header falls back to RemoteAddr",
			forwardFor: "",
			want:       "",
		},
		{
			name:       "single entry is trusted as-is",
			forwardFor: "203.0.113.9",
			want:       "203.0.113.9",
		},
		{
			name:       "attacker-prepended entries are ignored, only the rightmost (ACA-appended) IP counts",
			forwardFor: "6.6.6.6, 7.7.7.7, 203.0.113.9",
			want:       "203.0.113.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			var got, fallback string
			app.Get("/", func(c *fiber.Ctx) error {
				got = realClientIP(c)
				fallback = c.IP()
				return c.SendStatus(fiber.StatusOK)
			})

			req := httptest.NewRequest("GET", "/", nil)
			if tt.forwardFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardFor)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()

			want := tt.want
			if want == "" {
				want = fallback
			}
			if got != want {
				t.Errorf("realClientIP() = %q, want %q", got, want)
			}
		})
	}
}

// newTestRedis spins up an in-process miniredis instance so the rate limiter's
// actual block-after-N-failures behavior runs against something Redis-shaped,
// not just its call sites.
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

// TestLoginRateLimit_BlocksAfterMaxAttemptsThenResetsOnSuccess exercises the
// full loop a real login handler drives: RateLimit as gate, then the
// handler-side Increment/Reset calls that make the counting actually happen.
func TestLoginRateLimit_BlocksAfterMaxAttemptsThenResetsOnSuccess(t *testing.T) {
	redisClient := newTestRedis(t)
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    5,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:test:",
		SkipOnError:    true,
	}

	shouldFail := true
	app := fiber.New()
	app.Post("/authenticate", RateLimit(cfg), func(c *fiber.Ctx) error {
		if shouldFail {
			IncrementRateLimitOnFailure(c)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid_credentials"})
		}
		ResetRateLimitOnSuccess(c)
		return c.JSON(fiber.Map{"ok": true})
	})

	doRequest := func() int {
		req := httptest.NewRequest("POST", "/authenticate", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i := 1; i <= 5; i++ {
		if status := doRequest(); status != fiber.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, status)
		}
	}

	// The 6th attempt should be blocked by the rate limiter itself (429),
	// even though the handler would otherwise have returned 401 again.
	if status := doRequest(); status != fiber.StatusTooManyRequests {
		t.Fatalf("6th attempt: status = %d, want 429 (blocked)", status)
	}

	// A correct password submitted while blocked still gets the 429 — the
	// block is time-based, not attempt-based, matching RateLimit's Redis
	// block-key check running before the handler is ever reached.
	shouldFail = false
	if status := doRequest(); status != fiber.StatusTooManyRequests {
		t.Fatalf("attempt while blocked: status = %d, want 429 even on valid credentials", status)
	}
}

// TestLoginRateLimit_SuccessResetsCounter confirms a successful login clears
// the failure count so an unrelated later mistake doesn't inherit it.
func TestLoginRateLimit_SuccessResetsCounter(t *testing.T) {
	redisClient := newTestRedis(t)
	cfg := RateLimitConfig{
		RedisClient:    redisClient,
		MaxAttempts:    5,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:test2:",
		SkipOnError:    true,
	}

	outcome := make(chan bool, 1)
	app := fiber.New()
	app.Post("/authenticate", RateLimit(cfg), func(c *fiber.Ctx) error {
		if <-outcome {
			ResetRateLimitOnSuccess(c)
			return c.JSON(fiber.Map{"ok": true})
		}
		IncrementRateLimitOnFailure(c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid_credentials"})
	})

	request := func(succeed bool) int {
		outcome <- succeed
		req := httptest.NewRequest("POST", "/authenticate", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i := range 4 {
		if status := request(false); status != fiber.StatusUnauthorized {
			t.Fatalf("failure %d: status = %d, want 401", i, status)
		}
	}
	if status := request(true); status != fiber.StatusOK {
		t.Fatalf("success: status = %d, want 200", status)
	}

	// Counter was reset — 4 more failures should not trip the 5-attempt cap.
	for i := range 4 {
		if status := request(false); status != fiber.StatusUnauthorized {
			t.Fatalf("post-reset failure %d: status = %d, want 401 (not yet blocked)", i, status)
		}
	}
}
