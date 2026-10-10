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
//   - token_exchange, ciba: delegated tokens (RFC 8693) and decoupled
//     approval (OpenID CIBA) — not offered
func (h *CapabilitiesHandler) Get(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"multi_tenant":     true,
		"providers":        h.social.ConfiguredProviders(),
		"magic_link":       true,
		"mfa":              []string{"totp"},
		"signup_modes":     []string{tenant.SignupModeInviteOnly, tenant.SignupModeOpen},
		"account_deletion": true,
		"token_exchange":   false,
		"ciba":             false,
	})
}
