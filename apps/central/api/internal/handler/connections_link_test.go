package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// linkUsers is the part of the user service adding a provider account touches.
type linkUsers struct {
	user.Service
	accounts map[uuid.UUID]*user.User
	linked   []user.ProviderLink
	taken    bool
}

func (f *linkUsers) GetByID(id uuid.UUID) (*user.User, error) {
	if u, ok := f.accounts[id]; ok {
		return u, nil
	}
	return nil, user.ErrUserNotFound
}

func (f *linkUsers) LinkProvider(_ uuid.UUID, link user.ProviderLink) error {
	if f.taken {
		return user.ErrProviderSubjectTaken
	}
	f.linked = append(f.linked, link)
	return nil
}

type linkTestApp struct {
	app   *fiber.App
	users *linkUsers
	store *OAuthStateStore
	me    *user.User
	cl    *client.Client
}

// newLinkTestApp serves the link routes for one tenant, one client and one
// signed-in user (me), with a stand-in Google that signs in whoever google says.
func newLinkTestApp(t *testing.T, google *googleAccount, providers ...string) linkTestApp {
	t.Helper()
	stand := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer"}`))
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": google.ID, "email": google.Email, "verified_email": google.Verified})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stand.Close)

	tenant := uuid.New()
	cl := invitationClient(tenant, providers...)
	clients := newFakeClientService(cl)
	svc := social.NewGoogleService(&config.GoogleOAuthConfig{
		ClientID: "google-app", ClientSecret: "s", RedirectURL: "https://api.example.com/auth/google/callback",
		TokenURL: stand.URL + "/token", UserInfoURL: stand.URL + "/userinfo",
	}, nil, nil, clients, zap.NewNop())
	me := &user.User{ID: uuid.New(), TenantID: tenant, Email: "me@example.com", Active: true}
	users := &linkUsers{accounts: map[uuid.UUID]*user.User{me.ID: me}}
	store := NewOAuthStateStore(newTestRedisClient(t))
	h := NewSocialHandlerWithAllProviders(svc, nil, nil, nil, users, clients, hydra.NewClient("http://127.0.0.1:1"), zap.NewNop(), nil,
		store, testFrontendURL, SignInPolicy{})

	app := fiber.New()
	// Whoever the test says is signed in, and how recently.
	signedIn := func(c *fiber.Ctx) error {
		who := me.ID
		if other := c.Get("X-Test-User"); other != "" {
			who = uuid.MustParse(other)
		}
		authTime := time.Now().Add(-time.Minute)
		if c.Get("X-Test-Stale") != "" {
			authTime = time.Now().Add(-48 * time.Hour)
		}
		c.Locals("user_id", who)
		c.Locals("client_id", cl.ClientID)
		c.Locals("auth_time", authTime)
		return c.Next()
	}
	app.Get("/users/connections/available", signedIn, h.ListLinkableProviders)
	app.Post("/users/connections/:provider/link", signedIn, h.StartConnectionLink)
	app.Get("/users/connections/link/:ticket", h.RunConnectionLink)
	app.Post("/users/connections/link/complete", signedIn, h.CompleteConnectionLink)
	app.Get("/auth/google/callback", h.GoogleCallback)
	return linkTestApp{app: app, users: users, store: store, me: me, cl: cl}
}

func (a linkTestApp) do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := a.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func bodyOf(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// start asks to add a Google account and returns the ticket address.
func (a linkTestApp) start(t *testing.T, headers ...string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/users/connections/google/link", nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp := a.do(t, req)
	return resp.StatusCode, bodyOf(t, resp)
}

// ticketPath is the ticket address as the test app mounts it (without the
// public /api/v1 prefix).
func ticketPath(body map[string]any) string {
	address, _ := body["url"].(string)
	return strings.TrimPrefix(address, "/api/v1")
}

// roundTrip follows a ticket to the provider and back, as the user's browser
// does, and returns where it ended.
func (a linkTestApp) roundTrip(t *testing.T, ticketURL string) *url.URL {
	t.Helper()
	resp := a.do(t, httptest.NewRequest("GET", ticketURL, nil))
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("the ticket answered %d, want a redirect", resp.StatusCode)
	}
	to, _ := url.Parse(resp.Header.Get("Location"))
	if to.Query().Get("state") == "" {
		// The ticket was refused and sent the browser back to the account screen.
		return to
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "oauth_state" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no state cookie was set for the round trip")
	}
	req := httptest.NewRequest("GET", "/auth/google/callback?code=c&state="+url.QueryEscape(to.Query().Get("state")), nil)
	req.AddCookie(cookie)
	back := a.do(t, req)
	if back.StatusCode != http.StatusSeeOther && back.StatusCode != http.StatusFound {
		t.Fatalf("the callback answered %d, want a redirect to the account screen", back.StatusCode)
	}
	end, _ := url.Parse(back.Header.Get("Location"))
	return end
}

