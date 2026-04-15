package admin

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"authway/apps/central/api/pkg/audit"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// stubAuditService captures sync Log calls so tests can assert that auth
// failures are recorded (regression guard against silent 401/503 paths).
type stubAuditService struct {
	mu      sync.Mutex
	entries []*audit.AuditEntry
}

func (s *stubAuditService) Log(ctx context.Context, entry *audit.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}
func (s *stubAuditService) LogAsync(entry *audit.AuditEntry) { _ = s.Log(context.Background(), entry) }
func (s *stubAuditService) Query(q *audit.AuditLogQuery) ([]audit.AuditLog, int64, error) {
	return nil, 0, nil
}
func (s *stubAuditService) GetByID(id uuid.UUID) (*audit.AuditLog, error)       { return nil, nil }
func (s *stubAuditService) GetUserActivity(tenantID, userID uuid.UUID, limit int) ([]audit.AuditLog, error) {
	return nil, nil
}
func (s *stubAuditService) GetRecentSecurityEvents(tenantID uuid.UUID, hours int) ([]audit.AuditLog, error) {
	return nil, nil
}
func (s *stubAuditService) PurgeOldLogs(tenantID uuid.UUID, retentionDays int) (int64, error) {
	return 0, nil
}

func (s *stubAuditService) snapshot() []*audit.AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*audit.AuditEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

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
	return NewHandler(sess, zap.NewNop(), "test", apiKey, nil)
}

// newTestHandlerWithAudit builds a Handler wired to a stubAuditService so
// tests can assert on audit entries emitted during auth failure paths.
func newTestHandlerWithAudit(apiKey string, sess *stubService, aud audit.Service) *Handler {
	if sess == nil {
		sess = &stubService{validTokens: map[string]bool{}}
	}
	return NewHandler(sess, zap.NewNop(), "test", apiKey, aud)
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

// TestAdminConsoleAuth_LogsAuditOnFailure ensures every auth-failure path
// (503 no-key, 401 no-token, 401 invalid-token) writes a sync audit entry.
// Failures must be durable — LogAsync is unacceptable here because buffer
// drops would blind incident response. This is the regression guard for the
// "prod admin API had zero audit rows during the 2026-04-15 incident" bug.
func TestAdminConsoleAuth_LogsAuditOnFailure(t *testing.T) {
	cases := []struct {
		name         string
		apiKey       string
		authHeader   string
		wantStatus   int
		wantReason   string
		tenantHeader string
	}{
		{"no_api_key_503", "", "Bearer anything", fiber.StatusServiceUnavailable, "api_key_not_configured", ""},
		{"no_token_401", "valid-key", "", fiber.StatusUnauthorized, "no_token", "tenant-abc"},
		{"invalid_token_401", "valid-key", "Bearer wrong", fiber.StatusUnauthorized, "invalid_or_expired_session", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := &stubAuditService{}
			h := newTestHandlerWithAudit(tc.apiKey, nil, aud)
			app := newTestApp(h)

			req := httptest.NewRequest("GET", "/protected", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.tenantHeader != "" {
				req.Header.Set("X-Tenant-ID", tc.tenantHeader)
			}
			resp, _ := app.Test(req)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status: want %d, got %d", tc.wantStatus, resp.StatusCode)
			}

			entries := aud.snapshot()
			if len(entries) != 1 {
				t.Fatalf("audit entries: want 1, got %d", len(entries))
			}
			e := entries[0]
			if e.Success {
				t.Errorf("entry.Success: want false, got true")
			}
			if e.Action != audit.ActionUserLoginFailed {
				t.Errorf("entry.Action: want %q, got %q", audit.ActionUserLoginFailed, e.Action)
			}
			if got, _ := e.Details["reason"].(string); got != tc.wantReason {
				t.Errorf("reason: want %q, got %q", tc.wantReason, got)
			}
			if tc.tenantHeader != "" {
				if got, _ := e.Details["tenant_id_attempted"].(string); got != tc.tenantHeader {
					t.Errorf("tenant_id_attempted: want %q, got %q", tc.tenantHeader, got)
				}
			}
		})
	}
}
