package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// newTestRedisClient spins up an in-process miniredis instance so
// MFAChallengeStore's real Redis-backed behavior (HSET/TTL/the atomic
// RecordFailure script) runs in these tests, not just a mock.
func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

const testClientID = "test-client"

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
			json.NewEncoder(w).Encode(hydra.LoginRequest{
				Challenge: r.URL.Query().Get("challenge"),
				Client:    &hydra.OAuth2Client{ClientID: testClientID},
			})
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
	clients := newFakeClientService(&client.Client{ID: uuid.New(), TenantID: u.TenantID, ClientID: testClientID, AllowEmailLogin: true})
	hydraClient, acceptCount := newTestHydraServer(t)

	h := NewAuthHandler(users, clients, fakeClaimsService{}, &fakeMFAService{validTOTPCode: totpCode, validRecoveryCode: recoveryCode}, hydraClient, zap.NewNop(), nil, newTestRedisClient(t))

	app := fiber.New()
	app.Post("/login-flows/:flow/password", h.SubmitPassword)
	app.Post("/login-flows/:flow/mfa", h.VerifyMFALogin)
	app.Post("/login-flows/:flow/mfa/recovery", h.VerifyMFARecoveryLogin)
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

	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
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

	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["next"] != "mfa" {
		t.Errorf("next = %v, want mfa", body["next"])
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

	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"wrong"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["redirect_to"] != nil {
		t.Errorf("redirect_to = %v, want absent — a wrong password must not end the login flow", body["redirect_to"])
	}
	if body["mfa_challenge"] != nil {
		t.Errorf("mfa_challenge = %v, want absent — password never verified", body["mfa_challenge"])
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

func TestVerifyMFALogin_CompletesLoginOnCorrectCode(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
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
	status, body = doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("replay status = %d, want 400, body = %v", status, body)
	}
}

func TestVerifyMFALogin_WrongCodeDoesNotAccept(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"000000"}`)
	if status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401, body = %v", status, body)
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

func TestVerifyMFALogin_LocksAfterMaxAttempts(t *testing.T) {
	app, _, _ := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	var lastStatus int
	var lastBody map[string]any
	for range maxMFAAttempts {
		lastStatus, lastBody = doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"000000"}`)
	}
	if lastStatus != fiber.StatusUnauthorized || lastBody["error"] != "too many failed attempts — please sign in again" {
		t.Fatalf("after %d attempts: status=%d body=%v", maxMFAAttempts, lastStatus, lastBody)
	}

	// The challenge is gone now, even with the right code.
	status, body := doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("after lockout status = %d, want 400, body = %v", status, body)
	}
}

func TestVerifyMFARecoveryLogin_CompletesLoginOnCorrectCode(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "AAAA-BBBB-CCCC")

	_, loginBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/login-flows/c1/mfa/recovery", `{"mfa_challenge":"`+challenge+`","code":"AAAA-BBBB-CCCC"}`)
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

// TestLogin_TenantScoped_SameEmailDifferentTenant is the regression this
// cycle exists for (ISSUE-Authway-20260817-115815): the schema explicitly
// allows the same email in more than one tenant
// (idx_users_tenant_email), so Login must authenticate against the
// requesting OAuth client's tenant, not match the email globally.
func TestLogin_TenantScoped_SameEmailDifferentTenant(t *testing.T) {
	otherTenantHash, err := bcrypt.GenerateFromPassword([]byte("other-tenant-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	rightUser := buildTestUser(t, "correct-horse", false)
	wrongTenantUser := &user.User{
		ID:           uuid.New(),
		TenantID:     uuid.New(), // deliberately different tenant, same email
		Email:        rightUser.Email,
		PasswordHash: string(otherTenantHash),
	}

	users := newFakeUserService(rightUser, wrongTenantUser)
	clients := newFakeClientService(&client.Client{ID: uuid.New(), TenantID: rightUser.TenantID, ClientID: testClientID, AllowEmailLogin: true})
	hydraClient, acceptCount := newTestHydraServer(t)
	h := NewAuthHandler(users, clients, fakeClaimsService{}, &fakeMFAService{}, hydraClient, zap.NewNop(), nil, newTestRedisClient(t))
	app := fiber.New()
	app.Post("/login-flows/:flow/password", h.SubmitPassword)

	// The other tenant's password must NOT authenticate this login — if
	// Login matched by email alone (GetByEmail, deprecated), an
	// undefined-order global lookup could authenticate against either row.
	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"`+rightUser.Email+`","password":"other-tenant-password"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["error"] != "Invalid email or password" {
		t.Errorf("error = %v, want rejection — wrong-tenant password must not authenticate", body["error"])
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}

	// The requesting client's own tenant's password succeeds.
	status, body = doJSON(t, app, "/login-flows/c1/password", `{"email":"`+rightUser.Email+`","password":"correct-horse"}`)
	if status != fiber.StatusOK || body["redirect_to"] != "https://example.com/callback" {
		t.Fatalf("status = %d, body = %v", status, body)
	}
}

func TestVerifyMFALogin_UnknownChallenge(t *testing.T) {
	app, _, _ := newAuthTestApp(t, "correct-horse", true, "123456", "")

	status, body := doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+uuid.NewString()+`","code":"123456"}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %v", status, body)
	}
}

// The mfa_challenge proves the password step of one flow; quoting it on
// another flow must not complete that other flow.
func TestVerifyMFALogin_ChallengeIsBoundToItsFlow(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", true, "123456", "")

	_, loginBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	challenge := loginBody["mfa_challenge"].(string)

	status, body := doJSON(t, app, "/login-flows/other-flow/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_mfa_challenge" {
		t.Fatalf("status = %d, body = %v, want 400 invalid_mfa_challenge", status, body)
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

func TestSubmitPassword_RefusedWhenClientDisablesPasswordSignIn(t *testing.T) {
	u := buildTestUser(t, "correct-horse", false)
	clients := newFakeClientService(&client.Client{ID: uuid.New(), TenantID: u.TenantID, ClientID: testClientID,
		EnabledAuthProviders: []string{"google"}, AllowEmailLogin: true})
	hydraClient, acceptCount := newTestHydraServer(t)
	h := NewAuthHandler(newFakeUserService(u), clients, fakeClaimsService{}, &fakeMFAService{}, hydraClient, zap.NewNop(), nil, newTestRedisClient(t))
	app := fiber.New()
	app.Post("/login-flows/:flow/password", h.SubmitPassword)

	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
	if status != fiber.StatusForbidden || body["code"] != "sign_in_method_not_allowed" {
		t.Fatalf("status = %d, body = %v, want 403 sign_in_method_not_allowed", status, body)
	}
	if *acceptCount != 0 {
		t.Errorf("hydra accept called %d times, want 0", *acceptCount)
	}
}

// Hydra challenges end in "=", which the login screen percent-encodes in
// the path; the handler must look up the decoded id.
func TestSubmitPassword_DecodesTheFlowID(t *testing.T) {
	app, _, acceptCount := newAuthTestApp(t, "correct-horse", false, "", "")

	status, body := doJSON(t, app, "/login-flows/abc%3D%3D/password", `{"email":"user@example.com","password":"correct-horse"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}
}
