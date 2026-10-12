package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// A user's token says who they are; it is not proof of their second factor.
// With the factor on, turning it off or replacing the recovery codes needs a
// code, and a new setup is refused outright.
func TestMFAHandlers_ChangingAFactorThatIsOnNeedsProof(t *testing.T) {
	h := NewMFAHandler(&fakeMFAService{enabled: true, validTOTPCode: "123456", validRecoveryCode: "AAAA-BBBB-CCCC"}, nil, zap.NewNop(), nil, &fakeSessions{})
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/users/mfa/setup", h.SetupMFA)
	app.Delete("/users/mfa", h.DisableMFA)
	app.Post("/users/mfa/recovery/regenerate", h.RegenerateRecoveryCodes)

	call := func(method, path, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var answer struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&answer)
		return resp.StatusCode, answer.Code
	}

	if status, code := call(http.MethodPost, "/users/mfa/setup", ""); status != http.StatusConflict || code != "mfa_already_enabled" {
		t.Errorf("setup while on: %d %q, want 409 mfa_already_enabled", status, code)
	}
	for _, path := range []string{"DELETE /users/mfa", "POST /users/mfa/recovery/regenerate"} {
		method, target, _ := strings.Cut(path, " ")
		for _, body := range []string{"", `{}`, `{"code":"000000"}`} {
			if status, code := call(method, target, body); status != http.StatusForbidden || code != "mfa_code_required" {
				t.Errorf("%s with body %q: %d %q, want 403 mfa_code_required", path, body, status, code)
			}
		}
		for _, body := range []string{`{"code":"123456"}`, `{"code":" AAAA-BBBB-CCCC "}`} {
			if status, _ := call(method, target, body); status != http.StatusOK {
				t.Errorf("%s with body %q: %d, want 200", path, body, status)
			}
		}
	}
}
