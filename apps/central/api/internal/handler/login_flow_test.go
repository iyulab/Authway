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
				Client:         &hydra.OAuth2Client{ClientID: testClientID, ClientName: "Test App"},
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
	handler := NewAuthHandler(newFakeUserService(u), newFakeClientService(c), fakeClaimsService{}, &fakeMFAService{}, h.client(t), zap.NewNop(), nil, newTestRedisClient(t))
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
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com"}
	c := &client.Client{
		ID: uuid.New(), TenantID: tenantID, ClientID: testClientID,
		EnabledAuthProviders: []string{"email", "github"},
		AllowEmailSignup:     false,
		AllowEmailLogin:      true,
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
	cl, _ := body["client"].(map[string]any)
	if cl["client_id"] != testClientID {
		t.Errorf("client.client_id = %v", cl["client_id"])
	}
	providers, _ := cl["enabled_auth_providers"].([]any)
	if len(providers) != 2 || providers[0] != "email" || providers[1] != "github" {
		t.Errorf("client.enabled_auth_providers = %v, want [email github]", cl["enabled_auth_providers"])
	}
	if cl["allow_email_signup"] != false || cl["allow_email_login"] != true || cl["github_oauth_enabled"] != true {
		t.Errorf("client sign-in settings = %v", cl)
	}
	if h.acceptCount != 0 {
		t.Errorf("a flow that needs the form must not be accepted (accepts = %d)", h.acceptCount)
	}
}

func TestGetLoginFlow_SameTenantSessionSkipsTheForm(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com"}
	c := &client.Client{ID: uuid.New(), TenantID: tenantID, ClientID: testClientID}
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
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "user@example.com"}
	other := &client.Client{ID: uuid.New(), TenantID: u.TenantID, ClientID: "someone-else"}
	app := newLoginFlowApp(t, &loginFlowHydra{}, u, other)

	status, body := getJSON(t, app, "/api/v1/login-flows/flow-1")
	if status == fiber.StatusOK {
		t.Fatalf("status = 200 for a client Authway does not know, body = %v", body)
	}
}
