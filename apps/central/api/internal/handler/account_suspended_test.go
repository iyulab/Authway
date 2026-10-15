package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// An account an administrator suspended does not sign in, whichever way it
// comes: the flow is not accepted, and the refusal comes only after the user
// proved who they are.

func TestSuspendedAccount_PasswordIsRefusedAfterItIsChecked(t *testing.T) {
	for _, withSecondFactor := range []bool{false, true} {
		app, h, acceptCount := newAuthTestApp(t, "correct-horse", withSecondFactor, "123456", "")
		h.userService.(*fakeUserService).users[0].Active = false

		status, body := doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"wrong"}`)
		if status != fiber.StatusUnauthorized {
			t.Errorf("second factor %v, wrong password: status %d body %v, want the usual 401 — nothing about the account", withSecondFactor, status, body)
		}
		status, body = doJSON(t, app, "/login-flows/c1/password", `{"email":"user@example.com","password":"correct-horse"}`)
		if status != fiber.StatusForbidden || body["code"] != "account_suspended" || *acceptCount != 0 {
			t.Errorf("second factor %v: status %d body %v accepted=%d, want 403 account_suspended and no sign-in", withSecondFactor, status, body, *acceptCount)
		}
	}
}

func TestSuspendedAccount_EmailedLinkAndProviderAreRefused(t *testing.T) {
	app, acceptCount, users := newSecondFactorTestApp(t, false)
	users.users[0].Active = false

	status, body := doJSON(t, app, "/magic-links/redeem", `{"token":"tok"}`)
	if status != fiber.StatusForbidden || body["code"] != "account_suspended" {
		t.Errorf("emailed link: status %d body %v, want 403 account_suspended", status, body)
	}

	resp, err := app.Test(httptest.NewRequest("GET", "/provider-vouched", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Errorf("provider: status %d, want the sign-in ended with a redirect", resp.StatusCode)
	}
	if *acceptCount != 0 || users.signIns != 0 {
		t.Errorf("accepted=%d sign-ins=%d, want none", *acceptCount, users.signIns)
	}
}

func TestSuspendedAccount_ASessionIsNotReused(t *testing.T) {
	tenantID := uuid.New()
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Active: false}
	c := &client.Client{ID: uuid.New(), TenantID: tenantID, ClientID: testClientID}
	h := &loginFlowHydra{skip: true, subject: u.ID.String()}
	app := newLoginFlowApp(t, h, u, c)

	status, body := getJSON(t, app, "/api/v1/login-flows/flow-1")
	if status != fiber.StatusForbidden || body["code"] != "account_suspended" || h.acceptCount != 0 {
		t.Errorf("status %d body %v accepts=%d, want 403 account_suspended and no sign-in", status, body, h.acceptCount)
	}
}
