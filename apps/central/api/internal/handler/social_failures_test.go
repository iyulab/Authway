package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service/social"
)

const testFrontendURL = "https://login.example.com"

// newCallbackTestApp serves the Google callback with a Hydra that records the
// login rejections it receives.
func newCallbackTestApp(t *testing.T) (*fiber.App, *OAuthStateStore, *[]string) {
	t.Helper()
	var rejected []string
	hydraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/login/reject") {
			rejected = append(rejected, r.URL.Query().Get("challenge"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"redirect_to":"https://app.example.com/callback?error=access_denied"}`))
			return
		}
		t.Errorf("unexpected Hydra call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(hydraSrv.Close)

	store := NewOAuthStateStore(newTestRedisClient(t))
	h := NewSocialHandlerWithAllProviders(nil, nil, nil, nil, nil, hydra.NewClient(hydraSrv.URL), zap.NewNop(), nil, store, testFrontendURL)
	app := fiber.New()
	app.Get("/auth/google/callback", h.GoogleCallback)
	return app, store, &rejected
}

func get(t *testing.T, app *fiber.App, target, stateCookie string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	if stateCookie != "" {
		req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateCookie})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// A user who cancels at the provider goes back to the application with an
// OAuth error rather than being stranded on a JSON error page.
func TestGoogleCallback_ProviderRefusalReturnsToTheApplication(t *testing.T) {
	app, store, rejected := newCallbackTestApp(t)
	state, err := store.Save(context.Background(), oauthState{LoginChallenge: "challenge-1", ClientID: "app"})
	if err != nil {
		t.Fatal(err)
	}

	resp := get(t, app, "/auth/google/callback?error=access_denied&state="+url.QueryEscape(state), state)

	if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != "https://app.example.com/callback?error=access_denied" {
		t.Fatalf("status %d location %q, want 302 to the rejection redirect", resp.StatusCode, resp.Header.Get("Location"))
	}
	if len(*rejected) != 1 || (*rejected)[0] != "challenge-1" {
		t.Errorf("rejected %v, want the login request the state belonged to", *rejected)
	}
}

// Without a genuine state there is no login request to reject; the login UI's
// error screen explains instead — and nothing about the server leaks.
func TestGoogleCallback_InvalidLinkShowsTheErrorScreen(t *testing.T) {
	app, _, rejected := newCallbackTestApp(t)

	resp := get(t, app, "/auth/google/callback?code=abc&state=forged", "forged")

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status %d location %q, want a 302", resp.StatusCode, resp.Header.Get("Location"))
	}
	if loc.Scheme+"://"+loc.Host+loc.Path != testFrontendURL+"/error" || loc.Query().Get("error") != "invalid_request" {
		t.Errorf("redirected to %s, want the login UI error screen with invalid_request", loc)
	}
	if len(*rejected) != 0 {
		t.Errorf("rejected %v, want no Hydra call without a genuine state", *rejected)
	}
}

func TestSocialFailure_UninvitedAddressIsARefusal(t *testing.T) {
	if code, _ := socialFailure(social.ErrNotInvited); code != "access_denied" {
		t.Errorf("not invited -> %q, want access_denied", code)
	}
	code, description := socialFailure(errors.New("token exchange: dial tcp 10.0.0.1:443: i/o timeout"))
	if code != "server_error" || strings.Contains(description, "10.0.0.1") {
		t.Errorf("other failure -> %q %q, want server_error without internal detail", code, description)
	}
}
