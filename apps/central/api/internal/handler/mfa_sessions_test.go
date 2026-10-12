package handler

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// fakeSessions records what a handler asked to be ended.
type fakeSessions struct {
	calls [][2]string // subject, keep
	err   error
}

func (f *fakeSessions) RevokeOtherSessions(subject, keep string) error {
	f.calls = append(f.calls, [2]string{subject, keep})
	return f.err
}

// A session opened before the second factor was on was opened on one factor.
// Turning the factor on ends those — and keeps the one that just gave a code,
// whose answer carries the recovery codes.
func TestEnablingTheSecondFactor_EndsTheUsersOtherSessions(t *testing.T) {
	userID := uuid.New()
	enable := func(sessionID string, sessions *fakeSessions) (int, map[string]any) {
		h := NewMFAHandler(&fakeMFAService{validTOTPCode: "123456"}, nil, zap.NewNop(), nil, sessions)
		app := fiber.New()
		app.Use(func(c *fiber.Ctx) error {
			c.Locals("user_id", userID)
			if sessionID != "" {
				c.Locals("session_id", sessionID)
			}
			return c.Next()
		})
		app.Post("/users/mfa/verify", h.VerifyMFA)
		return doJSON(t, app, "/users/mfa/verify", `{"code":"123456"}`)
	}

	sessions := &fakeSessions{}
	status, body := enable("session-1", sessions)
	if status != fiber.StatusOK || body["sessions_ended"] != "others" || body["recovery_codes"] == nil {
		t.Fatalf("status %d body %v, want the recovery codes and the other sessions ended", status, body)
	}
	if len(sessions.calls) != 1 || sessions.calls[0] != [2]string{userID.String(), "session-1"} {
		t.Errorf("asked to end %v, want the user's sessions but session-1", sessions.calls)
	}

	// A token that does not name its session cannot be kept apart: all end.
	sessions = &fakeSessions{}
	if _, body := enable("", sessions); body["sessions_ended"] != "all" || sessions.calls[0][1] != "" {
		t.Errorf("body %v calls %v, want every session ended", body, sessions.calls)
	}

	// The factor is on whether or not the sessions could be ended; the answer says so.
	status, body = enable("session-1", &fakeSessions{err: errors.New("authorization server unreachable")})
	if status != fiber.StatusOK || body["sessions_ended"] != "none" || body["recovery_codes"] == nil {
		t.Errorf("status %d body %v, want the recovery codes and sessions_ended=none", status, body)
	}

	// A wrong code turns nothing on and ends nothing.
	sessions = &fakeSessions{}
	h := NewMFAHandler(&fakeMFAService{validTOTPCode: "123456"}, nil, zap.NewNop(), nil, sessions)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", userID); return c.Next() })
	app.Post("/users/mfa/verify", h.VerifyMFA)
	if status, _ := doJSON(t, app, "/users/mfa/verify", `{"code":"000000"}`); status == fiber.StatusOK || len(sessions.calls) != 0 {
		t.Errorf("a wrong code: status %d, sessions ended %v", status, sessions.calls)
	}
}
