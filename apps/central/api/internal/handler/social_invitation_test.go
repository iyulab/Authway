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
	"authway/apps/central/api/pkg/invitation"
)

// fakeInvitations answers the few things accepting an invitation with a
// provider asks of the invitation service. Embedding the interface leaves
// every other method nil: a test that reaches one fails loudly.
type fakeInvitations struct {
	invitation.Service
	byToken  map[string]*invitation.Invitation
	clientID string
	accepted []string
}

func (f *fakeInvitations) GetByToken(token string) (*invitation.Invitation, error) {
	if inv, ok := f.byToken[token]; ok {
		return inv, nil
	}
	return nil, invitation.ErrNotFound
}

func (f *fakeInvitations) SignInClientID(*invitation.Invitation) string { return f.clientID }

func (f *fakeInvitations) GetByID(id uuid.UUID) (*invitation.Invitation, error) {
	for _, inv := range f.byToken {
		if inv.ID == id {
			return inv, nil
		}
	}
	return nil, invitation.ErrNotFound
}

func (f *fakeInvitations) AcceptInvitedUser(id, _ uuid.UUID) error {
	f.accepted = append(f.accepted, id.String())
	return nil
}

type invitationTestApp struct {
	app   *fiber.App
	h     *SocialHandler
	inv   *fakeInvitations
	store *OAuthStateStore
}

// newInvitationTestApp serves the invitation sign-in routes for one tenant with
// one client, and a stand-in Google that signs in whoever *googleAccount says.
func newInvitationTestApp(t *testing.T, cl *client.Client, google *googleAccount) invitationTestApp {
	t.Helper()
	stand := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer"}`))
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": google.ID, "email": google.Email, "verified_email": google.Verified, "given_name": "Test", "family_name": "Person",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stand.Close)

	clients := newFakeClientService(cl)
	svc := social.NewGoogleService(&config.GoogleOAuthConfig{
		ClientID: "google-app", ClientSecret: "s", RedirectURL: "https://api.example.com/auth/google/callback",
		TokenURL: stand.URL + "/token", UserInfoURL: stand.URL + "/userinfo",
	}, nil, nil, clients, zap.NewNop())
	store := NewOAuthStateStore(newTestRedisClient(t))
	h := NewSocialHandlerWithAllProviders(svc, nil, nil, nil, nil, clients, hydra.NewClient("http://127.0.0.1:1"), zap.NewNop(), nil,
		store, testFrontendURL, SignInPolicy{})
	inv := &fakeInvitations{byToken: map[string]*invitation.Invitation{}, clientID: cl.ClientID}
	h.SetInvitations(inv)

	app := fiber.New()
	app.Get("/invitations/token/:token/social/:provider", h.StartInvitationSocial)
	app.Get("/invitations/receipts/:id", h.InvitationReceipt)
	app.Get("/auth/google/callback", h.GoogleCallback)
	return invitationTestApp{app: app, h: h, inv: inv, store: store}
}

type googleAccount struct {
	ID       string
	Email    string
	Verified bool
}

func pendingInvitation(tenantID uuid.UUID, email string) *invitation.Invitation {
	return &invitation.Invitation{
		ID: uuid.New(), TenantID: tenantID, Email: email, Status: invitation.StatusPending, ExpiresAt: time.Now().Add(time.Hour),
		TenantName: "Acme", InviterName: "Admin",
	}
}

func invitationClient(tenantID uuid.UUID, providers ...string) *client.Client {
	return &client.Client{Active: true, ID: uuid.New(), TenantID: tenantID, ClientID: "app", EnabledAuthProviders: providers}
}

