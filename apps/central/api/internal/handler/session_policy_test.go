package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

func TestSessionPolicy_Lifetime(t *testing.T) {
	tenant := uuid.New()
	of := func(d time.Duration) SessionPolicy {
		return SessionPolicy{Timeout: func(uuid.UUID) time.Duration { return d }}
	}
	cases := []struct {
		name         string
		policy       SessionPolicy
		keepSignedIn bool
		want         time.Duration
	}{
		{"no tenant lookup: the default", SessionPolicy{}, false, defaultSessionTimeout},
		{"a tenant without a timeout: the default", of(0), false, defaultSessionTimeout},
		{"the tenant's timeout", of(8 * time.Hour), false, 8 * time.Hour},
		{"keep me signed in outlasts the tenant's timeout", of(8 * time.Hour), true, keepSignedInFor},
		{"keep me signed in without a tenant lookup", SessionPolicy{}, true, keepSignedInFor},
		{"a timeout past what a session can last is cut to it", of(365 * 24 * time.Hour), false, keepSignedInFor},
	}
	for _, tc := range cases {
		if got := tc.policy.lifetime(tenant, tc.keepSignedIn); got != tc.want {
			t.Errorf("%s: lifetime = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestAsksForNoPrompt(t *testing.T) {
	cases := map[string]bool{
		"https://issuer.example/oauth2/auth?client_id=a&prompt=none":       true,
		"https://issuer.example/oauth2/auth?prompt=none&client_id=a":       true,
		"https://issuer.example/oauth2/auth?client_id=a&prompt=login":      false,
		"https://issuer.example/oauth2/auth?client_id=a":                   false,
		"https://issuer.example/oauth2/auth?client_id=a&state=prompt=none": false,
		"":      false,
		"%zz:/": false,
	}
	for requestURL, want := range cases {
		if got := asksForNoPrompt(requestURL); got != want {
			t.Errorf("asksForNoPrompt(%q) = %v, want %v", requestURL, got, want)
		}
	}
}

// sessionHydra stands in for the authorization server's admin API and keeps
// what it was told to accept or reject.
type sessionHydra struct {
	login   hydra.LoginRequest
	consent hydra.ConsentRequest

	acceptedLogin   *hydra.AcceptLoginRequest
	rejectedLogin   map[string]string
	acceptedConsent *hydra.AcceptConsentRequest
}

func (f *sessionHydra) client(t *testing.T) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/login"):
			_ = json.NewEncoder(w).Encode(f.login)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/login/accept"):
			f.acceptedLogin = &hydra.AcceptLoginRequest{}
			_ = json.NewDecoder(r.Body).Decode(f.acceptedLogin)
			_, _ = w.Write([]byte(`{"redirect_to":"https://issuer.example/after-login"}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/login/reject"):
			f.rejectedLogin = map[string]string{}
			_ = json.NewDecoder(r.Body).Decode(&f.rejectedLogin)
			_, _ = w.Write([]byte(`{"redirect_to":"https://app.example/callback?error=login_required"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/consent"):
			_ = json.NewEncoder(w).Encode(f.consent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/consent/accept"):
			f.acceptedConsent = &hydra.AcceptConsentRequest{}
			_ = json.NewDecoder(r.Body).Decode(f.acceptedConsent)
			_, _ = w.Write([]byte(`{"redirect_to":"https://issuer.example/after-consent"}`))
		default:
			t.Errorf("unexpected call to the authorization server: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

// Every sign-in opens a session, and the user's choice only sets its length.
func TestSignIn_OpensASessionWhateverTheUserChose(t *testing.T) {
	u := buildTestUser(t, "unused", false)
	policy := SessionPolicy{Timeout: func(tenantID uuid.UUID) time.Duration {
		if tenantID != u.TenantID {
			t.Errorf("the policy was asked about tenant %s, want the user's %s", tenantID, u.TenantID)
		}
		return 2 * time.Hour
	}}
	for _, tc := range []struct {
		name         string
		method       signInMethod
		keepSignedIn bool
		want         time.Duration
	}{
		{"password, box left unticked", byPassword, false, 2 * time.Hour},
		{"password, keep me signed in", byPassword, true, keepSignedInFor},
		{"emailed link", byEmailedLink, false, 2 * time.Hour},
		{"another identity provider", byProvider("google"), false, 2 * time.Hour},
	} {
		h := &sessionHydra{login: hydra.LoginRequest{Challenge: "flow-1", Client: &hydra.OAuth2Client{ClientID: testClientID}}}
		flows := newSignIn(h.client(t), newFakeUserService(u), zap.NewNop(), newTestRedisClient(t), SignInPolicy{Sessions: policy})

		step, err := flows.afterFirstFactor("flow-1", u, tc.method, tc.keepSignedIn)
		if err != nil || step.RedirectTo == "" {
			t.Fatalf("%s: step %+v, err %v", tc.name, step, err)
		}
		got := h.acceptedLogin
		if got == nil || !got.Remember {
			t.Fatalf("%s: the login was accepted as %+v, want a remembered one", tc.name, got)
		}
		if got.RememberFor != int(tc.want.Seconds()) {
			t.Errorf("%s: session lifetime = %ds, want %s", tc.name, got.RememberFor, tc.want)
		}
	}
}

// The choice made with the first factor survives the second factor step.
func TestSignIn_KeepsTheChoiceAcrossTheSecondFactor(t *testing.T) {
	u := buildTestUser(t, "unused", true)
	h := &sessionHydra{}
	flows := newSignIn(h.client(t), newFakeUserService(u), zap.NewNop(), newTestRedisClient(t), SignInPolicy{})

	step, err := flows.afterFirstFactor("flow-1", u, byPassword, true)
	if err != nil || step.MFAChallenge == "" {
		t.Fatalf("step %+v, err %v, want a second factor owed", step, err)
	}
	if h.acceptedLogin != nil {
		t.Fatal("the login was accepted before the second factor")
	}
	pending, ok := flows.pending.Get(step.MFAChallenge)
	if !ok || !pending.KeepSignedIn {
		t.Errorf("parked sign-in = %+v, want the keep-me-signed-in choice kept", pending)
	}
}

func newSessionFlowApp(t *testing.T, h *sessionHydra, u *user.User, clients ...*client.Client) *fiber.App {
	t.Helper()
	handler := NewAuthHandler(newFakeUserService(u), newFakeClientService(clients...), fakeClaimsService{}, &fakeMFAService{}, h.client(t), zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
	app := fiber.New()
	app.Get("/api/v1/login-flows/:flow", handler.GetLoginFlow)
	app.Get("/consent-flows/:flow", handler.GetConsentFlow)
	app.Post("/consent-flows/:flow/accept", handler.AcceptConsent)
	return app
}

// A browser session of another tenant is not this client's to use. Asked for
// an answer without a prompt, the application is told so; asked plainly, the
// user gets the form.
func TestGetLoginFlow_AnotherTenantsSession(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "user@example.com", Active: true}
	c := &client.Client{Active: true, ID: uuid.New(), TenantID: uuid.New(), ClientID: testClientID, AllowEmailLogin: true}
	request := func(requestURL string) *sessionHydra {
		return &sessionHydra{login: hydra.LoginRequest{
			Challenge: "flow-1", Skip: true, Subject: u.ID.String(), RequestURL: requestURL,
			Client: &hydra.OAuth2Client{ClientID: testClientID, ClientName: "Test App"},
		}}
	}

	silent := request("https://issuer.example/oauth2/auth?client_id=" + testClientID + "&prompt=none")
	status, body := getJSON(t, newSessionFlowApp(t, silent, u, c), "/api/v1/login-flows/flow-1")
	if status != fiber.StatusOK || body["next"] != "redirect" || body["redirect_to"] != "https://app.example/callback?error=login_required" {
		t.Fatalf("silent request: status %d body %v, want a redirect back to the application", status, body)
	}
	if silent.rejectedLogin["error"] != "login_required" {
		t.Errorf("silent request: rejected with %v, want login_required", silent.rejectedLogin)
	}
	if silent.acceptedLogin != nil {
		t.Error("silent request: another tenant's session was accepted")
	}

	plain := request("https://issuer.example/oauth2/auth?client_id=" + testClientID)
	status, body = getJSON(t, newSessionFlowApp(t, plain, u, c), "/api/v1/login-flows/flow-1")
	if status != fiber.StatusOK || body["next"] != "form" {
		t.Fatalf("plain request: status %d body %v, want the sign-in form", status, body)
	}
	if plain.rejectedLogin != nil || plain.acceptedLogin != nil {
		t.Errorf("plain request: the flow was settled (accept %v, reject %v) before the user signed in", plain.acceptedLogin, plain.rejectedLogin)
	}
}

// Reusing a session neither lengthens it nor marks the consent step.
func TestGetLoginFlow_ReusedSessionKeepsItsLifetime(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: true}
	c := &client.Client{Active: true, ID: uuid.New(), TenantID: tenantID, ClientID: testClientID}
	h := &sessionHydra{login: hydra.LoginRequest{
		Challenge: "flow-1", Skip: true, Subject: u.ID.String(),
		Client: &hydra.OAuth2Client{ClientID: testClientID},
	}}

	status, body := getJSON(t, newSessionFlowApp(t, h, u, c), "/api/v1/login-flows/flow-1")
	if status != fiber.StatusOK || body["next"] != "redirect" {
		t.Fatalf("status %d body %v, want next=redirect", status, body)
	}
	got := h.acceptedLogin
	if got == nil || got.Subject != u.ID.String() {
		t.Fatalf("accepted %+v, want the session's user", got)
	}
	if got.Remember || got.RememberFor != 0 {
		t.Errorf("accepted with remember=%v for %ds: a reused session's lifetime was set when it was opened", got.Remember, got.RememberFor)
	}
	if _, marked := got.Context["sso"]; marked {
		t.Error("the reused session marks the consent step: arriving on a session is no reason to skip the question")
	}
}

func consentRequestFor(u *user.User, skip, clientSkips bool, context map[string]any) hydra.ConsentRequest {
	return hydra.ConsentRequest{
		Challenge: "cf-1", Subject: u.ID.String(), RequestedScope: []string{"openid", "email"},
		Client:  &hydra.OAuth2Client{ClientID: testClientID, ClientName: "App", SkipConsent: clientSkips},
		Skip:    skip,
		Context: context,
	}
}

// The authorization server reports a client registered to skip consent only
// as client.skip_consent — `skip` stays false until a consent is remembered.
func TestConsentFlow_ClientRegisteredToSkipIsNotAsked(t *testing.T) {
	u := buildTestUser(t, "unused", false)
	h := &sessionHydra{consent: consentRequestFor(u, false, true, nil)}

	status, body := doConsent(t, newSessionFlowApp(t, h, u), "GET", "/consent-flows/cf-1", "")
	if status != fiber.StatusOK || body["next"] != "redirect" {
		t.Fatalf("status %d body %v, want next=redirect", status, body)
	}
	got := h.acceptedConsent
	if got == nil || !got.Remember || got.RememberFor != int(autoConsentRememberedFor.Seconds()) {
		t.Errorf("consent accepted as %+v, want it remembered for %s — a silent request needs a remembered consent", got, autoConsentRememberedFor)
	}
}

// Arriving on an existing session says who the user is, not what they allowed
// this client.
func TestConsentFlow_ExistingSessionIsNoReasonToSkip(t *testing.T) {
	u := buildTestUser(t, "unused", false)
	h := &sessionHydra{consent: consentRequestFor(u, false, false, map[string]any{"sso": true, "prompt": "none"})}

	status, body := doConsent(t, newSessionFlowApp(t, h, u), "GET", "/consent-flows/cf-1", "")
	if status != fiber.StatusOK || body["next"] != "form" {
		t.Fatalf("status %d body %v, want the question to be asked", status, body)
	}
	if h.acceptedConsent != nil {
		t.Error("consent was given on the user's behalf")
	}
}

// An answer the user asked to be remembered is kept until it is revoked; how
// long is not the screen's to say.
func TestConsentFlow_RememberedAnswerIsKeptUntilRevoked(t *testing.T) {
	u := buildTestUser(t, "unused", false)
	for _, tc := range []struct {
		body     string
		remember bool
	}{
		{`{"grant_scope":["openid"],"remember":true,"remember_for":60}`, true},
		{`{"grant_scope":["openid"]}`, false},
	} {
		h := &sessionHydra{consent: consentRequestFor(u, false, false, nil)}
		status, body := doConsent(t, newSessionFlowApp(t, h, u), "POST", "/consent-flows/cf-1/accept", tc.body)
		if status != fiber.StatusOK || body["next"] != "redirect" {
			t.Fatalf("%s: status %d body %v", tc.body, status, body)
		}
		got := h.acceptedConsent
		if got == nil || got.Remember != tc.remember || got.RememberFor != 0 {
			t.Errorf("%s: accepted as %+v, want remember=%v with no end", tc.body, got, tc.remember)
		}
	}
}

// The same decisions, taken where the authorization server sends the browser
// first: a flow that is settled there never opens a screen.
func TestFlowSettler_SettlesWhatNeedsNoScreen(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: true}
	own := &client.Client{Active: true, ID: uuid.New(), TenantID: tenantID, ClientID: testClientID}
	foreign := &client.Client{Active: true, ID: uuid.New(), TenantID: uuid.New(), ClientID: testClientID}
	session := func(requestURL string) hydra.LoginRequest {
		return hydra.LoginRequest{
			Challenge: "flow-1", Skip: true, Subject: u.ID.String(), RequestURL: requestURL,
			Client: &hydra.OAuth2Client{ClientID: testClientID},
		}
	}
	silent := "https://issuer.example/oauth2/auth?prompt=none"

	settle := func(h *sessionHydra, c *client.Client, method string) (string, bool) {
		handler := NewAuthHandler(newFakeUserService(u), newFakeClientService(c), fakeClaimsService{}, &fakeMFAService{}, h.client(t), zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
		entry := NewFlowEntryHandler(testFrontendURL, handler)
		app := fiber.New()
		app.Get("/login", entry.Login)
		app.Get("/consent", entry.Consent)
		resp := get(t, app, method, "")
		if resp.StatusCode != fiber.StatusFound {
			t.Fatalf("%s: status %d", method, resp.StatusCode)
		}
		location := resp.Header.Get("Location")
		return location, !strings.HasPrefix(location, testFrontendURL)
	}

	for _, tc := range []struct {
		name    string
		hydra   *sessionHydra
		client  *client.Client
		path    string
		settled bool
		want    string
	}{
		{"a session the client can use", &sessionHydra{login: session("")}, own, "/login?login_challenge=flow-1", true, "https://issuer.example/after-login"},
		{"no session: the user signs in", &sessionHydra{login: hydra.LoginRequest{Challenge: "flow-1", Client: &hydra.OAuth2Client{ClientID: testClientID}}}, own, "/login?login_challenge=flow-1", false, testFrontendURL + "/login?flow=flow-1"},
		{"another tenant's session: the user signs in", &sessionHydra{login: session("")}, foreign, "/login?login_challenge=flow-1", false, testFrontendURL + "/login?flow=flow-1"},
		{"another tenant's session, asked without a prompt", &sessionHydra{login: session(silent)}, foreign, "/login?login_challenge=flow-1", true, "https://app.example/callback?error=login_required"},
		{"a client registered to skip consent", &sessionHydra{consent: consentRequestFor(u, false, true, nil)}, own, "/consent?consent_challenge=cf-1", true, "https://issuer.example/after-consent"},
		{"a remembered consent", &sessionHydra{consent: consentRequestFor(u, true, false, nil)}, own, "/consent?consent_challenge=cf-1", true, "https://issuer.example/after-consent"},
		{"a consent to ask for", &sessionHydra{consent: consentRequestFor(u, false, false, nil)}, own, "/consent?consent_challenge=cf-1", false, testFrontendURL + "/consent?flow=cf-1"},
	} {
		location, settled := settle(tc.hydra, tc.client, tc.path)
		if settled != tc.settled || location != tc.want {
			t.Errorf("%s: went to %q (settled=%v), want %q (settled=%v)", tc.name, location, settled, tc.want, tc.settled)
		}
		if !tc.settled && (tc.hydra.acceptedLogin != nil || tc.hydra.rejectedLogin != nil || tc.hydra.acceptedConsent != nil) {
			t.Errorf("%s: the flow was settled behind a screen that is still to be shown", tc.name)
		}
	}
}
