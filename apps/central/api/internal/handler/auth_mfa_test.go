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
	"golang.org/x/crypto/bcrypt"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/user"
)

// newTestHydraServer stands in for Hydra's admin API — just enough of
// GetLoginRequest/AcceptLoginRequest for AuthHandler.Login and completeLogin
// to run end to end. acceptCount lets a test assert whether the login was
// actually accepted (it must NOT be, while MFA is pending).
func newTestHydraServer(t *testing.T) (client *hydra.Client, acceptCount *int) {
	t.Helper()
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/requests/login"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(hydra.LoginRequest{Challenge: r.URL.Query().Get("challenge")})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/requests/login/accept"):
			count++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(hydra.LoginResponse{RedirectTo: "https://example.com/callback"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/requests/login/reject"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(hydra.LoginResponse{RedirectTo: "https://example.com/error"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL), &count
}

func buildTestUser(t *testing.T, password string, totpEnabled bool) *user.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return &user.User{
		ID:           uuid.New(),
		TenantID:     uuid.New(),
		Email:        "user@example.com",
		PasswordHash: string(hash),
		TOTPEnabled:  totpEnabled,
	}
}

func newAuthTestApp(t *testing.T, password string, totpEnabled bool, totpCode, recoveryCode string) (*fiber.App, *AuthHandler, *int) {
	t.Helper()
	u := buildTestUser(t, password, totpEnabled)
	users := newFakeUserService(u)
	hydraClient, acceptCount := newTestHydraServer(t)

	h := NewAuthHandler(users, nil, fakeClaimsService{}, &fakeMFAService{validTOTPCode: totpCode, validRecoveryCode: recoveryCode}, hydraClient, zap.NewNop(), nil)

	app := fiber.New()
	app.Post("/authenticate", h.Login)
	app.Post("/mfa/verify", h.VerifyMFALogin)
	app.Post("/mfa/recovery", h.VerifyMFARecoveryLogin)
	return app, h, acceptCount
}

func doJSON(t *testing.T, app *fiber.App, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, out
}

func TestLogin_NoMFA_AcceptsImmediately(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", false, "", "")

	status, body := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["redirect_to"] != "https://example.com/callback" {
		t.Errorf("redirect_to = %v, want callback URL", body["redirect_to"])
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}
}

// TestLogin_MFAEnabled_DoesNotAcceptYet is the regression this cycle exists
// for: a TOTP-enabled user must NOT reach Hydra's accept endpoint on
// password alone (HANDOFF.md "MFA 로그인 강제 배선" / ISSUE-...-security-
// controls-not-wired.md item A).
func TestLogin_MFAEnabled_DoesNotAcceptYet(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	status, body := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["mfa_required"] != true {
		t.Errorf("mfa_required = %v, want true", body["mfa_required"])
	}
	challenge, _ := body["mfa_challenge"].(string)
	if challenge == "" {
		t.Fatal("mfa_challenge missing from response")
	}
	if body["redirect_to"] != nil {
		t.Errorf("redirect_to = %v, want absent — login must not be accepted before MFA", body["redirect_to"])
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0 (MFA still pending)", *acceptCount)
	}
}

func TestLogin_WrongPassword_NeverReachesMFABranch(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	status, body := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"wrong"}`)
	if status != fiber.StatusOK { // handler always 200s with an error field for this path, matching prior behavior
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["mfa_required"] != nil {
		t.Errorf("mfa_required = %v, want absent — password never verified", body["mfa_required"])
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

func TestVerifyMFALogin_CompletesLoginOnCorrectCode(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/mfa/verify", `{"challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["redirect_to"] != "https://example.com/callback" {
		t.Errorf("redirect_to = %v, want callback URL", body["redirect_to"])
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}

	// One-time use: replaying the same challenge (even with the right code)
	// must fail now that it has been consumed.
	status, body = doJSON(t, app, "/mfa/verify", `{"challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("replay status = %d, want 400, body = %v", status, body)
	}
}

func TestVerifyMFALogin_WrongCodeDoesNotAccept(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/mfa/verify", `{"challenge":"`+challenge+`","code":"000000"}`)
	if status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401, body = %v", status, body)
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

func TestVerifyMFALogin_LocksAfterMaxAttempts(t *testing.T) {
	app, _, _ := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	var lastStatus int
	var lastBody map[string]any
	for range maxMFAAttempts {
		lastStatus, lastBody = doJSON(t, app, "/mfa/verify", `{"challenge":"`+challenge+`","code":"000000"}`)
	}
	if lastStatus != fiber.StatusUnauthorized || lastBody["error"] != "too many failed attempts — please sign in again" {
		t.Fatalf("after %d attempts: status=%d body=%v", maxMFAAttempts, lastStatus, lastBody)
	}

	// The challenge is gone now, even with the right code.
	status, body := doJSON(t, app, "/mfa/verify", `{"challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("after lockout status = %d, want 400, body = %v", status, body)
	}
}

func TestVerifyMFARecoveryLogin_CompletesLoginOnCorrectCode(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "AAAA-BBBB-CCCC")

	_, loginBody := doJSON(t, app, "/authenticate", `{"challenge":"c1","email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/mfa/recovery", `{"challenge":"`+challenge+`","code":"AAAA-BBBB-CCCC"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["redirect_to"] != "https://example.com/callback" {
		t.Errorf("redirect_to = %v, want callback URL", body["redirect_to"])
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}
}

func TestVerifyMFALogin_UnknownChallenge(t *testing.T) {
	app, _, _ := newAuthTestApp(t, "correct-horse", true, "123456", "")

	status, body := doJSON(t, app, "/mfa/verify", `{"challenge":"`+uuid.NewString()+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %v", status, body)
	}
}
