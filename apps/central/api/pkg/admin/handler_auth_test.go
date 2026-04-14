package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// stubService is a minimal Service implementation for middleware tests.
// Only ValidateToken is consulted by GetAdminConsoleAuth.
type stubService struct {
	validTokens map[string]bool
	err         error
}

func (s *stubService) Authenticate(password string) (*AdminSession, error) { return nil, nil }
func (s *stubService) Logout(token string) error                           { return nil }
func (s *stubService) CleanupExpiredSessions() error                       { return nil }

func (s *stubService) ValidateToken(token string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.validTokens[token], nil
}

func newTestHandler(apiKey string, sess *stubService) *Handler {
	if sess == nil {
		sess = &stubService{validTokens: map[string]bool{}}
	}
	return NewHandler(sess, zap.NewNop(), "test", apiKey)
}

func newTestApp(h *Handler) *fiber.App {
	app := fiber.New()
	app.Get("/protected", h.GetAdminConsoleAuth(), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"auth_method": c.Locals("auth_method"),
		})
	})
	return app
}

// TestAdminConsoleAuth_FailClosed_NoAPIKey is the central regression guard
// for the pre-0.2.1 silent bypass. When apiKey is empty, every request
// — including ones with seemingly valid bearer tokens — must be rejected.
func TestAdminConsoleAuth_FailClosed_NoAPIKey(t *testing.T) {
	h := newTestHandler("", &stubService{validTokens: map[string]bool{"sess": true}})
	app := newTestApp(h)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer sess")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestAdminConsoleAuth_AcceptsAPIKey(t *testing.T) {
	h := newTestHandler("api-key-xyz", nil)
	app := newTestApp(h)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer api-key-xyz")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestAdminConsoleAuth_AcceptsValidSessionToken(t *testing.T) {
	sess := &stubService{validTokens: map[string]bool{"sess-abc": true}}
	h := newTestHandler("api-key-xyz", sess)
	app := newTestApp(h)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer sess-abc")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("want 200 (session), got %d", resp.StatusCode)
	}
}

func TestAdminConsoleAuth_RejectsInvalidToken(t *testing.T) {
	sess := &stubService{validTokens: map[string]bool{"sess-abc": true}}
	h := newTestHandler("api-key-xyz", sess)
	app := newTestApp(h)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-thing")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAdminConsoleAuth_NoAuthHeader(t *testing.T) {
	h := newTestHandler("api-key-xyz", nil)
	app := newTestApp(h)

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}
