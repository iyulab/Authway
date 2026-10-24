package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// engineSeen is what the stand-in authorization server received.
type engineSeen struct {
	calls   int
	path    string
	body    string
	headers http.Header
}

func newTokenEndpointApp(t *testing.T, answer func(w http.ResponseWriter)) (*fiber.App, *engineSeen) {
	t.Helper()
	seen := &engineSeen{}
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.calls++
		seen.path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		seen.body = string(b)
		seen.headers = r.Header.Clone()
		answer(w)
	}))
	t.Cleanup(engine.Close)

	app := fiber.New()
	app.Post("/oauth2/token", NewTokenEndpoint(engine.URL+"/", nil, zap.NewNop()).Token)
	return app, seen
}

func postToken(t *testing.T, app *fiber.App, body string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/oauth2/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestTokenEndpoint_PassesAGrantOnUnchangedAndReturnsTheAnswer(t *testing.T) {
	app, seen := newTokenEndpointApp(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Access-Control-Allow-Origin", "https://engine-decides.example")
		w.Header().Set("Set-Cookie", "engine=1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"bearer"}`))
	})
	body := "grant_type=authorization_code&code=c&redirect_uri=https%3A%2F%2Fapp.example%2Fcb&code_verifier=v"
	resp := postToken(t, app, body, map[string]string{
		"Authorization": "Basic Y2xpZW50OnNlY3JldA==",
		"Cookie":        "session=browser",
		"DPoP":          "proof",
	})

	if seen.calls != 1 || seen.path != "/oauth2/token" {
		t.Fatalf("engine calls %d at %q, want one at /oauth2/token", seen.calls, seen.path)
	}
	if seen.body != body {
		t.Errorf("body reached the engine as %q, want it unchanged", seen.body)
	}
	if got := seen.headers.Get("Authorization"); got != "Basic Y2xpZW50OnNlY3JldA==" {
		t.Errorf("client authentication reached the engine as %q", got)
	}
	if got := seen.headers.Get("DPoP"); got != "proof" {
		t.Errorf("DPoP proof reached the engine as %q", got)
	}
	if got := seen.headers.Get("Cookie"); got != "" {
		t.Errorf("a browser cookie reached the engine (%q)", got)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want the engine's 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want the engine's no-store", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("the engine's CORS answer was passed on (%q); the API decides that", got)
	}
	if got := resp.Header.Get("Set-Cookie"); got != "" {
		t.Errorf("an engine cookie was passed on (%q)", got)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != `{"access_token":"at","token_type":"bearer"}` {
		t.Errorf("body %s, want the engine's answer unchanged", b)
	}
}

func TestTokenEndpoint_PassesTheEnginesRefusalOnUnchanged(t *testing.T) {
	app, _ := newTokenEndpointApp(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", `Basic realm="token"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	})
	resp := postToken(t, app, "grant_type=client_credentials", nil)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("status %d WWW-Authenticate %q, want the engine's 401 and challenge", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestTokenEndpoint_TokenExchangeIsAnsweredHereNotPassedOn(t *testing.T) {
	app, seen := newTokenEndpointApp(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) })
	resp := postToken(t, app, "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange&subject_token=x", nil)
	if seen.calls != 0 {
		t.Fatal("a token exchange was passed on to the engine")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("a token endpoint answer must not be cached (RFC 6749 section 5.1)")
	}
	var answer map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if answer["error"] != "unsupported_grant_type" {
		t.Errorf("error = %q, want unsupported_grant_type", answer["error"])
	}
}

func TestTokenEndpoint_AnEngineThatDoesNotAnswerIsTemporarilyUnavailable(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	engine.Close()
	app := fiber.New()
	app.Post("/oauth2/token", NewTokenEndpoint(engine.URL, nil, zap.NewNop()).Token)

	resp := postToken(t, app, "grant_type=client_credentials", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
	var answer map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&answer)
	if answer["error"] != "temporarily_unavailable" {
		t.Errorf("error = %q, want temporarily_unavailable", answer["error"])
	}
}

func TestTokenHook_ChangesNothingForTheAuthorizationServerAndNothingForAnyoneElse(t *testing.T) {
	app := fiber.New()
	app.Post("/internal/token-hook", NewTokenHook([]string{"hook-key", "previous-key"}, nil, nil, zap.NewNop()).Hook)
	call := func(key string) int {
		req := httptest.NewRequest(fiber.MethodPost, "/internal/token-hook", strings.NewReader(`{"session":{},"request":{"grant_types":["authorization_code"]}}`))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set(TokenHookHeader, key)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	for _, key := range []string{"hook-key", "previous-key"} {
		if got := call(key); got != http.StatusNoContent {
			t.Errorf("with key %q: %d, want 204 (issue the token unchanged)", key, got)
		}
	}
	for _, key := range []string{"", "other-key", "hook-key-and-more"} {
		if got := call(key); got != http.StatusUnauthorized {
			t.Errorf("with key %q: %d, want 401", key, got)
		}
	}
}

// Only Authway's own assertions are meant to reach the authorization server's
// assertion grant; a deployment that cannot judge them refuses the token.
func TestTokenHook_RefusesAnAssertionGrantItCannotJudge(t *testing.T) {
	app := fiber.New()
	app.Post("/internal/token-hook", NewTokenHook([]string{"hook-key"}, nil, nil, zap.NewNop()).Hook)
	req := httptest.NewRequest(fiber.MethodPost, "/internal/token-hook", strings.NewReader(`{"request":{"client_id":"c","grant_types":["urn:ietf:params:oauth:grant-type:jwt-bearer"],"payload":{"assertion":["x"]}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHookHeader, "hook-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestPayloadValueReadsAStringOrAList(t *testing.T) {
	if got := payloadValue([]byte(`"a"`)); got != "a" {
		t.Errorf("string: %q", got)
	}
	if got := payloadValue([]byte(`["b","c"]`)); got != "b" {
		t.Errorf("list: %q", got)
	}
	if got := payloadValue(nil); got != "" {
		t.Errorf("absent: %q", got)
	}
}

// stubRegistrations answers one registered client, "m2m".
type stubRegistrations struct{ err error }

func (s stubRegistrations) Registration(clientID string) (string, []string, error) {
	if s.err != nil {
		return "", nil, s.err
	}
	if clientID != "m2m" {
		return "", nil, errors.New("not registered")
	}
	return "tenant-1", []string{"https://orders.example", "https://billing.example"}, nil
}

// A client's own token follows the rule of a user's: for the resource servers
// the client is registered for when the request names none.
func TestTokenEndpoint_AClientsOwnTokenIsForItsRegisteredAudienceWhenTheRequestNamesNone(t *testing.T) {
	seen := &engineSeen{}
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(engine.Close)
	app := fiber.New()
	app.Post("/oauth2/token", NewTokenEndpoint(engine.URL, stubRegistrations{}, zap.NewNop()).Token)

	cases := []struct {
		name, body string
		headers    map[string]string
		want       []string
	}{
		{"none named, secret in the form", "grant_type=client_credentials&client_id=m2m&client_secret=s", nil, []string{"https://orders.example", "https://billing.example"}},
		{"none named, HTTP Basic", "grant_type=client_credentials", map[string]string{"Authorization": "Basic bTJtOnM="}, []string{"https://orders.example", "https://billing.example"}},
		{"one named", "grant_type=client_credentials&client_id=m2m&client_secret=s&audience=https%3A%2F%2Forders.example", nil, []string{"https://orders.example"}},
		{"an unknown client", "grant_type=client_credentials&client_id=other&client_secret=s", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			postToken(t, app, tc.body, tc.headers)
			form, err := url.ParseQuery(seen.body)
			if err != nil {
				t.Fatal(err)
			}
			if got := form["audience"]; !slices.Equal(got, tc.want) {
				t.Errorf("audience reached the engine as %q, want %q", got, tc.want)
			}
			if form.Get("grant_type") != "client_credentials" {
				t.Errorf("grant_type lost: %q", seen.body)
			}
		})
	}
}

func TestTokenHook_AClientsOwnTokenGetsTheClientsTenant(t *testing.T) {
	call := func(regs ClientRegistrations) *http.Response {
		app := fiber.New()
		app.Post("/internal/token-hook", NewTokenHook([]string{"hook-key"}, nil, regs, zap.NewNop()).Hook)
		req := httptest.NewRequest(fiber.MethodPost, "/internal/token-hook", strings.NewReader(`{"request":{"client_id":"m2m","grant_types":["client_credentials"]}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(TokenHookHeader, "hook-key")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := call(stubRegistrations{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200 with claims", resp.StatusCode)
	}
	var answer struct {
		Session struct {
			AccessToken map[string]string `json:"access_token"`
		} `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if answer.Session.AccessToken["tenant_id"] != "tenant-1" {
		t.Errorf("access token claims %v, want tenant_id tenant-1", answer.Session.AccessToken)
	}

	// A token the contract would not describe is not issued.
	if got := call(stubRegistrations{err: errors.New("database down")}).StatusCode; got != http.StatusForbidden {
		t.Errorf("unreadable registration: %d, want 403", got)
	}
}

// A client_id in the form of a user's id, registered before registration
// refused one, gets no token: its sub would read as that user's.
func TestTokenHook_RefusesAClientTokenWhoseClientIDHasTheFormOfAUserID(t *testing.T) {
	app := fiber.New()
	app.Post("/internal/token-hook", NewTokenHook([]string{"hook-key"}, nil, uuidRegistered{}, zap.NewNop()).Hook)
	req := httptest.NewRequest(fiber.MethodPost, "/internal/token-hook", strings.NewReader(`{"request":{"client_id":"6f1c1e0e-3b4a-4f2a-9d55-6a2b1c3d4e5f","grant_types":["client_credentials"]}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHookHeader, "hook-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

// uuidRegistered answers every client as registered.
type uuidRegistered struct{}

func (uuidRegistered) Registration(string) (string, []string, error) { return "tenant-1", nil, nil }
