package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// A sign-in screen stays up while the browser follows the redirect, so the same
// flow is submitted again. The second submit is the same sign-in: it is
// answered the same way and not recorded again.

// recordingAudit keeps what is logged; the rest of the service is not used.
type recordingAudit struct {
	audit.Service
	mu      sync.Mutex
	entries []*audit.AuditEntry
}

func (r *recordingAudit) LogAsync(e *audit.AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *recordingAudit) Log(_ context.Context, e *audit.AuditEntry) error {
	r.LogAsync(e)
	return nil
}

func (r *recordingAudit) count(action audit.AuditAction) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.entries {
		if e.Action == action {
			n++
		}
	}
	return n
}

// newReplayTestApp is newAuthTestApp with the audit log recorded and any number
// of users in the one tenant.
func newReplayTestApp(t *testing.T, totpCode string, users ...*user.User) (*fiber.App, *int, *recordingAudit, *fakeUserService) {
	t.Helper()
	fakeUsers := newFakeUserService(users...)
	clients := newFakeClientService(&client.Client{Active: true, ID: uuid.New(), TenantID: users[0].TenantID, ClientID: testClientID})
	hydraClient, acceptCount := newTestHydraServer(t)
	log := &recordingAudit{}

	h := NewAuthHandler(fakeUsers, clients, fakeClaimsService{}, &fakeMFAService{validTOTPCode: totpCode}, hydraClient, zap.NewNop(), log, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
	app := fiber.New()
	app.Post("/login-flows/:flow/password", h.SubmitPassword)
	app.Post("/login-flows/:flow/mfa", h.VerifyMFALogin)
	return app, acceptCount, log, fakeUsers
}

func TestLogin_SubmittedAgainAfterItSucceeded_IsTheSameSignIn(t *testing.T) {
	u := buildTestUser(t, "correct-horse", false)
	app, acceptCount, log, users := newReplayTestApp(t, "", u)
	const login = `{"email":"user@example.com","password":"correct-horse"}`

	var redirects []any
	for i := 0; i < 3; i++ {
		status, body := doJSON(t, app, "/login-flows/c1/password", login)
		if status != fiber.StatusOK || body["next"] != "redirect" {
			t.Fatalf("submit %d: status %d body %v, want the redirect again", i+1, status, body)
		}
		redirects = append(redirects, body["redirect_to"])
	}
	if redirects[0] == nil || redirects[0] != redirects[1] || redirects[1] != redirects[2] {
		t.Errorf("redirects = %v, want the same one each time", redirects)
	}
	if *acceptCount != 1 {
		t.Errorf("the authorization server accepted the flow %d times, want 1", *acceptCount)
	}
	if got := log.count(audit.ActionUserLogin); got != 1 {
		t.Errorf("user.login recorded %d times for one flow, want 1", got)
	}
	if users.signIns != 1 {
		t.Errorf("last sign-in updated %d times, want 1", users.signIns)
	}

	// Another flow of the same user is another sign-in.
	if status, body := doJSON(t, app, "/login-flows/c2/password", login); status != fiber.StatusOK {
		t.Fatalf("second flow: status %d body %v", status, body)
	}
	if got := log.count(audit.ActionUserLogin); got != 2 {
		t.Errorf("user.login recorded %d times for two flows, want 2", got)
	}
}

// Asking again proves who you are again: the repeat is not a way to a
// redirect with a wrong password, and a wrong password is a failure like any.
func TestLogin_SubmittedAgainWithAWrongPassword_IsRefused(t *testing.T) {
	u := buildTestUser(t, "correct-horse", false)
	app, acceptCount, _, _ := newReplayTestApp(t, "", u)

	if status, _ := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`); status != fiber.StatusOK {
		t.Fatalf("first sign-in: status %d", status)
	}
	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"wrong"}`)
	if status != fiber.StatusUnauthorized || body["redirect_to"] != nil {
		t.Errorf("wrong password on a signed-in flow: status %d body %v, want 401 and no redirect", status, body)
	}
	if *acceptCount != 1 {
		t.Errorf("accepted %d times, want 1", *acceptCount)
	}
}