func (a linkTestApp) complete(t *testing.T, result string, headers ...string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/users/connections/link/complete", strings.NewReader(`{"result":"`+result+`"}`))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp := a.do(t, req)
	return resp.StatusCode, bodyOf(t, resp)
}

func TestConnectionLink_AddsTheProviderAccountThatSignedInAtTheProvider(t *testing.T) {
	a := newLinkTestApp(t, &googleAccount{ID: "g-home", Email: "home@gmail.example", Verified: true}, "google")

	status, body := a.start(t)
	if status != http.StatusCreated {
		t.Fatalf("start: %d %v, want 201", status, body)
	}
	if address, _ := body["url"].(string); !strings.HasPrefix(address, "/api/v1/users/connections/link/") {
		t.Fatalf("ticket address = %q", address)
	}
	end := a.roundTrip(t, ticketPath(body))
	result := end.Query().Get("link")
	if !strings.HasSuffix(end.Path, "/account") || result == "" {
		t.Fatalf("the callback ended at %s, want the account screen with a result", end)
	}
	if len(a.users.linked) != 0 {
		t.Fatal("the callback linked the account; only the signed-in user completing it may")
	}

	status, body = a.complete(t, result)
	if status != http.StatusOK {
		t.Fatalf("complete: %d %v, want 200", status, body)
	}
	if len(a.users.linked) != 1 {
		t.Fatalf("links = %+v, want one", a.users.linked)
	}
	link := a.users.linked[0]
	if link.Provider != "google" || link.Subject != "g-home" || link.Email != "home@gmail.example" || link.VerifiesAddress {
		t.Errorf("link = %+v, want google g-home under the provider's address, not verifying the account's own", link)
	}

	// A result works once.
	if status, _ := a.complete(t, result); status != http.StatusBadRequest {
		t.Errorf("a result used twice: %d, want 400", status)
	}
}

// A link started from someone else's ticket — a sign-in forced on this browser —
// is refused when this browser's user completes it.
func TestConnectionLink_AResultBelongsToTheUserItWasStartedFor(t *testing.T) {
	a := newLinkTestApp(t, &googleAccount{ID: "g-victim", Email: "victim@gmail.example", Verified: true}, "google")
	attacker := &user.User{ID: uuid.New(), TenantID: a.me.TenantID, Email: "attacker@example.com", Active: true}
	a.users.accounts[attacker.ID] = attacker

	// The attacker's ticket, followed in the victim's browser.
	_, body := a.start(t, "X-Test-User", attacker.ID.String())
	end := a.roundTrip(t, ticketPath(body))
	result := end.Query().Get("link")
	if result == "" {
		t.Fatalf("no result at %s", end)
	}

	// The victim's screen holds the victim's token and completes it.
	if status, _ := a.complete(t, result); status != http.StatusForbidden {
		t.Errorf("complete by another user: %d, want 403", status)
	}
	if len(a.users.linked) != 0 {
		t.Errorf("links = %+v, want none", a.users.linked)
	}
}

