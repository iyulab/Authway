package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/passwordless"
	"authway/apps/central/api/pkg/user"
)

// fakeLinks stands in for passwordless.Service: one link, bound to a flow.
type fakeLinks struct {
	sentTenant uuid.UUID
	sentEmail  string
	sentFlow   string
	link       *passwordless.MagicLink
	user       *user.User
	redeemed   bool
}

func (f *fakeLinks) SendMagicLink(tenantID uuid.UUID, email, loginFlow, _, _ string) (*passwordless.MagicLinkResponse, error) {
	f.sentTenant, f.sentEmail, f.sentFlow = tenantID, email, loginFlow
	return &passwordless.MagicLinkResponse{}, nil
}

func (f *fakeLinks) VerifyMagicLink(token string) (*passwordless.MagicLink, *user.User, error) {
	if token != "tok" || f.redeemed {
		return nil, nil, apierror.NewPublic("invalid, expired or already used token")
	}
	f.redeemed = true
	return f.link, f.user, nil
}

func (f *fakeLinks) VerifyMagicLinkCode(loginFlow, code string) (*passwordless.MagicLink, *user.User, error) {
	if loginFlow != f.link.LoginFlow || code != "BCDF-GHJK" || f.redeemed {
		return nil, nil, apierror.NewPublic("this code is not valid; check it, or ask for a new email")
	}
	f.redeemed = true
	return f.link, f.user, nil
}

func (f *fakeLinks) InspectMagicLink(token string) (*passwordless.MagicLink, error) {
	if token != "tok" {
		return nil, errors.New("not found")
	}
	return f.link, nil
}

func (f *fakeLinks) CleanupExpired() (int64, error) { return 0, nil }

func newMagicLinkTestApp(t *testing.T, allowed []string) (*fiber.App, *fakeLinks, *client.Client, *int) {
	t.Helper()
	u := buildTestUser(t, "unused", false)
	cl := &client.Client{Active: true, ID: uuid.New(), TenantID: u.TenantID, ClientID: testClientID, EnabledAuthProviders: allowed}
	hydraClient, acceptCount := newTestHydraServer(t)
	auth := NewAuthHandler(newFakeUserService(u), newFakeClientService(cl), fakeClaimsService{}, &fakeMFAService{}, hydraClient, zap.NewNop(), nil, newTestRedisClient(t), allowedSignInMethods{}, SignInPolicy{})
	links := &fakeLinks{
		link: &passwordless.MagicLink{Email: u.Email, TenantID: u.TenantID, LoginFlow: "flow-1=", ExpiresAt: time.Now().Add(time.Minute)},
		user: u,
	}
	h := NewMagicLinkFlowHandler(auth, links)
	app := fiber.New()
	app.Post("/login-flows/:flow/magic-link", h.Send)
	app.Post("/magic-links/inspect", h.Inspect)
	app.Post("/magic-links/redeem", h.Redeem)
	app.Post("/login-flows/:flow/magic-link/code", h.RedeemCode)
	return app, links, cl, acceptCount
}

func TestMagicLink_SendIsScopedToTheFlowsClient(t *testing.T) {
	app, links, cl, _ := newMagicLinkTestApp(t, []string{"email", "magic_link"})

	status, body := doJSON(t, app, "/login-flows/flow-1%3D/magic-link", `{"email":"someone@example.com"}`)
	if status != fiber.StatusOK || body["next"] != "email_sent" {
		t.Fatalf("status %d body %v, want next=email_sent", status, body)
	}
	if links.sentTenant != cl.TenantID || links.sentFlow != "flow-1=" || links.sentEmail != "someone@example.com" {
		t.Errorf("sent tenant=%v flow=%q email=%q, want the client's tenant and the decoded flow", links.sentTenant, links.sentFlow, links.sentEmail)
	}
}

func TestMagicLink_SendRefusedWhenTheClientDoesNotOfferIt(t *testing.T) {
	app, links, _, _ := newMagicLinkTestApp(t, []string{"email"})

	status, body := doJSON(t, app, "/login-flows/flow-1%3D/magic-link", `{"email":"someone@example.com"}`)
	if status != fiber.StatusForbidden || body["code"] != "sign_in_method_not_allowed" {
		t.Fatalf("status %d body %v, want 403 sign_in_method_not_allowed", status, body)
	}
	if links.sentEmail != "" {
		t.Errorf("a link was sent to %q", links.sentEmail)
	}
}

func TestMagicLink_InspectDoesNotRedeem_RedeemCompletesTheFlowOnce(t *testing.T) {
	app, links, _, acceptCount := newMagicLinkTestApp(t, []string{"magic_link"})

	status, body := doJSON(t, app, "/magic-links/inspect", `{"token":"tok"}`)
	if status != fiber.StatusOK || body["valid"] != true || links.redeemed {
		t.Fatalf("inspect: status %d body %v redeemed=%v", status, body, links.redeemed)
	}
	// The page compares it with the flows this browser started.
	if body["flow"] != "flow-1=" {
		t.Errorf("inspect flow = %v, want the link's login flow", body["flow"])
	}

	status, body = doJSON(t, app, "/magic-links/redeem", `{"token":"tok"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" || body["redirect_to"] != "https://example.com/callback" {
		t.Fatalf("redeem: status %d body %v", status, body)
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}

	status, body = doJSON(t, app, "/magic-links/redeem", `{"token":"tok"}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_link" {
		t.Fatalf("second redeem: status %d body %v, want 400 invalid_link", status, body)
	}
}

// The code completes the flow it was sent for, once, like the link; a wrong
// one is refused with invalid_code and does not sign anyone in.
func TestMagicLink_CodeCompletesItsFlowOnce(t *testing.T) {
	app, _, _, acceptCount := newMagicLinkTestApp(t, []string{"magic_link"})

	status, body := doJSON(t, app, "/login-flows/flow-1%3D/magic-link/code", `{"code":"BCDF-GHJX"}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_code" {
		t.Fatalf("wrong code: status %d body %v, want 400 invalid_code", status, body)
	}
	status, body = doJSON(t, app, "/login-flows/other-flow/magic-link/code", `{"code":"BCDF-GHJK"}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_code" {
		t.Fatalf("another flow: status %d body %v, want 400 invalid_code", status, body)
	}
	status, body = doJSON(t, app, "/login-flows/flow-1%3D/magic-link/code", `{}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_request" {
		t.Fatalf("no code: status %d body %v, want 400 invalid_request", status, body)
	}
	if *acceptCount != 0 {
		t.Fatalf("hydra accept called %d times before the right code", *acceptCount)
	}

	status, body = doJSON(t, app, "/login-flows/flow-1%3D/magic-link/code", `{"code":"BCDF-GHJK"}`)
	if status != fiber.StatusOK || body["next"] != "redirect" {
		t.Fatalf("right code: status %d body %v", status, body)
	}
	status, body = doJSON(t, app, "/login-flows/flow-1%3D/magic-link/code", `{"code":"BCDF-GHJK"}`)
	if status != fiber.StatusBadRequest || body["code"] != "invalid_code" {
		t.Fatalf("second use: status %d body %v, want 400 invalid_code", status, body)
	}
	if *acceptCount != 1 {
		t.Errorf("hydra accept called %d times, want 1", *acceptCount)
	}
}
