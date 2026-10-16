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

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// loginFlowHydra stands in for Hydra's admin API for GetLoginFlow. It records
// the challenge Hydra was asked about, so a test can check what the handler
// actually forwarded, and answers with the given skip/subject.
type loginFlowHydra struct {
	skip         bool
	subject      string
	gotChallenge string
	acceptCount  int
}

func (f *loginFlowHydra) client(t *testing.T) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/login"):
			f.gotChallenge = r.URL.Query().Get("login_challenge")
			if f.gotChallenge == "" {
				f.gotChallenge = r.URL.Query().Get("challenge")
			}
			json.NewEncoder(w).Encode(hydra.LoginRequest{
				Challenge:      f.gotChallenge,
				Skip:           f.skip,
				Subject:        f.subject,
				RequestedScope: []string{"openid"},
				Client:         &hydra.OAuth2Client{ClientID: testClientID, ClientName: "Test App", LogoURI: "https://app.example/logo.png"},
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/requests/login/accept"):
			f.acceptCount++
			json.NewEncoder(w).Encode(hydra.LoginResponse{RedirectTo: "https://example.com/callback"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

func newLoginFlowApp(t *testing.T, h *loginFlowHydra, u *user.User, c *client.Client) *fiber.App {
	t.Helper()
	handler := NewAuthHandler(newFakeUserService(u), newFakeClientService(c), fakeClaimsService{}, &fakeMFAService{}, h.client(t), zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
	app := fiber.New()
	app.Get("/api/v1/login-flows/:flow", handler.GetLoginFlow)
	return app
}

func getJSON(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, out
}

func TestGetLoginFlow_ReturnsSignInOptions(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: true}
	c := &client.Client{
		ID: uuid.New(), TenantID: tenantID, ClientID: testClientID, Active: true,
		EnabledAuthProviders: []string{"email", "github"},
		GithubOAuthEnabled:   true,
	}
	h := &loginFlowHydra{}
	app := newLoginFlowApp(t, h, u, c)

	// Hydra challenges are padded base64 and end in "=". The sign-in screen
	// builds the path with encodeURIComponent, which sends "=" as %3D (Go's
	// url.PathEscape would leave it as is, so the escaped form is spelled out).
	const flow = "abc-123=="
	status, body := getJSON(t, app, "/api/v1/login-flows/abc-123%3D%3D")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if h.gotChallenge != flow {
		t.Errorf("Hydra was asked about %q, want the decoded flow %q", h.gotChallenge, flow)
	}
	if body["next"] != "form" || body["flow"] != flow {
		t.Errorf("next/flow = %v/%v, want form/%s", body["next"], body["flow"], flow)
	}
	if body["client_name"] != "Test App" {
		t.Errorf("client_name = %v", body["client_name"])
	}
	if body["client_logo_uri"] != "https://app.example/logo.png" {
		t.Errorf("client_logo_uri = %v, want the client's logo", body["client_logo_uri"])
	}
	cl, _ := body["client"].(map[string]any)
	if cl["client_id"] != testClientID {
		t.Errorf("client.client_id = %v", cl["client_id"])
	}
	methods, _ := cl["sign_in_methods"].([]any)
	if len(methods) != 2 || methods[0] != "email" || methods[1] != "github" {
		t.Errorf("client.sign_in_methods = %v, want [email github]", cl["sign_in_methods"])
	}
	if h.acceptCount != 0 {
		t.Errorf("a flow that needs the form must not be accepted (accepts = %d)", h.acceptCount)
	}
}

func TestGetLoginFlow_SameTenantSessionSkipsTheForm(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: true}
	c := &client.Client{Active: true, ID: uuid.New(), TenantID: tenantID, ClientID: testClientID}
	h := &loginFlowHydra{skip: true, subject: u.ID.String()}
	app := newLoginFlowApp(t, h, u, c)

	status, body := getJSON(t, app, "/api/v1/login-flows/flow-1")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["next"] != "redirect" || body["redirect_to"] != "https://example.com/callback" {
		t.Errorf("next/redirect_to = %v/%v, want redirect/callback", body["next"], body["redirect_to"])
	}
	if h.acceptCount != 1 {
		t.Errorf("accepts = %d, want 1", h.acceptCount)
	}
}

func TestGetLoginFlow_UnregisteredClientIsRejected(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "user@example.com", Active: true}
	other := &client.Client{Active: true, ID: uuid.New(), TenantID: u.TenantID, ClientID: "someone-else"}
	app := newLoginFlowApp(t, &loginFlowHydra{}, u, other)

	status, body := getJSON(t, app, "/api/v1/login-flows/flow-1")
	if status == fiber.StatusOK {
		t.Fatalf("status = 200 for a client Authway does not know, body = %v", body)
	}
}

// What the application's request asked of the sign-in screen reaches the
// screen, which otherwise only has a flow id.
func TestRequestHints(t *testing.T) {
	methods := []string{"email", "google"}
	request := func(query string, oidc *hydra.OIDCContext) *hydra.LoginRequest {
		return &hydra.LoginRequest{RequestURL: "https://issuer.example/oauth2/auth?client_id=a" + query, OIDCContext: oidc}
	}
	for _, tc := range []struct {
		name string
		req  *hydra.LoginRequest
		want map[string]any
	}{
		{"a request that asks nothing", request("", nil), map[string]any{}},
		{"a way of signing in the screen offers", request("&connection=google", nil), map[string]any{"sign_in_method": "google"}},
		{"a way of signing in it does not offer", request("&connection=github", nil), map[string]any{}},
		{"whose address to expect, and the language",
			request("", &hydra.OIDCContext{LoginHint: "user@example.com", UILocales: []string{"ko", "en"}}),
			map[string]any{"login_hint": "user@example.com", "ui_locales": []any{"ko", "en"}}},
		{"an address that cannot be read", &hydra.LoginRequest{RequestURL: "%zz:/"}, map[string]any{}},
	} {
		raw, err := json.Marshal(requestHints(tc.req, methods))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got map[string]any
		_ = json.Unmarshal(raw, &got)
		want, _ := json.Marshal(tc.want)
		if string(mustJSON(t, got)) != string(want) {
			t.Errorf("%s: hints = %s, want %s", tc.name, raw, want)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