func TestConnectionLink_RefusalsAndTheirCodes(t *testing.T) {
	t.Run("a stale sign-in is asked to sign in again, at the start and at the end", func(t *testing.T) {
		a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "google")
		if status, body := a.start(t, "X-Test-Stale", "1"); status != 401 || body["code"] != "insufficient_user_authentication" {
			t.Errorf("start: %d %v, want 401 insufficient_user_authentication", status, body)
		}
		_, body := a.start(t)
		end := a.roundTrip(t, ticketPath(body))
		if status, body := a.complete(t, end.Query().Get("link"), "X-Test-Stale", "1"); status != 401 || body["code"] != "insufficient_user_authentication" {
			t.Errorf("complete: %d %v, want 401 insufficient_user_authentication", status, body)
		}
	})

	t.Run("a provider the client does not offer cannot be added", func(t *testing.T) {
		a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "email")
		if status, body := a.start(t); status != 404 || body["code"] != "provider_unavailable" {
			t.Errorf("start: %d %v, want 404 provider_unavailable", status, body)
		}
		resp := a.do(t, httptest.NewRequest("GET", "/users/connections/available", nil))
		if got, _ := bodyOf(t, resp)["providers"].([]any); len(got) != 0 {
			t.Errorf("available = %v, want none", got)
		}
	})

	t.Run("a ticket is used once", func(t *testing.T) {
		a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "google")
		_, body := a.start(t)
		ticket := ticketPath(body)
		_ = a.roundTrip(t, ticket)
		again := a.do(t, httptest.NewRequest("GET", ticket, nil))
		to, _ := url.Parse(again.Header.Get("Location"))
		if to.Query().Get("link_error") != "invalid_request" {
			t.Errorf("a ticket used twice went to %s, want link_error=invalid_request", to)
		}
	})

	t.Run("a provider account that signs in to another account is refused", func(t *testing.T) {
		a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "google")
		a.users.taken = true
		_, body := a.start(t)
		end := a.roundTrip(t, ticketPath(body))
		if status, body := a.complete(t, end.Query().Get("link")); status != 409 || body["code"] != "linked_to_another" {
			t.Errorf("complete: %d %v, want 409 linked_to_another", status, body)
		}
	})

	t.Run("an unknown or missing result is a bad request", func(t *testing.T) {
		a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "google")
		if status, _ := a.complete(t, "no-such-result"); status != 400 {
			t.Errorf("unknown result: %d, want 400", status)
		}
		if status, _ := a.complete(t, ""); status != 400 {
			t.Errorf("empty result: %d, want 400", status)
		}
	})
}

// The three records of a link — ticket, provider state, result — look alike;
// each step takes only the one made for it.
func TestConnectionLink_EachStepTakesOnlyItsOwnRecord(t *testing.T) {
	a := newLinkTestApp(t, &googleAccount{ID: "g-1", Email: "x@gmail.example", Verified: true}, "google")
	redirectsTo := func(resp *http.Response) *url.URL {
		to, _ := url.Parse(resp.Header.Get("Location"))
		return to
	}
	refused := func(to *url.URL) bool { return to.Query().Get("link_error") == "invalid_request" }

	// A provider state used as a ticket.
	_, body := a.start(t)
	toProvider := redirectsTo(a.do(t, httptest.NewRequest("GET", ticketPath(body), nil)))
	state := toProvider.Query().Get("state")
	if state == "" {
		t.Fatal("the ticket did not lead to the provider")
	}
	if to := redirectsTo(a.do(t, httptest.NewRequest("GET", "/users/connections/link/"+url.PathEscape(state), nil))); !refused(to) {
		t.Errorf("a provider state used as a ticket went to %s, want link_error=invalid_request", to)
	}

	// A ticket presented as the provider's state at the callback.
	_, body = a.start(t)
	ticket := strings.TrimPrefix(ticketPath(body), "/users/connections/link/")
	req := httptest.NewRequest("GET", "/auth/google/callback?code=c&state="+ticket, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: ticket})
	if to := redirectsTo(a.do(t, req)); !refused(to) || to.Query().Get("link") != "" {
		t.Errorf("a ticket used as the state went to %s, want link_error=invalid_request and no result", to)
	}

	// A result used as a ticket, and a ticket or state used as a result.
	_, body = a.start(t)
	end := a.roundTrip(t, ticketPath(body))
	result := end.Query().Get("link")
	if result == "" {
		t.Fatalf("no result at %s", end)
	}
	if to := redirectsTo(a.do(t, httptest.NewRequest("GET", "/users/connections/link/"+url.PathEscape(result), nil))); !refused(to) {
		t.Errorf("a result used as a ticket went to %s, want link_error=invalid_request", to)
	}
	_, body = a.start(t)
	if status, _ := a.complete(t, strings.TrimPrefix(ticketPath(body), "/users/connections/link/")); status != http.StatusBadRequest {
		t.Errorf("a ticket completed as a result: %d, want 400", status)
	}
	if len(a.users.linked) != 0 {
		t.Errorf("links = %+v, want none", a.users.linked)
	}
}
