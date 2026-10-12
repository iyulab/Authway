package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
)

// introspectingClients knows a resource server (confidential), a public
// application, and the clients tokens were issued to.
type introspectingClients struct {
	client.Service
	clients     map[string]*client.Client
	secrets     map[string]string
	validations int
}

func (c *introspectingClients) ValidateClient(id, secret string) (*client.Client, error) {
	c.validations++
	found, ok := c.clients[id]
	if !ok {
		return nil, errors.New("client not found")
	}
	if found.Public {
		return found, nil
	}
	if c.secrets[id] != secret {
		return nil, errors.New("invalid client credentials")
	}
	return found, nil
}

func (c *introspectingClients) GetByClientID(id string) (*client.Client, error) {
	if found, ok := c.clients[id]; ok {
		return found, nil
	}
	return nil, errors.New("client not found")
}

const resourceServerSecret = "s3cret with/odd:chars"

// newIntrospectionApp serves the endpoint with an authorization server that
// knows the tokens in `tokens` (anything else is inactive).
func newIntrospectionApp(t *testing.T, tokens map[string]string) (*fiber.App, *introspectingClients, uuid.UUID) {
	t.Helper()
	tenant, otherTenant := uuid.New(), uuid.New()
	clients := &introspectingClients{
		clients: map[string]*client.Client{
			"resource-server": {ClientID: "resource-server", TenantID: tenant},
			"spa":             {ClientID: "spa", TenantID: tenant, Public: true},
			"other-tenant":    {ClientID: "other-tenant", TenantID: otherTenant},
		},
		secrets: map[string]string{"resource-server": resourceServerSecret},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		answer, known := tokens[form.Get("token")]
		switch {
		case form.Get("token") == "authorization-server-down":
			w.WriteHeader(http.StatusBadGateway)
		case known:
			_, _ = w.Write([]byte(answer))
		default:
			_, _ = w.Write([]byte(`{"active":false}`))
		}
	}))
	t.Cleanup(srv.Close)

	app := fiber.New()
	app.Post("/api/v1/oauth2/introspect", NewIntrospectionHandler(clients, hydra.NewClient(srv.URL), zap.NewNop()).Introspect)
	return app, clients, tenant
}

func basic(id, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(id)+":"+url.QueryEscape(secret)))
}

func introspect(t *testing.T, app *fiber.App, authorization string, form url.Values) (int, map[string]any, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth2/introspect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body, resp.Header
}

const (
	userToken = `{"active":true,"sub":"user-1","client_id":"spa","scope":"openid profile","token_type":"Bearer","token_use":"access_token",
		"iss":"https://issuer.example","aud":["https://api.example"],"exp":1900000000,"iat":1800000000,
		"ext":{"tenant_id":"t-1","email":"u@example.com","name":"U","auth_time":1800000000,"role":"editor"}}`
	otherTenantToken = `{"active":true,"sub":"user-9","client_id":"other-tenant"}`
	strayToken       = `{"active":true,"sub":"user-9","client_id":"no-such-client"}`
)

func TestIntrospect_AnswersForTokensOfTheCallersTenant(t *testing.T) {
	app, _, _ := newIntrospectionApp(t, map[string]string{"good": userToken})

	status, body, _ := introspect(t, app, basic("resource-server", resourceServerSecret), url.Values{"token": {"good"}})
	if status != http.StatusOK || body["active"] != true {
		t.Fatalf("status %d body %v, want an active token", status, body)
	}
	for name, want := range map[string]any{
		"sub": "user-1", "client_id": "spa", "scope": "openid profile", "iss": "https://issuer.example",
		"tenant_id": "t-1", "email": "u@example.com", "name": "U",
	} {
		if body[name] != want {
			t.Errorf("%s = %v, want %v", name, body[name], want)
		}
	}
	if body["exp"] != float64(1900000000) || body["auth_time"] != float64(1800000000) {
		t.Errorf("exp %v auth_time %v", body["exp"], body["auth_time"])
	}
	// What is not part of the contract stays under ext, as in a JWT access token.
	ext, _ := body["ext"].(map[string]any)
	if ext["role"] != "editor" || ext["tenant_id"] != nil {
		t.Errorf("ext = %v, want the stored claims only", ext)
	}

	// The same credentials in the form instead of the header.
	status, body, _ = introspect(t, app, "", url.Values{"token": {"good"}, "client_id": {"resource-server"}, "client_secret": {resourceServerSecret}})
	if status != http.StatusOK || body["active"] != true {
		t.Errorf("form credentials: status %d body %v", status, body)
	}
}