// A flow belongs to the user who signed in to it.
func TestLogin_AnotherUserOnASignedInFlow_IsRefused(t *testing.T) {
	first := buildTestUser(t, "correct-horse", false)
	hash, err := bcrypt.GenerateFromPassword([]byte("other-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	second := &user.User{ID: uuid.New(), TenantID: first.TenantID, Email: "second@example.com", PasswordHash: string(hash), Active: true}
	app, acceptCount, log, _ := newReplayTestApp(t, "", first, second)

	if status, _ := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`); status != fiber.StatusOK {
		t.Fatalf("first sign-in: status %d", status)
	}
	// The password is not checked: right or wrong, the answer is the one a wrong password gets.
	status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"second@example.com","password":"other-horse"}`)
	wrongStatus, wrongBody := doJSON(t, app, "/login-flows/c1/password", `{"email":"second@example.com","password":"wrong"}`)
	if status != fiber.StatusUnauthorized || body["code"] != "invalid_credentials" || wrongStatus != status || wrongBody["code"] != body["code"] || wrongBody["detail"] != body["detail"] {
		t.Errorf("another user on the flow: status %d body %v; a wrong password gets %d %v — want the same 401", status, body, wrongStatus, wrongBody)
	}
	if *acceptCount != 1 || log.count(audit.ActionUserLogin) != 1 {
		t.Errorf("accepted %d, recorded %d, want one of each — the first user's", *acceptCount, log.count(audit.ActionUserLogin))
	}
}

// A repeat of the first factor does not skip the second: it asks for the
// second factor again, and that is answered with the first answer.
func TestLogin_SecondFactorUser_RepeatStillOwesTheSecondFactor(t *testing.T) {
	u := buildTestUser(t, "correct-horse", true)
	app, acceptCount, log, _ := newReplayTestApp(t, "123456", u)
	const login = `{"email":"user@example.com","password":"correct-horse"}`

	var redirects []any
	for i := 0; i < 2; i++ {
		status, body := doJSON(t, app, "/login-flows/c1/password", login)
		if status != fiber.StatusOK || body["next"] != "mfa" {
			t.Fatalf("round %d, password: status %d body %v, want the second factor owed", i+1, status, body)
		}
		challenge, _ := body["mfa_challenge"].(string)
		status, body = doJSON(t, app, "/login-flows/c1/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
		if status != fiber.StatusOK || body["next"] != "redirect" {
			t.Fatalf("round %d, second factor: status %d body %v", i+1, status, body)
		}
		redirects = append(redirects, body["redirect_to"])
	}
	if redirects[0] == nil || redirects[0] != redirects[1] {
		t.Errorf("redirects = %v, want the same one", redirects)
	}
	if *acceptCount != 1 || log.count(audit.ActionUserLogin) != 1 {
		t.Errorf("accepted %d, recorded %d, want one of each", *acceptCount, log.count(audit.ActionUserLogin))
	}
}

// Submits of one flow that overlap are one sign-in: the second waits for the
// first's answer instead of having the flow accepted again.
func TestLogin_SubmittedAtOnce_IsOneSignIn(t *testing.T) {
	var accepted atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/requests/login"):
			_ = json.NewEncoder(w).Encode(hydra.LoginRequest{Challenge: r.URL.Query().Get("challenge"), Client: &hydra.OAuth2Client{ClientID: testClientID}})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/requests/login/accept"):
			n := accepted.Add(1)
			time.Sleep(150 * time.Millisecond) // the authorization server takes its time
			_ = json.NewEncoder(w).Encode(hydra.LoginResponse{RedirectTo: fmt.Sprintf("https://example.com/callback?verifier=%d", n)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	u := buildTestUser(t, "correct-horse", false)
	clients := newFakeClientService(&client.Client{Active: true, ID: uuid.New(), TenantID: u.TenantID, ClientID: testClientID})
	log := &recordingAudit{}
	h := NewAuthHandler(newFakeUserService(u), clients, fakeClaimsService{}, &fakeMFAService{}, hydra.NewClient(srv.URL), zap.NewNop(), log, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
	app := fiber.New()
	app.Post("/login-flows/:flow/password", h.SubmitPassword)

	const submits = 6
	redirects := make([]any, submits)
	statuses := make([]int, submits)
	var wg sync.WaitGroup
	for i := 0; i < submits; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
			statuses[i], redirects[i] = status, body["redirect_to"]
		}()
	}
	wg.Wait()

	for i := 0; i < submits; i++ {
		if statuses[i] != fiber.StatusOK || redirects[i] != redirects[0] || redirects[i] == nil {
			t.Errorf("submit %d: status %d redirect %v, want 200 and the first's redirect %v", i, statuses[i], redirects[i], redirects[0])
		}
	}
	if got := accepted.Load(); got != 1 {
		t.Errorf("the authorization server accepted the flow %d times, want 1", got)
	}
	if got := log.count(audit.ActionUserLogin); got != 1 {
		t.Errorf("user.login recorded %d times, want 1", got)
	}
}
