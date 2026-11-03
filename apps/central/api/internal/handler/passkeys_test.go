package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/passkey"
	"authway/apps/central/api/pkg/user"
)

// fakePasskeys is the passkey store, in memory. Remove keeps the last-way rule
// as the real one does, for an account without a password and without
// provider accounts.
type fakePasskeys struct {
	rows []user.Passkey
}

func (f *fakePasskeys) List(userID uuid.UUID) ([]user.Passkey, error) {
	var out []user.Passkey
	for _, r := range f.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakePasskeys) Add(p *user.Passkey) error { f.rows = append(f.rows, *p); return nil }
func (f *fakePasskeys) Remove(userID, id uuid.UUID) error {
	mine, at := 0, -1
	for i, r := range f.rows {
		if r.UserID == userID {
			mine++
			if r.ID == id {
				at = i
			}
		}
	}
	switch {
	case at < 0:
		return user.ErrPasskeyNotFound
	case mine == 1:
		return user.ErrLastSignInMethod
	}
	f.rows = append(f.rows[:at], f.rows[at+1:]...)
	return nil
}
func (f *fakePasskeys) Find(tenantID uuid.UUID, credentialID []byte) (*user.Passkey, error) {
	for _, r := range f.rows {
		if r.TenantID == tenantID && bytes.Equal(r.CredentialID, credentialID) {
			return &r, nil
		}
	}
	return nil, user.ErrPasskeyNotFound
}
func (f *fakePasskeys) RecordUse(id uuid.UUID, count uint32, flags byte) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].SignCount, f.rows[i].Flags = int64(count), int16(flags)
		}
	}
	return nil
}

type passkeyFixture struct {
	app    *fiber.App
	keys   *fakePasskeys
	user   *user.User
	issued *int
}

// passkeyApp mounts the passkey endpoints as the server does, behind a stand-in
// for the token check that signs the request in as u with a token issued to
// clientID at authTime. ceremonies is nil where passkeys are not set up.
func passkeyApp(t *testing.T, ceremonies bool, clientID string, authTime time.Time) passkeyFixture {
	t.Helper()
	tenant := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenant, Email: "pat@example.com", Active: true}
	account := &client.Client{ClientID: "account-app", TenantID: tenant, System: true, SystemKind: client.SystemKindAccount, Active: true}
	thirdParty := &client.Client{ClientID: "third-party", TenantID: tenant, Active: true}
	// The console's client is a system client too - not the account screen's.
	console := &client.Client{ClientID: "console-app", TenantID: tenant, System: true, SystemKind: client.SystemKindConsole, Active: true}
	keys := &fakePasskeys{}
	users := newFakeUserService(u)

	var c *passkey.Ceremonies
	if ceremonies {
		var err error
		c, err = passkey.New(passkey.Config{RPID: "auth.example.com", RPName: "Authway", Origins: []string{"https://auth.example.com"}}, ephemeral.NewMemory(context.Background()), keys, users)
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewPasskeyHandler(users, keys, newFakeClientService(account, thirdParty, console), c, func(uuid.UUID) string { return "Acme" }, nil, zap.NewNop())
	app := fiber.New()
	signedIn := func(ctx *fiber.Ctx) error {
		ctx.Locals("user_id", u.ID)
		ctx.Locals("client_id", clientID)
		ctx.Locals("auth_time", authTime)
		return ctx.Next()
	}
	app.Get("/users/passkeys", signedIn, h.List)
	app.Post("/users/passkeys/register/begin", signedIn, h.BeginRegistration)
	app.Post("/users/passkeys/register/finish", signedIn, h.FinishRegistration)
	app.Delete("/users/passkeys/:id", signedIn, h.Remove)
	return passkeyFixture{app: app, keys: keys, user: u}
}

