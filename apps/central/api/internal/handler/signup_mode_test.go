package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// The sign-in screen says who can sign in when someone cannot, so the login
// flow carries how the tenant takes people in. A tenant that was never asked
// is invite-only, and the answer is the tenant's, not any address's.
func TestGetLoginFlow_CarriesTheTenantsSignupMode(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: true}
	c := &client.Client{ID: uuid.New(), TenantID: tenantID, ClientID: testClientID, Active: true, EnabledAuthProviders: []string{"email"}}

	for _, tc := range []struct {
		name string
		wire func(*AuthHandler)
		want string
	}{
		{"not announced answers the default of every tenant", func(*AuthHandler) {}, tenant.SignupModeInviteOnly},
		{"the tenant's own", func(h *AuthHandler) {
			h.AnnouncesSignupMode(func(id uuid.UUID) string {
				if id != tenantID {
					return tenant.SignupModeInviteOnly
				}
				return tenant.SignupModeOpen
			})
		}, tenant.SignupModeOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &loginFlowHydra{}
			authHandler := NewAuthHandler(newFakeUserService(u), newFakeClientService(c), fakeClaimsService{}, &fakeMFAService{}, h.client(t), zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
			tc.wire(authHandler)
			app := fiber.New()
			app.Get("/api/v1/login-flows/:flow", authHandler.GetLoginFlow)

			status, body := getJSON(t, app, "/api/v1/login-flows/abc-123")
			if status != fiber.StatusOK {
				t.Fatalf("status = %d, body = %v", status, body)
			}
			if body["signup_mode"] != tc.want {
				t.Errorf("signup_mode = %v, want %s", body["signup_mode"], tc.want)
			}
		})
	}
}

func resetRequest(t *testing.T, app *fiber.App, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/email/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("POST forgot-password: %v", err)
	}
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// A password reset that names an application says how its tenant takes people
// in, and says the same to an address with an account and to one without — it
// is what lets the screen tell someone who gets no mail that the service is
// for invited people, without telling anyone which addresses are registered.
func TestForgotPassword_NamesTheTenantsSignupModeTheSameForEveryAddress(t *testing.T) {
	tenantID := uuid.New()
	// A suspended account is sent nothing, which keeps the repository (and so a
	// database) out of this test; the conformance scenario runs the mailed path.
	known := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "known@example.com", Active: false}
	c := &client.Client{ID: uuid.New(), TenantID: tenantID, ClientID: testClientID, Active: true}
	h := &EmailHandler{userSvc: newFakeUserService(known), clientSvc: newFakeClientService(c), validator: validator.New(), logger: zap.NewNop(), emailSvc: refusingMailer{}}
	h.AnnouncesSignupMode(func(uuid.UUID) string { return tenant.SignupModeInviteOnly })
	app := emailApp(h)

	body := func(address, clientID string) string {
		if clientID == "" {
			return fmt.Sprintf(`{"email":%q}`, address)
		}
		return fmt.Sprintf(`{"email":%q,"client_id":%q}`, address, clientID)
	}

	knownStatus, knownBody := resetRequest(t, app, body(known.Email, testClientID))
	unknownStatus, unknownBody := resetRequest(t, app, body("nobody@example.com", testClientID))
	if knownStatus != 200 || unknownStatus != 200 {
		t.Fatalf("status known=%d unknown=%d, want 200 for both", knownStatus, unknownStatus)
	}
	if knownBody != unknownBody {
		t.Errorf("known address answered %s, unknown address %s — they must match", knownBody, unknownBody)
	}
	var answer struct {
		SignupMode string `json:"signup_mode"`
	}
	if err := json.Unmarshal([]byte(knownBody), &answer); err != nil {
		t.Fatalf("decode %q: %v", knownBody, err)
	}
	if answer.SignupMode != tenant.SignupModeInviteOnly {
		t.Errorf("signup_mode = %q, want %q", answer.SignupMode, tenant.SignupModeInviteOnly)
	}

	// A request for a new verification link says the same.
	req := httptest.NewRequest("POST", "/api/v1/email/send-verification", strings.NewReader(body("nobody@example.com", testClientID)))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("POST send-verification: %v", err)
	}
	verification, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(verification), `"signup_mode":"invite_only"`) {
		t.Errorf("send-verification answered %s, want signup_mode invite_only", verification)
	}

	// No application named: no tenant to speak of, and nothing more is said.
	_, anonymous := resetRequest(t, app, body("nobody@example.com", ""))
	if strings.Contains(anonymous, "signup_mode") {
		t.Errorf("a request that names no application was answered %s, want no signup_mode", anonymous)
	}
}
