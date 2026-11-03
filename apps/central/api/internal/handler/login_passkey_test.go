package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/passkey"
	"authway/apps/central/api/pkg/passkey/passkeytest"
	"authway/apps/central/api/pkg/user"
)

const (
	loginRPID   = "auth.example.com"
	loginOrigin = "https://auth.example.com"
)

type passkeyLogin struct {
	app     *fiber.App
	dev     *passkeytest.Device
	user    *user.User
	accepts *int
	ceremon *passkey.Ceremonies
}

// newPasskeyLoginApp mounts the sign-in endpoints over a fake authorization
// server, with a user who has registered one passkey. clientTenant is the
// tenant the flow's client belongs to; enabled the client's sign-in methods
// (nil: the default); ceremonies off leaves passkeys unset up.
func newPasskeyLoginApp(t *testing.T, totp bool, clientTenant *uuid.UUID, enabled []string, setUp bool) passkeyLogin {
	t.Helper()
	u := buildTestUser(t, "correct-horse", totp)
	tenant := u.TenantID
	if clientTenant != nil {
		tenant = *clientTenant
	}
	users := newFakeUserService(u)
	cl := &client.Client{Active: true, ID: uuid.New(), TenantID: tenant, ClientID: testClientID, EnabledAuthProviders: enabled}
	hydraClient, accepts := newTestHydraServer(t)
	h := NewAuthHandler(users, newFakeClientService(cl), fakeClaimsService{}, &fakeMFAService{validTOTPCode: "123456"}, hydraClient, zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})

	keys := &fakePasskeys{}
	var ceremonies *passkey.Ceremonies
	var dev *passkeytest.Device
	if setUp {
		var err error
		ceremonies, err = passkey.New(passkey.Config{RPID: loginRPID, RPName: "Authway", Origins: []string{loginOrigin}}, ephemeral.NewMemory(context.Background()), keys, users)
		if err != nil {
			t.Fatal(err)
		}
		h.GrantsPasskeys(ceremonies)

		// The user registers a passkey on their device, in the tenant they are of.
		dev = passkeytest.New(t)
		reg, err := ceremonies.BeginRegistration(context.Background(), u, u.Email)
		if err != nil {
			t.Fatal(err)
		}
		options, _ := json.Marshal(reg.Options)
		if _, err := ceremonies.FinishRegistration(context.Background(), u, reg.CeremonyID, dev.Register(t, options, passkeytest.GoodRegistration(loginOrigin, loginRPID, options)), "device"); err != nil {
			t.Fatal(err)
		}
	}

	app := fiber.New()
	app.Get("/login-flows/:flow", h.GetLoginFlow)
	app.Post("/login-flows/:flow/passkey/begin", h.BeginPasskeyLogin)
	app.Post("/login-flows/:flow/passkey/finish", h.FinishPasskeyLogin)
	app.Post("/login-flows/:flow/mfa", h.VerifyMFALogin)
	return passkeyLogin{app: app, dev: dev, user: u, accepts: accepts, ceremon: ceremonies}
}

// begin starts a sign-in on flow and returns the ceremony id and the options.
func (p passkeyLogin) begin(t *testing.T, flow string) (string, []byte) {
	t.Helper()
	status, body := doJSON(t, p.app, "/login-flows/"+flow+"/passkey/begin", `{}`)
	if status != 200 {
		t.Fatalf("begin: %d %v", status, body)
	}
	options, _ := json.Marshal(body["options"])
	return body["ceremony_id"].(string), options
}

func finishBody(ceremony string, credential []byte) string {
	out, _ := json.Marshal(map[string]any{"ceremony_id": ceremony, "credential": json.RawMessage(credential)})
	return string(out)
}

func TestPasskeyLogin_SignsInWithoutAnyoneNamingThemselves(t *testing.T) {
	p := newPasskeyLoginApp(t, false, nil, nil, true)
	id, options := p.begin(t, "flow-1")

	status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish",
		finishBody(id, p.dev.SignIn(t, p.dev.GoodSignIn(loginOrigin, loginRPID, options))))
	if status != 200 || body["next"] != "redirect" {
		t.Fatalf("finish: %d %v, want the flow accepted", status, body)
	}
	if *p.accepts != 1 {
		t.Fatalf("the authorization server accepted %d logins, want 1", *p.accepts)
	}
	// The key proved possession and the device verified the user.
	wantAcceptedAMR(t, "hwk,mfa")
}

// A passkey is a first factor: a user with an authenticator app is still asked
// for the code, and the amr says both.
func TestPasskeyLogin_ASecondFactorIsStillOwed(t *testing.T) {
	p := newPasskeyLoginApp(t, true, nil, nil, true)
	id, options := p.begin(t, "flow-1")

	status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish",
		finishBody(id, p.dev.SignIn(t, p.dev.GoodSignIn(loginOrigin, loginRPID, options))))
	if status != 200 || body["next"] != "mfa" || body["mfa_challenge"] == "" {
		t.Fatalf("finish: %d %v, want the second factor asked", status, body)
	}
	if *p.accepts != 0 {
		t.Fatalf("the login was accepted (%d) before the second factor", *p.accepts)
	}
	challenge, _ := body["mfa_challenge"].(string)
	status, body = doJSON(t, p.app, "/login-flows/flow-1/mfa", `{"mfa_challenge":"`+challenge+`","code":"123456"}`)
	if status != 200 || body["next"] != "redirect" || *p.accepts != 1 {
		t.Fatalf("mfa: %d %v accepts=%d, want the flow accepted", status, body, *p.accepts)
	}
	// mfa is said once.
	wantAcceptedAMR(t, "hwk,mfa,otp")
}

