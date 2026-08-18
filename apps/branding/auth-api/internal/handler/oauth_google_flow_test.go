package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"authway/apps/branding/auth-api/internal/config"
	"authway/apps/branding/auth-api/internal/service"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// newOAuthTestHandler wires an OAuthHandler to three fake backends (Google's
// token+userinfo endpoints combined behind one server routed by path, Central
// API, and Hydra admin) so the handshake can be driven end-to-end with no
// real network dependency. A nil handler fails the test if that backend is
// ever called — most tests only exercise a subset of the three.
func newOAuthTestHandler(t *testing.T, google, central, hydra http.HandlerFunc) (*OAuthHandler, func()) {
	t.Helper()

	unexpected := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected call to %s backend: %s %s", name, r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	if google == nil {
		google = unexpected("google")
	}
	if central == nil {
		central = unexpected("central API")
	}
	if hydra == nil {
		hydra = unexpected("hydra")
	}

	googleSrv := httptest.NewServer(google)
	centralSrv := httptest.NewServer(central)
	hydraSrv := httptest.NewServer(hydra)
	t.Cleanup(func() {
		googleSrv.Close()
		centralSrv.Close()
		hydraSrv.Close()
	})

	googleCfg := &config.GoogleOAuthConfig{ClientID: "google-client", ClientSecret: "google-secret", RedirectURL: "http://auth.local/callback"}
	googleSvc := service.NewGoogleServiceForTesting(googleCfg, zap.NewNop(),
		"https://accounts.example.com/auth", googleSrv.URL+"/token", googleSrv.URL+"/userinfo")
	centralAPI := service.NewCentralAPIClient(&config.CentralAPIConfig{BaseURL: centralSrv.URL, InternalKey: "internal-key"}, zap.NewNop())
	hydraClient := service.NewHydraClient(&config.HydraConfig{AdminURL: hydraSrv.URL}, zap.NewNop())

	redisClient, _ := newTestRedisClient(t)
	h := NewOAuthHandler(googleSvc, centralAPI, hydraClient, zap.NewNop(), redisClient)
	return h, func() {}
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

// --- GoogleLoginGet ---

func TestGoogleLoginGet_MissingChallenge(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil)
	app := fiber.New()
	app.Get("/login/google", h.GoogleLoginGet)

	resp, err := app.Test(httptest.NewRequest("GET", "/login/google", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGoogleLoginGet_HappyPath(t *testing.T) {
	hydra := jsonHandler(http.StatusOK, `{"challenge":"lc1","client":{"client_id":"client-1","client_name":"Test App"},"requested_scope":["openid","email"]}`)
	central := jsonHandler(http.StatusOK, `{"client":{"client_id":"client-1","name":"Test App","enabled_auth_providers":["email","google"],"allow_email_signup":true,"allow_email_login":true,"google_oauth_enabled":true,"github_oauth_enabled":false,"microsoft_oauth_enabled":false,"apple_oauth_enabled":false}}`)

	h, _ := newOAuthTestHandler(t, nil, central, hydra)
	app := fiber.New()
	app.Get("/login/google", h.GoogleLoginGet)

	req := httptest.NewRequest("GET", "/login/google?login_challenge=lc1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to decode response: %v (body=%s)", err, body)
	}
	if got["client_name"] != "Test App" {
		t.Errorf("client_name = %v, want Test App", got["client_name"])
	}
	client, _ := got["client"].(map[string]any)
	if client["google_oauth_enabled"] != true {
		t.Errorf("client.google_oauth_enabled = %v, want true (from Central API config)", client["google_oauth_enabled"])
	}
}

func TestGoogleLoginGet_CentralAPIUnavailable_FallsBackToDefaults(t *testing.T) {
	hydra := jsonHandler(http.StatusOK, `{"challenge":"lc1","client":{"client_id":"client-1","client_name":"Test App"}}`)
	central := jsonHandler(http.StatusInternalServerError, `{"error":"down"}`)

	h, _ := newOAuthTestHandler(t, nil, central, hydra)
	app := fiber.New()
	app.Get("/login/google", h.GoogleLoginGet)

	req := httptest.NewRequest("GET", "/login/google?login_challenge=lc1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	// Central API being down must degrade to defaults, not fail the request —
	// otherwise a Central API outage takes the login page down with it.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful fallback)", resp.StatusCode)
	}
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &got)
	client, _ := got["client"].(map[string]any)
	if client["google_oauth_enabled"] != true {
		t.Errorf("client.google_oauth_enabled = %v, want default true", client["google_oauth_enabled"])
	}
}

// --- GoogleLogin ---

