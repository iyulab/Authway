package middleware

import (
	"authway/apps/central/api/internal/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"authway/apps/central/api/pkg/ephemeral"
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
func newTestRedis(t *testing.T) ephemeral.Store {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// Under a prefix, as a deployment that shares its Redis runs: a command
	// the prefix cannot place fails here rather than there.
	database.UseKeyPrefix(client, "test:")
	return ephemeral.NewRedis(client)
}

// TestLoginRateLimit_BlocksAfterMaxAttemptsThenResetsOnSuccess exercises the
// full loop a real login handler drives: RateLimit as gate, then the
// handler-side Increment/Reset calls that make the counting actually happen.
func TestLoginRateLimit_BlocksAfterMaxAttemptsThenResetsOnSuccess(t *testing.T) {
	redisClient := newTestRedis(t)
	cfg := RateLimitConfig{
		Store:          redisClient,
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
		Store:          redisClient,
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

// TestRateLimit_CountsPerRouteNotPerPath pins that a path parameter does not
// split the counter: each failed attempt below uses a different flow id, as an
// attacker restarting the sign-in would, and the limit still applies.
func TestRateLimit_CountsPerRouteNotPerPath(t *testing.T) {
	cfg := RateLimitConfig{
		Store:          newTestRedis(t),
		MaxAttempts:    3,
		WindowDuration: time.Minute,
		BlockDuration:  15 * time.Minute,
		KeyPrefix:      "ratelimit:test:",
		SkipOnError:    true,
	}
	app := fiber.New()
	app.Post("/flows/:flow/password", RateLimit(cfg), func(c *fiber.Ctx) error {
		IncrementRateLimitOnFailure(c)
		return c.SendStatus(fiber.StatusUnauthorized)
	})

	statusFor := func(flow string) int {
		req := httptest.NewRequest("POST", "/flows/"+flow+"/password", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for i, flow := range []string{"a", "b", "c"} {
		if status := statusFor(flow); status != fiber.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, status)
		}
	}
	if status := statusFor("d"); status != fiber.StatusTooManyRequests {
		t.Fatalf("attempt on a fresh flow id: status = %d, want 429", status)
	}
}

// A blocked request answers like every other refusal: a message to show and
// a stable code, plus when to retry.
func TestRateLimit_BlockedAnswerHasCode(t *testing.T) {
	cfg := RateLimitConfig{
		Store:          newTestRedis(t),
		MaxAttempts:    1,
		WindowDuration: time.Minute,
		BlockDuration:  time.Minute,
		KeyPrefix:      "ratelimit:test:",
		SkipOnError:    true,
	}
	app := fiber.New()
	app.Post("/x", RateLimit(cfg), func(c *fiber.Ctx) error {
		IncrementRateLimitOnFailure(c)
		return c.SendStatus(fiber.StatusUnauthorized)
	})
	send := func() *http.Response {
		resp, err := app.Test(httptest.NewRequest("POST", "/x", nil))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	send()
	resp := send()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests || body["code"] != "too_many_requests" || body["error"] == "" || body["retry_after"] == nil {
		t.Fatalf("status %d body %v", resp.StatusCode, body)
	}
}

// newTestRedisClock is newTestRedis with the clock in the test's hands.
func newTestRedisClock(t *testing.T) (ephemeral.Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	database.UseKeyPrefix(client, "test:")
	return ephemeral.NewRedis(client), mr
}

// statusFrom sends one request to path from the given address.
func statusFrom(t *testing.T, app *fiber.App, method, path, ip string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestRateLimit_CountEveryRequestIsACeilingPerAddressAndRoute: an endpoint with
// no failure to count still has a ceiling — the middleware counts each request
// itself — and the ceiling belongs to one address on one route.
func TestRateLimit_CountEveryRequestIsACeilingPerAddressAndRoute(t *testing.T) {
	store, _ := newTestRedisClock(t)
	limit := RateLimit(RateLimitConfig{
		Store:             store,
		MaxAttempts:       3,
		WindowDuration:    time.Minute,
		BlockDuration:     time.Minute,
		KeyPrefix:         "ratelimit:test:",
		SkipOnError:       true,
		CountEveryRequest: true,
	})
	app := fiber.New()
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Get("/start/:flow", limit, ok)
	app.Get("/other", limit, ok)

	for i := 1; i <= 3; i++ {
		// A new flow id each time, as a client restarting the sign-in would.
		if resp := statusFrom(t, app, "GET", "/start/f"+string(rune('a'+i)), "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i, resp.StatusCode)
		}
	}
	blocked := statusFrom(t, app, "GET", "/start/fz", "203.0.113.9")
	var body map[string]any
	if err := json.NewDecoder(blocked.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if blocked.StatusCode != fiber.StatusTooManyRequests || body["code"] != "too_many_requests" || body["retry_after"] == nil {
		t.Fatalf("fourth request: status %d body %v, want 429 with a code and when to retry", blocked.StatusCode, body)
	}
	if resp := statusFrom(t, app, "GET", "/start/fa", "203.0.113.9"); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("while blocked: status %d, want 429", resp.StatusCode)
	}
	if resp := statusFrom(t, app, "GET", "/start/fa", "198.51.100.7"); resp.StatusCode != fiber.StatusOK {
		t.Errorf("another address: status %d, want 200", resp.StatusCode)
	}
	if resp := statusFrom(t, app, "GET", "/other", "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
		t.Errorf("another route from the blocked address: status %d, want 200", resp.StatusCode)
	}
}

// TestRateLimit_CountEveryRequestOpensAgainWhateverTheTraffic: the window starts
// at the first request. Requests that keep arriving inside it do not extend it,
// so an address that stays under the ceiling is never blocked by how long it
// has been using the service.
func TestRateLimit_CountEveryRequestOpensAgainWhateverTheTraffic(t *testing.T) {
	store, mr := newTestRedisClock(t)
	limit := RateLimit(RateLimitConfig{
		Store:             store,
		MaxAttempts:       3,
		WindowDuration:    time.Minute,
		BlockDuration:     time.Minute,
		KeyPrefix:         "ratelimit:test:",
		SkipOnError:       true,
		CountEveryRequest: true,
	})
	app := fiber.New()
	app.Get("/start", limit, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	for _, wait := range []time.Duration{0, 30 * time.Second, 29 * time.Second} {
		mr.FastForward(wait)
		if resp := statusFrom(t, app, "GET", "/start", "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("inside the window: status %d, want 200", resp.StatusCode)
		}
	}
	// 61 seconds after the first request: a new window, though a request came
	// two seconds before. A window that moved with each request would be at
	// four here.
	mr.FastForward(2 * time.Second)
	if resp := statusFrom(t, app, "GET", "/start", "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("after the window: status %d, want 200", resp.StatusCode)
	}
}

// TestMagicLinkRateLimit_IsACeilingNotAFuse: the handler counts each request, and
// an address that asks for a sign-in link now and then — here every 14 minutes,
// for hours — stays under a limit of 20 per 15 minutes. The counter used to
// live as long as the traffic did, so that address was blocked after the 20th.
func TestMagicLinkRateLimit_IsACeilingNotAFuse(t *testing.T) {
	store, mr := newTestRedisClock(t)
	app := fiber.New()
	app.Post("/link", MagicLinkRateLimit(store), func(c *fiber.Ctx) error {
		IncrementRateLimitOnFailure(c)
		return c.SendStatus(fiber.StatusOK)
	})
	for i := 1; i <= 30; i++ {
		if resp := statusFrom(t, app, "POST", "/link", "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i, resp.StatusCode)
		}
		mr.FastForward(14 * time.Minute)
	}
}

// The limiters built for the endpoints that start a step of a sign-in and for
// those that redeem an emailed token have ceilings an office behind one
// address stays far below.
func TestFlowAndRedemptionLimitsAreGenerous(t *testing.T) {
	store, _ := newTestRedisClock(t)
	for name, tc := range map[string]struct {
		limit fiber.Handler
		sent  int
	}{
		"flow start": {FlowStartRateLimit(store), 120},
		"redeem":     {TokenRedemptionRateLimit(store), 60},
	} {
		app := fiber.New()
		app.Get("/x", tc.limit, func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
		for i := 1; i <= tc.sent; i++ {
			if resp := statusFrom(t, app, "GET", "/x", "203.0.113.9"); resp.StatusCode != fiber.StatusOK {
				t.Fatalf("%s: request %d: status %d, want 200", name, i, resp.StatusCode)
			}
		}
		if resp := statusFrom(t, app, "GET", "/x", "203.0.113.9"); resp.StatusCode != fiber.StatusTooManyRequests {
			t.Fatalf("%s: request %d: status %d, want 429", name, tc.sent+1, resp.StatusCode)
		}
	}
}
