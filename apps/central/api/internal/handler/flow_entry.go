package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// FlowEntryHandler is where the authorization server sends the browser for
// login, consent and logout. A flow that needs nothing from the user is
// settled here and the browser goes straight back; any other is handed to the
// login UI with an opaque flow id, so the UI never sees the authorization
// server's own parameter names and works unchanged with any backend that
// issues flow ids.
type FlowEntryHandler struct {
	frontendURL string
	settle      FlowSettler
}

// FlowSettler settles the flows that need no screen: a login in a browser
// that already has a session the client can use, a consent that is not to be
// asked for. Each method returns where the browser goes next, and false when
// the screen has to be opened.
type FlowSettler interface {
	LoginWithoutScreen(c *fiber.Ctx, flow string) (redirectTo string, settled bool)
	ConsentWithoutScreen(c *fiber.Ctx, flow string) (redirectTo string, settled bool)
}

// NewFlowEntryHandler hands every flow to the login UI at frontendURL, except
// those settle can finish on its own. settle may be nil.
func NewFlowEntryHandler(frontendURL string, settle FlowSettler) *FlowEntryHandler {
	return &FlowEntryHandler{frontendURL: strings.TrimRight(frontendURL, "/"), settle: settle}
}

// Login serves GET /login?login_challenge=… (Hydra's URLS_LOGIN).
func (h *FlowEntryHandler) Login(c *fiber.Ctx) error {
	var settle func(*fiber.Ctx, string) (string, bool)
	if h.settle != nil {
		settle = h.settle.LoginWithoutScreen
	}
	return h.enter(c, "login_challenge", "/login", settle)
}

// Consent serves GET /consent?consent_challenge=… (Hydra's URLS_CONSENT).
func (h *FlowEntryHandler) Consent(c *fiber.Ctx) error {
	var settle func(*fiber.Ctx, string) (string, bool)
	if h.settle != nil {
		settle = h.settle.ConsentWithoutScreen
	}
	return h.enter(c, "consent_challenge", "/consent", settle)
}

// Device serves GET /device?device_challenge=…[&user_code=…] (Hydra's
// URLS_DEVICE_VERIFICATION): a user signing in a device (RFC 8628) is handed
// to the login UI's code page. A user code the device's complete
// verification URI carried goes along — it is the code the user is about to
// type, shown to them, not a secret.
func (h *FlowEntryHandler) Device(c *fiber.Ctx) error {
	flow := string([]byte(c.Query("device_challenge")))
	if flow == "" {
		q := url.Values{"error": {"invalid_request"}, "error_description": {msgInvalidSignIn}}
		return c.Redirect(h.frontendURL+"/error?"+q.Encode(), fiber.StatusFound)
	}
	q := url.Values{"flow": {flow}}
	if code := c.Query("user_code"); code != "" {
		q.Set("user_code", code)
	}
	return c.Redirect(h.frontendURL+"/device?"+q.Encode(), fiber.StatusFound)
}

// Logout serves GET /logout?logout_challenge=… (Hydra's URLS_LOGOUT).
func (h *FlowEntryHandler) Logout(c *fiber.Ctx) error {
	return h.enter(c, "logout_challenge", "/logout", nil)
}

func (h *FlowEntryHandler) enter(c *fiber.Ctx, challengeParam, screen string, settle func(*fiber.Ctx, string) (string, bool)) error {
	flow := string([]byte(c.Query(challengeParam)))
	if flow == "" {
		q := url.Values{"error": {"invalid_request"}, "error_description": {msgInvalidSignIn}}
		return c.Redirect(h.frontendURL+"/error?"+q.Encode(), fiber.StatusFound)
	}
	if settle != nil {
		if redirectTo, settled := settle(c, flow); settled {
			return c.Redirect(redirectTo, fiber.StatusFound)
		}
	}
	return c.Redirect(h.frontendURL+screen+"?"+url.Values{"flow": {flow}}.Encode(), fiber.StatusFound)
}