func callPasskeys(t *testing.T, app *fiber.App, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestPasskeys_OnlyTheAccountApplicationManagesThem(t *testing.T) {
	f := passkeyApp(t, true, "third-party", time.Now().Add(-time.Minute))
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/users/passkeys", nil},
		{http.MethodPost, "/users/passkeys/register/begin", nil},
		{http.MethodPost, "/users/passkeys/register/finish", map[string]any{"ceremony_id": "x", "credential": map[string]any{}}},
		{http.MethodDelete, "/users/passkeys/" + uuid.NewString(), nil},
	} {
		status, body := callPasskeys(t, f.app, call.method, call.path, call.body)
		if status != 403 || body["code"] != "account_application_required" {
			t.Errorf("%s %s with another application's token: %d %v, want 403 account_application_required", call.method, call.path, status, body)
		}
	}
}

// A system client is not necessarily the account screen: the console's token,
// held by whoever has it for ten minutes after an operator signs in, must not
// plant a passkey on the operator's account.
func TestPasskeys_TheConsolesTokenIsNotTheAccountScreens(t *testing.T) {
	f := passkeyApp(t, true, "console-app", time.Now().Add(-time.Minute))
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/users/passkeys"},
		{http.MethodPost, "/users/passkeys/register/begin"},
		{http.MethodDelete, "/users/passkeys/" + uuid.NewString()},
	} {
		status, body := callPasskeys(t, f.app, call.method, call.path, nil)
		if status != 403 || body["code"] != "account_application_required" {
			t.Errorf("%s %s with the console's token: %d %v, want 403 account_application_required", call.method, call.path, status, body)
		}
	}
}

func TestPasskeys_AreOffWhereTheyAreNotSetUp(t *testing.T) {
	f := passkeyApp(t, false, "account-app", time.Now().Add(-time.Minute))
	status, body := callPasskeys(t, f.app, http.MethodGet, "/users/passkeys", nil)
	if status != 404 || body["code"] != "passkeys_unavailable" {
		t.Fatalf("%d %v, want 404 passkeys_unavailable", status, body)
	}
}

func TestPasskeys_ChangesNeedARecentSignIn(t *testing.T) {
	f := passkeyApp(t, true, "account-app", time.Now().Add(-time.Hour))
	for _, call := range []struct{ method, path string }{
		{http.MethodPost, "/users/passkeys/register/begin"},
		{http.MethodDelete, "/users/passkeys/" + uuid.NewString()},
	} {
		status, body := callPasskeys(t, f.app, call.method, call.path, nil)
		if status != 401 || body["code"] != "insufficient_user_authentication" {
			t.Errorf("%s %s with a stale sign-in: %d %v, want 401 insufficient_user_authentication", call.method, call.path, status, body)
		}
	}
	// Reading them does not.
	if status, _ := callPasskeys(t, f.app, http.MethodGet, "/users/passkeys", nil); status != 200 {
		t.Errorf("list with a stale sign-in: %d, want 200", status)
	}
}

func TestPasskeys_BeginGivesOptionsAndACeremony(t *testing.T) {
	f := passkeyApp(t, true, "account-app", time.Now().Add(-time.Minute))
	status, body := callPasskeys(t, f.app, http.MethodPost, "/users/passkeys/register/begin", nil)
	if status != 200 {
		t.Fatalf("begin: %d %v", status, body)
	}
	options, _ := body["options"].(map[string]any)
	if body["ceremony_id"] == "" || options["challenge"] == nil || options["rp"] == nil {
		t.Fatalf("begin body = %v, want ceremony_id and the creation options", body)
	}
	// The options are the standard ones, not wrapped.
	if _, wrapped := options["publicKey"]; wrapped {
		t.Error("options are wrapped in publicKey; the contract passes them to parseCreationOptionsFromJSON unwrapped")
	}
	user, _ := options["user"].(map[string]any)
	if user["displayName"] != "pat@example.com · Acme" {
		t.Errorf("user.displayName = %v, want the address and the tenant", user["displayName"])
	}
}

