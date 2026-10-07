package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// newConsentTestApp serves the consent-flow routes against a Hydra that knows
// the consent flow "cf-1=" for user u, and records what it was told.
func newConsentTestApp(t *testing.T, skip bool) (*fiber.App, *[]string) {
	t.Helper()
	u := buildTestUser(t, "unused", false)
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("challenge") != "cf-1=" && r.URL.Query().Get("consent_challenge") != "cf-1=" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/consent"):
			_ = json.NewEncoder(w).Encode(hydra.ConsentRequest{
				Challenge: "cf-1=", Subject: u.ID.String(), RequestedScope: []string{"openid", "email"},
				Client: &hydra.OAuth2Client{ClientID: testClientID, ClientName: "App"}, Skip: skip,
			})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/consent/accept"):
			calls = append(calls, "accept")
			_, _ = w.Write([]byte(`{"redirect_to":"https://example.com/after-consent"}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/consent/reject"):
			calls = append(calls, "reject")
			_, _ = w.Write([]byte(`{"redirect_to":"https://example.com/callback?error=access_denied"}`))
		default:
			t.Errorf("unexpected Hydra call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	h := NewAuthHandler(newFakeUserService(u), newFakeClientService(), fakeClaimsService{}, &fakeMFAService{}, hydra.NewClient(srv.URL), zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{})
	app := fiber.New()
	app.Get("/consent-flows/:flow", h.GetConsentFlow)
	app.Post("/consent-flows/:flow/accept", h.AcceptConsent)
	app.Post("/consent-flows/:flow/reject", h.RejectConsent)
	return app, &calls
}

func doConsent(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestConsentFlow_AsksWhenTheClientDoesNotSkip(t *testing.T) {
	app, calls := newConsentTestApp(t, false)

	status, body := doConsent(t, app, "GET", "/consent-flows/cf-1%3D", "")
	if status != fiber.StatusOK || body["next"] != "form" || body["flow"] != "cf-1=" || body["client_name"] != "App" {
		t.Fatalf("status %d body %v, want next=form for the decoded flow", status, body)
	}
	if len(*calls) != 0 {
		t.Errorf("Hydra calls %v, want none before the user answers", *calls)
	}

	status, body = doConsent(t, app, "POST", "/consent-flows/cf-1%3D/accept", `{"grant_scope":["openid"],"remember":true,"remember_for":3600}`)
	if status != fiber.StatusOK || body["next"] != "redirect" || body["redirect_to"] != "https://example.com/after-consent" {
		t.Fatalf("accept: status %d body %v", status, body)
	}
}

func TestConsentFlow_SkippedConsentRedirectsAtOnce(t *testing.T) {
	app, calls := newConsentTestApp(t, true)

	status, body := doConsent(t, app, "GET", "/consent-flows/cf-1%3D", "")
	if status != fiber.StatusOK || body["next"] != "redirect" || body["redirect_to"] != "https://example.com/after-consent" {
		t.Fatalf("status %d body %v, want next=redirect", status, body)
	}
	if len(*calls) != 1 || (*calls)[0] != "accept" {
		t.Errorf("Hydra calls %v, want one accept", *calls)
	}
}

func TestConsentFlow_RejectAndUnknownFlow(t *testing.T) {
	app, _ := newConsentTestApp(t, false)

	status, body := doConsent(t, app, "POST", "/consent-flows/cf-1%3D/reject", "")
	if status != fiber.StatusOK || body["next"] != "redirect" || !strings.Contains(body["redirect_to"].(string), "access_denied") {
		t.Fatalf("reject: status %d body %v", status, body)
	}

	status, body = doConsent(t, app, "GET", "/consent-flows/no-such-flow", "")
	if status != fiber.StatusBadRequest || body["code"] != "invalid_flow" {
		t.Fatalf("unknown flow: status %d body %v, want 400 invalid_flow", status, body)
	}
}
