package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// FlowEntryHandler is where the authorization server sends the browser to
// start a login. It hands the login UI an opaque flow id, so the UI never
// sees the authorization server's own parameter names and works unchanged
// with any backend that issues flow ids.
type FlowEntryHandler struct {
	frontendURL string
}

func NewFlowEntryHandler(frontendURL string) *FlowEntryHandler {
	return &FlowEntryHandler{frontendURL: strings.TrimRight(frontendURL, "/")}
}

// Login serves GET /login?login_challenge=… (Hydra's URLS_LOGIN).
func (h *FlowEntryHandler) Login(c *fiber.Ctx) error {
	return h.enter(c, "login_challenge", "/login")
}

func (h *FlowEntryHandler) enter(c *fiber.Ctx, challengeParam, screen string) error {
	flow := string([]byte(c.Query(challengeParam)))
	if flow == "" {
		q := url.Values{"error": {"invalid_request"}, "error_description": {msgInvalidSignIn}}
		return c.Redirect(h.frontendURL+"/error?"+q.Encode(), fiber.StatusFound)
	}
	return c.Redirect(h.frontendURL+screen+"?"+url.Values{"flow": {flow}}.Encode(), fiber.StatusFound)
}