func TestPasskeys_FinishRefusesWhatIsNotAnAnswerToACeremony(t *testing.T) {
	f := passkeyApp(t, true, "account-app", time.Now().Add(-time.Minute))

	if status, body := callPasskeys(t, f.app, http.MethodPost, "/users/passkeys/register/finish", map[string]any{"name": "x"}); status != 400 || body["code"] != "invalid_request" {
		t.Errorf("no ceremony or credential: %d %v, want 400 invalid_request", status, body)
	}
	if status, body := callPasskeys(t, f.app, http.MethodPost, "/users/passkeys/register/finish", map[string]any{"ceremony_id": "never-started", "credential": map[string]any{}}); status != 400 || body["code"] != "ceremony_expired" {
		t.Errorf("unknown ceremony: %d %v, want 400 ceremony_expired", status, body)
	}
	// A real ceremony with an answer that is not a credential.
	_, begun := callPasskeys(t, f.app, http.MethodPost, "/users/passkeys/register/begin", nil)
	status, body := callPasskeys(t, f.app, http.MethodPost, "/users/passkeys/register/finish", map[string]any{"ceremony_id": begun["ceremony_id"], "credential": map[string]any{"id": "x"}})
	if status != 400 || body["code"] != "passkey_rejected" {
		t.Errorf("a bad credential: %d %v, want 400 passkey_rejected", status, body)
	}
	// And the reason is not in the answer.
	if msg, _ := body["detail"].(string); msg == "" || bytes.Contains([]byte(msg), []byte("challenge")) || bytes.Contains([]byte(msg), []byte("origin")) {
		t.Errorf("the refusal says too much: %q", msg)
	}
	if len(f.keys.rows) != 0 {
		t.Errorf("stored %d passkeys, want none", len(f.keys.rows))
	}
}

func TestPasskeys_ListAndRemove_KeepingAWayToSignIn(t *testing.T) {
	f := passkeyApp(t, true, "account-app", time.Now().Add(-time.Minute))
	laptop := user.Passkey{ID: uuid.New(), UserID: f.user.ID, TenantID: f.user.TenantID, CredentialID: []byte("secret-credential-id"), PublicKey: []byte("secret-key"), Name: "Laptop", CreatedAt: time.Now().Add(-time.Hour), Flags: 0x18}
	phone := user.Passkey{ID: uuid.New(), UserID: f.user.ID, TenantID: f.user.TenantID, CredentialID: []byte("other"), PublicKey: []byte("k"), Name: "Phone", CreatedAt: time.Now()}
	f.keys.rows = []user.Passkey{laptop, phone}

	status, body := callPasskeys(t, f.app, http.MethodGet, "/users/passkeys", nil)
	list, _ := body["passkeys"].([]any)
	if status != 200 || len(list) != 2 {
		t.Fatalf("list: %d %v, want both", status, body)
	}
	first := list[0].(map[string]any)
	if first["name"] != "Laptop" || first["synced"] != true || list[1].(map[string]any)["synced"] != false {
		t.Errorf("list entries = %v", list)
	}
	// Neither the credential id nor the key leaves the server.
	raw, _ := json.Marshal(body)
	if bytes.Contains(raw, []byte("secret")) || bytes.Contains(raw, []byte("credential")) || bytes.Contains(raw, []byte("public_key")) {
		t.Errorf("the list shows the credential: %s", raw)
	}

	if status, body := callPasskeys(t, f.app, http.MethodDelete, "/users/passkeys/"+uuid.NewString(), nil); status != 404 {
		t.Errorf("remove of an unknown id: %d %v, want 404", status, body)
	}
	if status, body := callPasskeys(t, f.app, http.MethodDelete, "/users/passkeys/not-an-id", nil); status != 404 {
		t.Errorf("remove of a malformed id: %d %v, want 404", status, body)
	}
	if status, body := callPasskeys(t, f.app, http.MethodDelete, "/users/passkeys/"+phone.ID.String(), nil); status != 200 {
		t.Errorf("remove: %d %v, want 200", status, body)
	}
	status, body = callPasskeys(t, f.app, http.MethodDelete, "/users/passkeys/"+laptop.ID.String(), nil)
	if status != 409 || body["code"] != "last_sign_in_method" {
		t.Errorf("remove of the last way to sign in: %d %v, want 409 last_sign_in_method", status, body)
	}
}
