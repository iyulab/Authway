package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
)

// A password is reset with a link mailed to the account's address, so a reset
// proves the address as a verification mail would — and the password it sets
// meets the tenant's rule, whose refusal says what the rule is.
func TestResetPassword_ProvesTheAddressAndHoldsThePasswordToTheRule(t *testing.T) {
	db := handlerPostgres(t)
	suffix := uuid.New().String()[:8]
	tenants := tenant.NewService(db)
	tn, err := tenants.CreateTenant(tenant.CreateTenantRequest{
		Name: "reset-" + suffix, Slug: "reset-" + suffix,
		Settings: tenant.TenantSettings{PasswordMinLength: 12},
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID) })
	users := user.NewService(db, zap.NewNop(), user.PasswordMinLength(tenants.PasswordMinLength))
	u, err := users.Create(tn.ID, &user.CreateUserRequest{Email: "member-" + suffix + "@example.com", Name: "Member", Password: "the-first-password"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	if u.EmailVerified {
		t.Fatal("an account created without proof of its address starts unverified")
	}

	authorizationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(authorizationServer.Close)
	repo := email.NewRepository(db)
	h := &EmailHandler{
		emailRepo: repo, userSvc: users, clientSvc: newFakeClientService(), hydraClient: hydra.NewClient(authorizationServer.URL),
		validator: validator.New(), logger: zap.NewNop(),
	}
	app := emailApp(h)

	reset, err := repo.CreatePasswordReset(u.ID)
	if err != nil {
		t.Fatalf("create reset: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM password_resets WHERE user_id = ?`, u.ID) })
	post := func(password string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/email/reset-password", strings.NewReader(fmt.Sprintf(`{"token":%q,"new_password":%q}`, reset.Token, password)))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("POST reset-password: %v", err)
		}
		raw, _ := io.ReadAll(res.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		return res.StatusCode, body
	}

	// Eleven characters pass the request's own check (8) and not the tenant's.
	status, body := post("elevenchars")
	if status != 400 || body["code"] != "password_too_short" || body["min_length"] != float64(12) {
		t.Fatalf("a password under the tenant's length: %d %v, want 400 password_too_short naming 12", status, body)
	}
	if after, _ := users.GetByID(u.ID); after == nil || after.EmailVerified || !users.VerifyPassword(after, "the-first-password") {
		t.Fatal("a refused reset changed the account")
	}

	// The refusal did not use the link up.
	if status, body := post("twelve-chars"); status != 200 {
		t.Fatalf("reset: %d %v", status, body)
	}
	after, err := users.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !after.EmailVerified {
		t.Error("email_verified is false after a reset by the mailed link")
	}
	if !users.VerifyPassword(after, "twelve-chars") {
		t.Error("the new password does not sign the account in")
	}
}