// callbackAs presents the callback the way the browser that started the
// sign-in does: with the state cookie and the cookie remembering the token.
func callbackAs(t *testing.T, app *fiber.App, state, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=c&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	if token != "" {
		req.AddCookie(&http.Cookie{Name: invitationTokenCookie, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestOfferForInvitation_OnlyWhenTheInvitationsClientAllowsGoogle(t *testing.T) {
	tenant := uuid.New()
	for _, tc := range []struct {
		name      string
		providers []string
		want      int
	}{
		{"google allowed", []string{"email", "google"}, 1},
		{"google not allowed", []string{"email"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ta := newInvitationTestApp(t, invitationClient(tenant, tc.providers...), &googleAccount{})
			if got := ta.h.OfferForInvitation(pendingInvitation(tenant, "a@example.test")); len(got) != tc.want {
				t.Fatalf("offer = %v, want %d entries", got, tc.want)
			}
		})
	}

	t.Run("a client of another tenant is never offered", func(t *testing.T) {
		ta := newInvitationTestApp(t, invitationClient(uuid.New(), "google"), &googleAccount{})
		if got := ta.h.OfferForInvitation(pendingInvitation(tenant, "a@example.test")); len(got) != 0 {
			t.Fatalf("offered %v for another tenant's client", got)
		}
	})
	t.Run("no client to sign in under", func(t *testing.T) {
		ta := newInvitationTestApp(t, invitationClient(tenant, "google"), &googleAccount{})
		ta.inv.clientID = ""
		if got := ta.h.OfferForInvitation(pendingInvitation(tenant, "a@example.test")); len(got) != 0 {
			t.Fatalf("offered %v without a client", got)
		}
	})
}

func TestStartInvitationSocial_SendsTheBrowserToGoogleHintingTheInvitedAddress(t *testing.T) {
	tenant := uuid.New()
	ta := newInvitationTestApp(t, invitationClient(tenant, "email", "google"), &googleAccount{})
	ta.inv.byToken["tok"] = pendingInvitation(tenant, "invited@example.test")

	resp := get(t, ta.app, "/invitations/token/tok/social/google", "")

	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != fiber.StatusFound || loc.Host != "accounts.google.com" {
		t.Fatalf("status %d location %q, want 302 to Google", resp.StatusCode, resp.Header.Get("Location"))
	}
	if loc.Query().Get("login_hint") != "invited@example.test" {
		t.Errorf("login_hint = %q, want the invited address", loc.Query().Get("login_hint"))
	}
	var cookie *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "oauth_state" {
			cookie = ck
		}
	}
	if cookie == nil || cookie.Value != loc.Query().Get("state") {
		t.Fatalf("oauth_state cookie %v does not bind the state sent to Google", cookie)
	}
	stored, found, err := ta.store.Consume(t.Context(), cookie.Value)
	if err != nil || !found || stored.InvitationID != ta.inv.byToken["tok"].ID.String() || stored.ClientID != "app" || stored.LoginChallenge != "" {
		t.Fatalf("stored state = %+v found=%v err=%v, want the invitation and its client, no login flow", stored, found, err)
	}
}

func TestStartInvitationSocial_RefusalsEndOnTheAcceptScreen(t *testing.T) {
	tenant := uuid.New()
	ta := newInvitationTestApp(t, invitationClient(tenant, "email", "google"), &googleAccount{})
	ta.inv.byToken["ok"] = pendingInvitation(tenant, "a@example.test")
	used := pendingInvitation(tenant, "b@example.test")
	used.Status = invitation.StatusAccepted
	ta.inv.byToken["used"] = used
	expired := pendingInvitation(tenant, "c@example.test")
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	ta.inv.byToken["expired"] = expired

	for _, tc := range []struct{ target, wantQuery string }{
		{"/invitations/token/ok/social/github", "error=provider_unavailable&token=ok"},
		{"/invitations/token/used/social/google", "error=invitation_unavailable"},
		{"/invitations/token/expired/social/google", "error=invitation_unavailable"},
		{"/invitations/token/unknown/social/google", "error=invitation_unavailable"},
	} {
		resp := get(t, ta.app, tc.target, "")
		want := testFrontendURL + "/invitation/accept?" + tc.wantQuery
		if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != want {
			t.Errorf("%s: status %d location %q, want 302 to %q", tc.target, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
}

// A Google account that is not the invited address — or is, but unverified —
// ends the sign-in with nothing created, linked or accepted, and the
// invitation stays as it was.
func TestInvitationCallback_RefusesAnyOtherGoogleAccountAndChangesNothing(t *testing.T) {
	tenant := uuid.New()
	for _, tc := range []struct {
		name   string
		google googleAccount
	}{
		{"another address", googleAccount{ID: "g1", Email: "someone-else@example.test", Verified: true}},
		{"the invited address, not verified by Google", googleAccount{ID: "g2", Email: "invited@example.test", Verified: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.google
			ta := newInvitationTestApp(t, invitationClient(tenant, "email", "google"), &g)
			ta.inv.byToken["tok"] = pendingInvitation(tenant, "invited@example.test")
			state, err := ta.store.Save(t.Context(), oauthState{ClientID: "app", InvitationID: ta.inv.byToken["tok"].ID.String()})
			if err != nil {
				t.Fatal(err)
			}

			resp := callbackAs(t, ta.app, state, "tok")

			want := testFrontendURL + "/invitation/accept?error=address_mismatch&token=tok"
			if resp.StatusCode != fiber.StatusFound || resp.Header.Get("Location") != want {
				t.Fatalf("status %d location %q, want 302 to %q", resp.StatusCode, resp.Header.Get("Location"), want)
			}
			if len(ta.inv.accepted) != 0 {
				t.Fatalf("the invitation was accepted: %v", ta.inv.accepted)
			}
			// The state is single-use.
			if _, found, _ := ta.store.Consume(t.Context(), state); found {
				t.Error("the state outlived its callback")
			}
		})
	}
}

func TestInvitationCallback_ARepeatedStateDoesNothing(t *testing.T) {
	tenant := uuid.New()
	ta := newInvitationTestApp(t, invitationClient(tenant, "email", "google"), &googleAccount{ID: "g", Email: "x@example.test", Verified: true})
	ta.inv.byToken["tok"] = pendingInvitation(tenant, "invited@example.test")
	state, _ := ta.store.Save(t.Context(), oauthState{ClientID: "app", InvitationID: ta.inv.byToken["tok"].ID.String()})
	target := "/auth/google/callback?code=c&state=" + url.QueryEscape(state)
	get(t, ta.app, target, state)

	resp := get(t, ta.app, target, state)

	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, testFrontendURL+"/error?") {
		t.Fatalf("a second callback with the same state went to %q, want the error screen", loc)
	}
}

func TestInvitationReceipt(t *testing.T) {
	tenant := uuid.New()
	ta := newInvitationTestApp(t, invitationClient(tenant, "google"), &googleAccount{})

	missing := get(t, ta.app, "/invitations/receipts/nope", "")
	if missing.StatusCode != fiber.StatusNotFound {
		t.Fatalf("an unknown receipt answered %d, want 404", missing.StatusCode)
	}

	inv := pendingInvitation(tenant, "a@example.test")
	inv.Message = "Open the app"
	inv.App = &invitation.InvitationApp{ClientID: "app", Name: "App", ClientURI: "https://app.example.test/"}
	app := fiber.New()
	app.Get("/mk", func(c *fiber.Ctx) error {
		id, err := ta.h.saveInvitationReceipt(c, inv)
		if err != nil {
			return err
		}
		return c.SendString(id)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/mk", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Same store: the receipt is made against ta's own store through ta.h.
	got := get(t, ta.app, "/invitations/receipts/"+string(raw), "")
	if got.StatusCode != fiber.StatusOK {
		t.Fatalf("receipt answered %d, want 200", got.StatusCode)
	}
	var body struct {
		Invitation struct {
			TenantName string                    `json:"tenant_name"`
			Message    string                    `json:"message"`
			Client     *invitation.InvitationApp `json:"client"`
		} `json:"invitation"`
	}
	if err := json.NewDecoder(got.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Invitation.TenantName != "Acme" || body.Invitation.Message != "Open the app" || body.Invitation.Client == nil || body.Invitation.Client.ClientURI != "https://app.example.test/" {
		t.Fatalf("receipt = %+v", body.Invitation)
	}
}
