package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/user"
)

// passwordUsers changes passwords the way the real service decides: the
// current one must match an account that has one.
type passwordUsers struct {
	*fakeUserService
	changed []string
}

func (p *passwordUsers) ChangePassword(id uuid.UUID, req *user.ChangePasswordRequest) error {
	u, err := p.GetByID(id)
	if err != nil {
		return err
	}
	if u.PasswordHash != "" && req.CurrentPassword != "right" {
		return user.ErrWrongPassword
	}
	if len(req.NewPassword) < user.MinPasswordLength {
		return &user.PasswordError{MinLength: user.MinPasswordLength}
	}
	p.changed = append(p.changed, req.NewPassword)
	return nil
}

func changePasswordApp(t *testing.T, h *AuthHandler, userID uuid.UUID, authTime time.Time, session string) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Post("/users/password", func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		if !authTime.IsZero() {
			c.Locals("auth_time", authTime)
		}
		if session != "" {
			c.Locals("session_id", session)
		}
		return c.Next()
	}, h.ChangePassword)
	return app
}

func postPassword(t *testing.T, app *fiber.App, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/users/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestChangePassword_NeedsTheCurrentOne_ThenEndsTheOtherSessions(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "member@example.com", PasswordHash: "set"}
	log := &eventLog{}
	users := &passwordUsers{fakeUserService: newFakeUserService(u)}
	h := &AuthHandler{userService: users, hydraClient: revokingHydra(t, log, http.StatusNoContent), logger: zap.NewNop()}
	// An old token — the current password, not a recent sign-in, is what counts.
	app := changePasswordApp(t, h, u.ID, time.Now().Add(-24*time.Hour), "")

	if status, body := postPassword(t, app, `{"current_password":"wrong","new_password":"new-password-1"}`); status != 400 || body["code"] != "invalid_current_password" {
		t.Fatalf("wrong current password: %d %v, want 400 invalid_current_password", status, body)
	}
	if status, body := postPassword(t, app, `{"current_password":"right","new_password":"short"}`); status != 400 || body["code"] != "password_too_short" || body["min_length"] != float64(8) {
		t.Fatalf("short new password: %d %v, want 400 password_too_short with min_length", status, body)
	}
	if status, body := postPassword(t, app, `{"current_password":"right"}`); status != 400 || body["code"] != "invalid_request" {
		t.Fatalf("no new password: %d %v, want 400 invalid_request", status, body)
	}
	if len(log.events) != 0 || len(users.changed) != 0 {
		t.Fatalf("a refused change changed something: %v %v", users.changed, log.events)
	}

	status, body := postPassword(t, app, `{"current_password":"right","new_password":"new-password-1"}`)
	if status != 200 || body["sessions_ended"] != "all" {
		t.Fatalf("change: %d %v, want 200 sessions_ended=all (the token names no session)", status, body)
	}
	if len(users.changed) != 1 || len(log.events) == 0 {
		t.Errorf("changed %v, sessions ended %v", users.changed, log.events)
	}
}

// An account that signs in by link or another provider has no password to
// prove; it sets its first one with a recent sign-in instead.
func TestChangePassword_FirstPasswordNeedsARecentSignIn(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "linked@example.com"}
	log := &eventLog{}
	users := &passwordUsers{fakeUserService: newFakeUserService(u)}
	h := &AuthHandler{userService: users, hydraClient: revokingHydra(t, log, http.StatusNoContent), logger: zap.NewNop()}

	stale := changePasswordApp(t, h, u.ID, time.Now().Add(-time.Hour), "")
	if status, body := postPassword(t, stale, `{"new_password":"first-password"}`); status != 401 || body["code"] != "insufficient_user_authentication" {
		t.Fatalf("stale sign-in: %d %v, want 401 insufficient_user_authentication", status, body)
	}
	if len(users.changed) != 0 {
		t.Fatal("a stale token set a password")
	}

	recent := changePasswordApp(t, h, u.ID, time.Now().Add(-time.Minute), "")
	if status, body := postPassword(t, recent, `{"new_password":"first-password"}`); status != 200 {
		t.Fatalf("recent sign-in: %d %v, want 200", status, body)
	}
}
