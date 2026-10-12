package handler

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/passwordless"
)

// A user who has a second factor owes it however they proved themselves
// first. These tests come in by an emailed link and by an identity provider —
// the two ways that used to end the flow on one factor — and check that the
// flow is not accepted until the code is given, and that the tokens then say
// both factors were used.

const secondFactorCode = "123456"

// newSecondFactorTestApp serves the emailed-link redeem, a provider sign-in
// that has just vouched for the user, and the second factor step — all over
// one store of parked sign-ins, as the deployed handlers are.
func newSecondFactorTestApp(t *testing.T, totpEnabled bool) (*fiber.App, *int, *fakeUserService) {
	t.Helper()
	u := buildTestUser(t, "unused", totpEnabled)
	users := newFakeUserService(u)
	clients := newFakeClientService(&client.Client{ID: uuid.New(), TenantID: u.TenantID, ClientID: testClientID})
	hydraClient, acceptCount := newTestHydraServer(t)
	redisClient := newTestRedisClient(t)

	auth := NewAuthHandler(users, clients, fakeClaimsService{}, &fakeMFAService{validTOTPCode: secondFactorCode, validRecoveryCode: "recovery-1"}, hydraClient, zap.NewNop(), nil, redisClient, allowedSignInMethods{})
	links := NewMagicLinkFlowHandler(auth, &fakeLinks{
		link: &passwordless.MagicLink{Email: u.Email, TenantID: u.TenantID, LoginFlow: "flow-1", ExpiresAt: time.Now().Add(time.Minute)},
		user: u,
	})
	social := NewSocialHandlerWithAllProviders(nil, nil, nil, nil, users, clients, hydraClient, zap.NewNop(), nil, NewOAuthStateStore(redisClient), testFrontendURL)

	app := fiber.New()
	app.Post("/magic-links/redeem", links.Redeem)
	app.Get("/provider-vouched", func(c *fiber.Ctx) error {
		return social.finishSignIn(c, "google", testClientID, "flow-1", u)
	})
	app.Post("/login-flows/:flow/mfa", auth.VerifyMFALogin)
	app.Post("/login-flows/:flow/mfa/recovery", auth.VerifyMFARecoveryLogin)
	return app, acceptCount, users
}

func TestEmailedLink_AUserWithASecondFactorOwesIt(t *testing.T) {
	app, acceptCount, users := newSecondFactorTestApp(t, true)

	status, body := doJSON(t, app, "/magic-links/redeem", `{"token":"tok"}`)
	if status != fiber.StatusOK || body["next"] != "mfa" || body["mfa_challenge"] == "" || body["flow"] != "flow-1" {
		t.Fatalf("redeem: status %d body %v, want the second factor step naming its flow", status, body)
	}
	if *acceptCount != 0 || users.signIns != 0 {
		t.Fatalf("the flow was accepted (%d) or a sign-in recorded (%d) on the link alone", *acceptCount, users.signIns)
	}
	challenge := body["mfa_challenge"].(string)

	status, body = doJSON(t, app, "/login-flows/flow-1/mfa", `{"mfa_challenge":"`+challenge+`","code":"000000"}`)
	if status != fiber.StatusUnauthorized || *acceptCount != 0 {
		t.Fatalf("wrong code: status %d body %v accepted=%d, want 401 and no sign-in", status, body, *acceptCount)
	}

	status, body = doJSON(t, app, "/login-flows/flow-1/mfa", `{"mfa_challenge":"`+challenge+`","code":"`+secondFactorCode+`"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" || *acceptCount != 1 {
		t.Fatalf("right code: status %d body %v accepted=%d, want signed in", status, body, *acceptCount)
	}
	wantAcceptedAMR(t, "email,otp,mfa")
	if users.signIns != 1 {
		t.Errorf("the sign-in was recorded as the last login %d times, want once", users.signIns)
	}
}

func TestEmailedLink_AUserWithoutASecondFactorIsSignedIn(t *testing.T) {
	app, acceptCount, users := newSecondFactorTestApp(t, false)

	status, body := doJSON(t, app, "/magic-links/redeem", `{"token":"tok"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" || *acceptCount != 1 {
		t.Fatalf("redeem: status %d body %v accepted=%d, want signed in", status, body, *acceptCount)
	}
	wantAcceptedAMR(t, "email")
	if users.signIns != 1 {
		t.Errorf("the sign-in was recorded as the last login %d times, want once", users.signIns)
	}
}

func TestProviderSignIn_AUserWithASecondFactorOwesIt(t *testing.T) {
	app, acceptCount, users := newSecondFactorTestApp(t, true)

	resp, err := app.Test(httptest.NewRequest("GET", "/provider-vouched", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status %d, want a redirect to the second factor step", resp.StatusCode)
	}
	to, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(to.String(), testFrontendURL+"/mfa/verify?") || to.Query().Get("flow") != "flow-1" || to.Query().Get("mfa_challenge") == "" {
		t.Fatalf("redirected to %q, want the sign-in screen's second factor step for the flow", to)
	}
	if *acceptCount != 0 || users.signIns != 0 {
		t.Fatalf("the flow was accepted (%d) or a sign-in recorded (%d) on the provider's word alone", *acceptCount, users.signIns)
	}

	status, body := doJSON(t, app, "/login-flows/flow-1/mfa/recovery", `{"mfa_challenge":"`+to.Query().Get("mfa_challenge")+`","code":"recovery-1"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" || *acceptCount != 1 {
		t.Fatalf("recovery code: status %d body %v accepted=%d, want signed in", status, body, *acceptCount)
	}
	wantAcceptedAMR(t, "fed,mfa")
	if users.signIns != 1 {
		t.Errorf("the sign-in was recorded as the last login %d times, want once", users.signIns)
	}
}

func TestProviderSignIn_AUserWithoutASecondFactorIsSignedIn(t *testing.T) {
	app, acceptCount, users := newSecondFactorTestApp(t, false)

	resp, err := app.Test(httptest.NewRequest("GET", "/provider-vouched", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK || *acceptCount != 1 {
		t.Fatalf("status %d accepted=%d, want the page that continues to the application", resp.StatusCode, *acceptCount)
	}
	wantAcceptedAMR(t, "fed")
	if users.signIns != 1 {
		t.Errorf("the sign-in was recorded as the last login %d times, want once", users.signIns)
	}
}

// A sign-in parked by a replica that did not yet record how the user came in
// reached the second step by password — the only way there at the time.
func TestSecondFactor_AParkedSignInWithoutAMethodCameInByPassword(t *testing.T) {
	if got := firstFactor("", "").withTOTP(); strings.Join(got.amr, ",") != "pwd,otp,mfa" || got.audit != "password+totp" {
		t.Errorf("got %+v, want password followed by totp", got)
	}
	if got := firstFactor("social", "github").withTOTP(); strings.Join(got.amr, ",") != "fed,otp,mfa" || got.provider != "github" {
		t.Errorf("got %+v, want the provider sign-in followed by totp", got)
	}
}