func TestGoogleLogin_MissingChallenge(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil)
	app := fiber.New()
	app.Post("/login/google", h.GoogleLogin)

	req := httptest.NewRequest("POST", "/login/google", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGoogleLogin_ClientIDProvided_SkipsHydraLookup(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil) // hydra: unexpected — must not be called
	app := fiber.New()
	app.Post("/login/google", h.GoogleLogin)

	req := httptest.NewRequest("POST", "/login/google", strings.NewReader(`{"login_challenge":"lc1","client_id":"client-1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &got)
	state, _ := got["state"].(string)
	if state == "" {
		t.Fatal("expected a non-empty state in the response")
	}
	if _, exists := h.stateStore.Get(state); !exists {
		t.Error("expected the returned state to be stored server-side")
	}
	redirectURL, _ := got["redirect_url"].(string)
	if !strings.Contains(redirectURL, "state="+state) {
		t.Errorf("redirect_url = %q, want it to carry the same state", redirectURL)
	}
}

func TestGoogleLogin_DerivesClientIDFromHydraWhenNotProvided(t *testing.T) {
	var hydraCalled bool
	hydra := func(w http.ResponseWriter, r *http.Request) {
		hydraCalled = true
		jsonHandler(http.StatusOK, `{"challenge":"lc1","client":{"client_id":"derived-client"}}`)(w, r)
	}

	h, _ := newOAuthTestHandler(t, nil, nil, hydra)
	app := fiber.New()
	app.Post("/login/google", h.GoogleLogin)

	req := httptest.NewRequest("POST", "/login/google", strings.NewReader(`{"login_challenge":"lc1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !hydraCalled {
		t.Error("expected Hydra to be consulted to derive client_id from login_challenge")
	}
}

// --- GoogleCallback ---

func TestGoogleCallback_MissingCodeOrState(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil)
	app := fiber.New()
	app.Get("/callback/google", h.GoogleCallback)

	resp, err := app.Test(httptest.NewRequest("GET", "/callback/google", nil))
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGoogleCallback_UnknownState(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil)
	app := fiber.New()
	app.Get("/callback/google", h.GoogleCallback)

	req := httptest.NewRequest("GET", "/callback/google?code=abc&state=never-issued", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown/expired state", resp.StatusCode)
	}
}

func TestGoogleCallback_OAuthErrorParam(t *testing.T) {
	h, _ := newOAuthTestHandler(t, nil, nil, nil)
	app := fiber.New()
	app.Get("/callback/google", h.GoogleCallback)

	req := httptest.NewRequest("GET", "/callback/google?error=access_denied&error_description=user+cancelled", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "access_denied") {
		t.Errorf("body = %q, want it to surface the OAuth error", body)
	}
}

func TestGoogleCallback_HappyPath_AcceptsHydraLogin(t *testing.T) {
	var hydraPath string
	hydra := func(w http.ResponseWriter, r *http.Request) {
		hydraPath = r.URL.Path
		jsonHandler(http.StatusOK, `{"redirect_to":"https://hydra.example.com/oauth2/auth?resume=1"}`)(w, r)
	}
	central := jsonHandler(http.StatusOK, `{"user_id":"user-1","tenant_id":"tenant-1","email":"user@example.com"}`)
	google := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			jsonHandler(http.StatusOK, `{"access_token":"gtok"}`)(w, r)
		case "/userinfo":
			jsonHandler(http.StatusOK, `{"id":"g1","email":"user@example.com","name":"Test User"}`)(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	h, _ := newOAuthTestHandler(t, google, central, hydra)
	h.stateStore.Set("state-1", &StateData{LoginChallenge: "lc1", ClientID: "client-1"})

	app := fiber.New()
	app.Get("/callback/google", h.GoogleCallback)

	req := httptest.NewRequest("GET", "/callback/google?code=auth-code&state=state-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to Hydra's accept response", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://hydra.example.com/oauth2/auth?resume=1" {
		t.Errorf("Location = %q, want Hydra's redirect_to forwarded unchanged", loc)
	}
	if hydraPath != "/admin/oauth2/auth/requests/login/accept" {
		t.Errorf("hydra called at %q, want the login accept endpoint", hydraPath)
	}
	if _, exists := h.stateStore.Get("state-1"); exists {
		t.Error("expected state to be consumed (single-use) after a successful callback")
	}
}

func TestGoogleCallback_CentralAPIAuthFails_RejectsHydraLogin(t *testing.T) {
	var hydraPath string
	hydra := func(w http.ResponseWriter, r *http.Request) {
		hydraPath = r.URL.Path
		jsonHandler(http.StatusOK, `{"redirect_to":"https://hydra.example.com/oauth2/auth?error=1"}`)(w, r)
	}
	central := jsonHandler(http.StatusInternalServerError, `{"error":"no invitation"}`)
	google := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			jsonHandler(http.StatusOK, `{"access_token":"gtok"}`)(w, r)
		case "/userinfo":
			jsonHandler(http.StatusOK, `{"id":"g1","email":"uninvited@example.com","name":"Nobody"}`)(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	h, _ := newOAuthTestHandler(t, google, central, hydra)
	h.stateStore.Set("state-2", &StateData{LoginChallenge: "lc2", ClientID: "client-1"})

	app := fiber.New()
	app.Get("/callback/google", h.GoogleCallback)

	req := httptest.NewRequest("GET", "/callback/google?code=auth-code&state=state-2", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to Hydra's reject response", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://hydra.example.com/oauth2/auth?error=1" {
		t.Errorf("Location = %q, want Hydra's reject redirect_to forwarded unchanged", loc)
	}
	if hydraPath != "/admin/oauth2/auth/requests/login/reject" {
		t.Errorf("hydra called at %q, want the login reject endpoint when Central API auth fails", hydraPath)
	}
}
