package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/mfa"
)

// Regression for ISSUE-Authway-20260817-131800: every MFA handler asserted
// c.Locals("user_id") to a string, but JWTAuth (internal/middleware/jwt.go)
// stores a uuid.UUID — every authenticated call panicked into a 500. This
// wires each route behind a stand-in for JWTAuth (same Locals type, same
// key) and asserts the real HTTP status, so a reintroduced string assertion
// fails the build instead of only failing at runtime.
func TestMFAHandlers_AcceptUUIDLocals(t *testing.T) {
	h := NewMFAHandler(&fakeMFAService{
		status:            &mfa.MFAStatusResponse{Enabled: true},
		validTOTPCode:     "000000",
		validRecoveryCode: "AAAA-BBBB-CCCC",
	}, nil, zap.NewNop(), nil)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	})
	app.Post("/setup", h.SetupMFA)
	app.Post("/verify", h.VerifyMFA)
	app.Delete("/disable", h.DisableMFA)
	app.Get("/status", h.GetMFAStatus)
	app.Post("/recovery", h.VerifyRecoveryCode)
	app.Post("/recovery/regenerate", h.RegenerateRecoveryCodes)

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"SetupMFA", "POST", "/setup", "", fiber.StatusOK},
		{"VerifyMFA", "POST", "/verify", `{"code":"000000"}`, fiber.StatusOK},
		{"DisableMFA", "DELETE", "/disable", "", fiber.StatusOK},
		{"GetMFAStatus", "GET", "/status", "", fiber.StatusOK},
		{"VerifyRecoveryCode", "POST", "/recovery", `{"code":"AAAA-BBBB-CCCC"}`, fiber.StatusOK},
		{"RegenerateRecoveryCodes", "POST", "/recovery/regenerate", "", fiber.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(r)
			if err != nil {
				t.Fatalf("app.Test panicked/errored (the exact failure mode of the original bug): %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

// TestMFAHandlers_MissingLocals confirms the guard still 401s cleanly when
// no auth middleware ran at all (Locals unset) — the fixed code path must
// not regress this pre-existing behavior.
func TestMFAHandlers_MissingLocals(t *testing.T) {
	h := NewMFAHandler(&fakeMFAService{}, nil, zap.NewNop(), nil)
	app := fiber.New()
	app.Get("/status", h.GetMFAStatus)

	resp, err := app.Test(httptest.NewRequest("GET", "/status", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}