func TestPasskeyLogin_ASuspendedAccountDoesNotSignIn(t *testing.T) {
	p := newPasskeyLoginApp(t, false, nil, nil, true)
	p.user.Active = false
	id, options := p.begin(t, "flow-1")

	status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish",
		finishBody(id, p.dev.SignIn(t, p.dev.GoodSignIn(loginOrigin, loginRPID, options))))
	if body["next"] == "redirect" || *p.accepts != 0 {
		t.Fatalf("a suspended account signed in: %d %v accepts=%d", status, body, *p.accepts)
	}
}

// The login screen serves every tenant, so the browser offers this device's
// passkey on any tenant's flow. The server opens only its own tenant's.
func TestPasskeyLogin_APasskeyOpensOnlyItsOwnTenantsFlow(t *testing.T) {
	other := uuid.New()
	p := newPasskeyLoginApp(t, false, &other, nil, true)
	id, options := p.begin(t, "flow-1")

	status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish",
		finishBody(id, p.dev.SignIn(t, p.dev.GoodSignIn(loginOrigin, loginRPID, options))))
	if status != 401 || body["code"] != "passkey_rejected" || *p.accepts != 0 {
		t.Fatalf("finish: %d %v accepts=%d, want 401 passkey_rejected and nothing accepted", status, body, *p.accepts)
	}
}

func TestPasskeyLogin_RefusesWhatIsNotAnAnswerToTheCeremony(t *testing.T) {
	p := newPasskeyLoginApp(t, false, nil, nil, true)

	if status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish", `{"ceremony_id":"x"}`); status != 400 || body["code"] != "invalid_request" {
		t.Errorf("no credential: %d %v, want 400 invalid_request", status, body)
	}
	if status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish", finishBody("never-started", []byte(`{}`))); status != 400 || body["code"] != "ceremony_expired" {
		t.Errorf("unknown ceremony: %d %v, want 400 ceremony_expired", status, body)
	}

	id, options := p.begin(t, "flow-1")
	ans := p.dev.GoodSignIn("https://evil.example", loginRPID, options)
	status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/finish", finishBody(id, p.dev.SignIn(t, ans)))
	if status != 401 || body["code"] != "passkey_rejected" {
		t.Errorf("an answer from another origin: %d %v, want 401 passkey_rejected", status, body)
	}
	// What the caller learns is the same for every refusal: no reason in it.
	if detail, _ := body["detail"].(string); strings.Contains(detail, "origin") || strings.Contains(detail, "challenge") {
		t.Errorf("the refusal says why: %q", detail)
	}
	if *p.accepts != 0 {
		t.Errorf("accepted %d logins, want none", *p.accepts)
	}
}

func TestPasskeyLogin_FollowsWhatTheClientOffers(t *testing.T) {
	cases := map[string]struct {
		enabled []string
		setUp   bool
		status  int
		code    string
	}{
		"the default offers it (the password form is there)": {enabled: nil, setUp: true, status: 200},
		"email offers it":                        {enabled: []string{"email"}, setUp: true, status: 200},
		"naming passkey offers it":               {enabled: []string{"passkey"}, setUp: true, status: 200},
		"single sign-on only does not":           {enabled: []string{"google"}, setUp: true, status: 403, code: "sign_in_method_not_allowed"},
		"where passkeys are not set up, nothing": {enabled: nil, setUp: false, status: 404, code: "passkeys_unavailable"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPasskeyLoginApp(t, false, nil, tc.enabled, tc.setUp)
			status, body := doJSON(t, p.app, "/login-flows/flow-1/passkey/begin", `{}`)
			if status != tc.status || (tc.code != "" && body["code"] != tc.code) {
				t.Fatalf("begin: %d %v, want %d %s", status, body, tc.status, tc.code)
			}
			// And finishing is as closed as beginning.
			if tc.status != 200 {
				status, body = doJSON(t, p.app, "/login-flows/flow-1/passkey/finish", `{"ceremony_id":"x","credential":{}}`)
				if status != tc.status {
					t.Fatalf("finish: %d %v, want %d", status, body, tc.status)
				}
			}
		})
	}
}

// The screen offers a passkey where the server would take one.
func TestPasskeyLogin_OfferedByTheSameRuleAsAccepted(t *testing.T) {
	for _, tc := range []struct {
		enabled []string
		want    bool
	}{
		{nil, true}, {[]string{"email"}, true}, {[]string{"passkey"}, true}, {[]string{"google"}, false}, {[]string{"magic_link", "google"}, false},
	} {
		cl := &client.Client{EnabledAuthProviders: tc.enabled}
		if got := cl.AllowsPasskey(); got != tc.want {
			t.Errorf("AllowsPasskey with %v = %v, want %v", tc.enabled, got, tc.want)
		}
	}
}

// The sign-in screen is told to offer a passkey exactly where the server takes
// one.
func TestPasskeyLogin_TheScreenIsToldWhatToOffer(t *testing.T) {
	for name, tc := range map[string]struct {
		enabled []string
		setUp   bool
		want    bool
	}{
		"offered by default":     {nil, true, true},
		"offered where named":    {[]string{"passkey"}, true, true},
		"not for single sign-on": {[]string{"google"}, true, false},
		"not where not set up":   {nil, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPasskeyLoginApp(t, false, nil, tc.enabled, tc.setUp)
			res, err := p.app.Test(httptest.NewRequest("GET", "/login-flows/flow-1", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Client struct {
					Methods []string `json:"sign_in_methods"`
				} `json:"client"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			offered := false
			for _, m := range body.Client.Methods {
				offered = offered || m == "passkey"
			}
			if offered != tc.want {
				t.Errorf("sign_in_methods = %v, passkey offered = %v, want %v", body.Client.Methods, offered, tc.want)
			}
		})
	}
}