// Whatever the reason a token is not good for this caller, the answer is the
// same and says nothing more.
func TestIntrospect_EveryOtherTokenIsInactiveAndSaysNothingElse(t *testing.T) {
	app, _, _ := newIntrospectionApp(t, map[string]string{"elsewhere": otherTenantToken, "stray": strayToken})
	for _, token := range []string{"unknown-or-revoked", "elsewhere", "stray"} {
		status, body, _ := introspect(t, app, basic("resource-server", resourceServerSecret), url.Values{"token": {token}})
		if status != http.StatusOK || len(body) != 1 || body["active"] != false {
			t.Errorf("%s: status %d body %v, want exactly {active: false}", token, status, body)
		}
	}
}

func TestIntrospect_OnlyAConfidentialClientWithItsSecretMayAsk(t *testing.T) {
	app, _, _ := newIntrospectionApp(t, map[string]string{"good": userToken})
	for name, authorization := range map[string]string{
		"no credentials":    "",
		"wrong secret":      basic("resource-server", "wrong"),
		"unknown client":    basic("nobody", "x"),
		"a public client":   basic("spa", "anything"),
		"malformed header":  "Basic !!!",
		"secret left empty": basic("resource-server", ""),
	} {
		status, body, header := introspect(t, app, authorization, url.Values{"token": {"good"}})
		if status != http.StatusUnauthorized || body["error"] != "invalid_client" || body["active"] != nil {
			t.Errorf("%s: status %d body %v, want 401 invalid_client", name, status, body)
		}
		if !strings.HasPrefix(header.Get("WWW-Authenticate"), "Basic") {
			t.Errorf("%s: no WWW-Authenticate challenge", name)
		}
	}
}

func TestIntrospect_RequestAndUpstreamFailures(t *testing.T) {
	app, _, _ := newIntrospectionApp(t, nil)
	if status, body, _ := introspect(t, app, basic("resource-server", resourceServerSecret), url.Values{}); status != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Errorf("no token: status %d body %v", status, body)
	}
	// Not knowing is not the same as inactive: a resource server must not
	// take an outage for a logout.
	status, body, _ := introspect(t, app, basic("resource-server", resourceServerSecret), url.Values{"token": {"authorization-server-down"}})
	if status != http.StatusServiceUnavailable || body["active"] != nil {
		t.Errorf("authorization server down: status %d body %v, want 503 without an answer", status, body)
	}
}

// A secret is slow to check on purpose, and a resource server asks on every
// request, so a pair found right is not checked again straight away.
func TestIntrospect_RemembersCredentialsItJustChecked(t *testing.T) {
	app, clients, _ := newIntrospectionApp(t, map[string]string{"good": userToken})
	for i := 0; i < 3; i++ {
		introspect(t, app, basic("resource-server", resourceServerSecret), url.Values{"token": {"good"}})
	}
	if clients.validations != 1 {
		t.Errorf("the secret was checked %d times for 3 calls, want once", clients.validations)
	}
	// A wrong secret is never remembered as right.
	introspect(t, app, basic("resource-server", "wrong"), url.Values{"token": {"good"}})
	introspect(t, app, basic("resource-server", "wrong"), url.Values{"token": {"good"}})
	if clients.validations != 3 {
		t.Errorf("wrong secrets were checked %d times in total, want every time (3)", clients.validations)
	}
}
