package handler

import (
	"github.com/gofiber/fiber/v2"

	"authway/apps/central/api/pkg/tenant"
)

// CapabilitiesHandler answers what this deployment offers, so the login UI
// and the admin console show only features that work here. Another Authway
// implementation (for example a single-tenant one) answers the same document
// with its own values.
type CapabilitiesHandler struct {
	social *SocialHandler
}

func NewCapabilitiesHandler(social *SocialHandler) *CapabilitiesHandler {
	return &CapabilitiesHandler{social: social}
}

// Get serves GET /api/v1/capabilities.
//
//   - multi_tenant: tenants are a dimension of the admin API
//   - providers: social providers this deployment has credentials for (a
//     client may add its own credentials for some of them)
//   - magic_link: emailed sign-in links (opened in the browser that started
//     the sign-in) complete a login flow
//   - mfa: second factors a user can enrol
//   - signup_modes: the values a tenant's signup_mode accepts
//   - account_deletion: a signed-in user can delete their own account
//     (DELETE /api/v1/profile/me)
//   - custom_claims: claims stored for a user by an administrator or a
//     tenant's service credential go into the user's tokens
//     (GET/PATCH /api/v1/users/{id}/claims)
//   - backchannel_logout: a client that registers backchannel_logout_uri is
//     told there when a user's session ends (OpenID Connect Back-Channel
//     Logout 1.0)
//   - token_introspection: a confidential client can ask whether a token of
//     its tenant is still good (POST /api/v1/oauth2/introspect, RFC 7662)
//   - device_authorization: a device without a usable browser or keyboard
//     signs in with a code the user types elsewhere (RFC 8628, discovery's
//     device_authorization_endpoint; clients register the device_code grant)
//   - terms_acceptance: a tenant names documents its users accept at
//     sign-in; a login flow owes the terms step until they have accepted the
//     current versions (/api/v1/tenants/{id}/terms)
//   - token_exchange, ciba: delegated tokens (RFC 8693) and decoupled
//     approval (OpenID CIBA) — not offered
func (h *CapabilitiesHandler) Get(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"multi_tenant":         true,
		"providers":            h.social.ConfiguredProviders(),
		"magic_link":           true,
		"mfa":                  []string{"totp"},
		"signup_modes":         []string{tenant.SignupModeInviteOnly, tenant.SignupModeOpen},
		"account_deletion":     true,
		"custom_claims":        true,
		"backchannel_logout":   true,
		"token_introspection":  true,
		"device_authorization": true,
		"terms_acceptance":     true,
		"token_exchange":       false,
		"ciba":                 false,
	})
}
