package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
)

// What the sign-in screen does after a flow endpoint answers. Every login-flow
// response carries one of these in "next".
const (
	nextForm     = "form"     // show the sign-in form
	nextRedirect = "redirect" // navigate to redirect_to
	nextMFA      = "mfa"      // ask for the second factor, quoting mfa_challenge
)

// flowParam reads the flow id from the route. Fiber does not percent-decode
// path parameters, and Hydra challenges end in "=" (sent as %3D), so every
// flow route reads it through here.
func flowParam(c *fiber.Ctx) string {
	flow := c.Params("flow")
	if decoded, err := url.PathUnescape(flow); err == nil {
		return decoded
	}
	return flow
}
