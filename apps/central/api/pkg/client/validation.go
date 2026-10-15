package client

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"authway/apps/central/api/internal/hydra"
)

// ConfigError represents a structured client-configuration validation failure.
// The Code field is stable and machine-readable; UI/SDKs key on it.
type ConfigError struct {
	Code    string `json:"code"`    // e.g. "public_with_secret", "confidential_without_credentials"
	Field   string `json:"field"`   // e.g. "client_secret", "grant_types"
	Message string `json:"message"` // human-readable
	Hint    string `json:"hint"`    // remediation guidance
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %s (field=%s, hint=%s)", e.Code, e.Message, e.Field, e.Hint)
}

// validateClientConfig checks the combination of public/grant_types/secret/origins
// for internally inconsistent setups. Returns the first error found.
//
// This catches the ASP.NET-on-PKCE foot-gun (issue
// `aspnet-confidential-guidance-gap`): a server-side web app registered as a
// `public` client appears to work through the consent screen, then fails at
// `/signin-oidc` with Hydra `invalid_client` because ASP.NET sends
// `client_secret_post` while the registered method is `none`. Catching the
// mismatch at registration is much cheaper than debugging the runtime symptom.
func validateClientConfig(public bool, clientSecret string, grantTypes []string, redirectURIs []string, allowedOrigins []string) *ConfigError {
	gt := normalizeGrantTypes(grantTypes)

	if cerr := validateGrantTypes(gt); cerr != nil {
		return cerr
	}
	if cerr := validateRedirectURIForm(redirectURIs); cerr != nil {
		return cerr
	}

	if public {
		// Public clients (SPA, native, mobile) must NOT carry a client_secret —
		// it's a leaked credential by definition (RFC 6749 §2.1).
		if strings.TrimSpace(clientSecret) != "" {
			return &ConfigError{
				Code:    "public_client_with_secret",
				Field:   "client_secret",
				Message: "Public clients must not be assigned a client_secret",
				Hint:    "Either set public=false (confidential, recommended for ASP.NET / Next.js / Blazor Server / any backend), or omit client_secret and use PKCE (recommended for SPA / native / mobile).",
			}
		}

		// Public clients use PKCE — the client_credentials grant requires a
		// confidential client.
		if containsAny(gt, "client_credentials") {
			return &ConfigError{
				Code:    "public_client_with_client_credentials",
				Field:   "grant_types",
				Message: "Public clients cannot use the client_credentials grant",
				Hint:    "Machine-to-machine (M2M) clients must be confidential — set public=false.",
			}
		}

		// authorization_code in a browser context needs CORS allow-list. A
		// native app has no origin to list.
		if containsAny(gt, "authorization_code") && usesBrowserRedirect(redirectURIs) && len(allowedOrigins) == 0 {
			return &ConfigError{
				Code:    "public_client_missing_allowed_origins",
				Field:   "allowed_origins",
				Message: "Public clients with authorization_code need allowed_origins for browser CORS",
				Hint:    "Add the SPA origin (e.g. https://app.example.com) so the reverse proxy can validate cross-origin token requests.",
			}
		}
		return validateRedirectURIRequirement(gt, redirectURIs)
	}

	// Confidential clients (server-side web apps, M2M, backend services).
	return validateRedirectURIRequirement(gt, redirectURIs)
}

// supportedGrantTypes are the grants the authorization server serves to a
// registered client. Others are refused rather than stored: implicit cannot
// be used (a client is registered for the code response type only), the
// resource owner password grant is not offered (RFC 9700 §2.4 says it MUST
// NOT be used), and an unknown name would be kept as configuration that
// does nothing.
var supportedGrantTypes = []string{"authorization_code", "refresh_token", "client_credentials", deviceCodeGrant}

func validateGrantTypes(normalizedGrants []string) *ConfigError {
	for _, g := range normalizedGrants {
		if !containsAny(supportedGrantTypes, g) {
			return &ConfigError{
				Code:    "unsupported_grant_type",
				Field:   "grant_types",
				Message: fmt.Sprintf("%q is not a grant this provider offers", g),
				Hint:    "Use authorization_code (with refresh_token for renewals), client_credentials for machine-to-machine, or " + deviceCodeGrant + " for devices without a browser.",
			}
		}
	}
	return nil
}

// validateAccessTokenStrategy accepts "" (inherit the deployment-wide strategy)
// alongside the two real formats.
//
// This cannot be a `validate:"omitempty,oneof=jwt opaque"` struct tag. On the
// update request the field is a *string, and `omitempty` does NOT short-circuit
// for a non-nil pointer to "" — the validator dereferences it and `oneof` then
// rejects the empty value. That would make the un-pin signal (`""`, the console's
// "Default (inherit)" option) un-submittable, so a client pinned to jwt could
// never be returned to inheriting. Empirically confirmed; see
// TestUpdateClientRequest_AccessTokenStrategyValidation.
func validateAccessTokenStrategy(strategy string) *ConfigError {
	switch strategy {
	case "", "jwt", "opaque":
		return nil
	}
	return &ConfigError{
		Code:    "invalid_access_token_strategy",
		Field:   "access_token_strategy",
		Message: "access_token_strategy must be \"jwt\", \"opaque\", or empty",
		Hint:    "Use \"jwt\" for offline validation by a resource server, \"opaque\" to pin the revocable format, or omit the field (empty) to inherit the deployment-wide setting.",
	}
}

// validateRedirectURIRequirement enforces redirect_uris only for grants that
// actually redirect the user-agent back to the client (RFC 6749 §3.1.2, §4.1.1).
//
// Requiring it unconditionally forces machine-to-machine registrations
// (`client_credentials`, which never redirects) to invent a dummy URI. That dummy
// then propagates into post_logout_redirect_uris and becomes
// permanent configuration pollution for a URI that is never used.
//
// Runs last so that client-type violations (secret-on-public, wrong grant) are
// reported first — those describe *what the client is*, this one only describes
// what it is missing.
func validateRedirectURIRequirement(normalizedGrants []string, redirectURIs []string) *ConfigError {
	if !containsAny(normalizedGrants, "authorization_code") {
		return nil
	}
	if len(redirectURIs) > 0 {
		return nil
	}
	return &ConfigError{
		Code:    "redirect_grant_without_redirect_uris",
		Field:   "redirect_uris",
		Message: "Clients using authorization_code must declare at least one redirect_uri",
		Hint:    "Add the callback URL the authorization server redirects to (e.g. https://app.example.com/signin-oidc). Machine-to-machine clients using only client_credentials do not need one.",
	}
}

// validateRedirectURIForm requires each redirect URI to be absolute and to
// carry no fragment (RFC 6749 §3.1.2). Any scheme is allowed: a native app
// may be called back on a private-use scheme such as
// `com.example.app:/oauth2redirect` (RFC 8252 §7.1), which has no host.
func validateRedirectURIForm(redirectURIs []string) *ConfigError {
	for _, raw := range redirectURIs {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || (u.Host == "" && u.Path == "" && u.Opaque == "") || u.Fragment != "" || strings.Contains(raw, "#") {
			return &ConfigError{
				Code:    "invalid_redirect_uri",
				Field:   "redirect_uris",
				Message: fmt.Sprintf("%q is not a usable redirect URI", raw),
				Hint:    "Use an absolute URI without a fragment: https://app.example.com/callback for a web app, http://127.0.0.1/callback or com.example.app:/callback for a native app.",
			}
		}
	}
	return nil
}

// usesBrowserRedirect reports whether any redirect URI returns the user to a
// web page — an https URL, or http to a named host. A native app is called
// back on a loopback IP literal (RFC 8252 §7.3) or a private-use URI scheme
// (§7.1); it makes no cross-origin browser request, so it has no origin to
// allow. `localhost` counts as a web page: it is where a browser app runs
// during development, and RFC 8252 §8.3 tells native apps to use the IP
// literal instead.
func usesBrowserRedirect(redirectURIs []string) bool {
	for _, raw := range redirectURIs {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return true
		}
		switch strings.ToLower(u.Scheme) {
		case "https":
			return true
		case "http":
			if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
				return true
			}
		}
	}
	return false
}

// deviceCodeGrant is the device authorization grant (RFC 8628): a device
// without a usable browser or keyboard shows a code, the user approves it on
// another screen, and the device polls for its tokens. It needs no redirect
// URI, and a public client may use it — it is meant for exactly those.
const deviceCodeGrant = "urn:ietf:params:oauth:grant-type:device_code"

func normalizeGrantTypes(grantTypes []string) []string {
	out := make([]string, 0, len(grantTypes))
	for _, t := range grantTypes {
		out = append(out, strings.TrimSpace(strings.ToLower(t)))
	}
	return out
}

func containsAny(haystack []string, needles ...string) bool {
	for _, h := range haystack {
		for _, n := range needles {
			if h == n {
				return true
			}
		}
	}
	return false
}

// maxAudienceLength bounds one resource identifier.
const maxAudienceLength = 255

// validateAudience checks the resource identifiers a client names. Each is an
// opaque identifier the resource server compares its own with — commonly the
// API's URL — so it must be something that survives as one value of a JWT's
// aud claim: not empty, no whitespace, not repeated.
func validateAudience(audience []string) *ConfigError {
	seen := map[string]bool{}
	for _, value := range audience {
		if value == "" || len(value) > maxAudienceLength || strings.ContainsAny(value, " \t\r\n") || seen[value] {
			return &ConfigError{
				Code:    "invalid_audience",
				Field:   "audience",
				Message: "each audience must be a distinct identifier of 1 to 255 characters without whitespace",
				Hint:    "Name each API the client's access tokens are for, usually by its URL (https://api.example.com). The API checks that the token's aud claim contains its own identifier.",
			}
		}
		seen[value] = true
	}
	return nil
}

// refusedByAuthorizationServer turns the authorization server's refusal of a
// registration into the answer the caller gets. It checks the addresses a
// client names on its own side as well, when it connects to them — so it can
// refuse one this service let through, such as a host name that resolved
// differently for it.
func refusedByAuthorizationServer(refused *hydra.ClientRefusedError) *ConfigError {
	if strings.Contains(refused.Reason, "backchannel_logout_uri") {
		return &ConfigError{
			Code:    "invalid_backchannel_logout_uri",
			Field:   "backchannel_logout_uri",
			Message: "backchannel_logout_uri must not point at a local or private address",
			Hint:    "The authorization server may not call this address. Give a public https address of the application's backend.",
		}
	}
	return &ConfigError{
		Code:    "invalid_client_metadata",
		Message: "The authorization server did not accept this client: " + refused.Reason,
		Hint:    "Correct the field the message names and try again.",
	}
}

// validateApplicationURI checks an address the application publishes for
// users — where it starts a sign-in, its home page: empty, or an absolute
// https URL without a fragment. Users are sent there, so a scheme other
// than https (javascript:, a custom one) is refused.
func validateApplicationURI(field, uri string) *ConfigError {
	if uri == "" {
		return nil
	}
	u, err := url.Parse(uri)
	if err != nil || len(uri) > maxBackChannelLogoutURILength || u.Scheme != "https" || u.Host == "" || u.Fragment != "" {
		return &ConfigError{Code: "invalid_" + field, Field: field, Message: field + " must be an absolute https URL without a fragment"}
	}
	return nil
}

// signInMethods are the ways of signing in a client can offer.
var signInMethods = map[string]bool{"email": true, "magic_link": true, "google": true, "github": true, "microsoft": true, "apple": true}

// validateSignInMethods checks a client's enabled_auth_providers when the
// request names them (nil: not named). An empty list is refused: a client
// nobody can sign in to is not a configuration, and the stored empty list was
// read as the default — email and Google — the opposite of what was asked.
func validateSignInMethods(methods []string) *ConfigError {
	if methods == nil {
		return nil
	}
	if len(methods) == 0 {
		return &ConfigError{Code: "invalid_enabled_auth_providers", Field: "enabled_auth_providers", Message: "name at least one way to sign in", Hint: "One or more of email, magic_link, google, github, microsoft, apple."}
	}
	for _, m := range methods {
		if !signInMethods[m] {
			return &ConfigError{Code: "invalid_enabled_auth_providers", Field: "enabled_auth_providers", Message: fmt.Sprintf("%q is not a way to sign in", m), Hint: "One or more of email, magic_link, google, github, microsoft, apple."}
		}
	}
	return nil
}

// maxBackChannelLogoutURILength bounds the logout address.
const maxBackChannelLogoutURILength = 2048

// validateBackChannelLogoutURI checks the address a client's backend is told
// about a logout at. Empty is valid: the client is not told.
//
// The authorization server calls it, not a browser, and it does so from
// inside the deployment's network — so the address must be one the
// deployment may call on a client's say-so (outbound.Policy): a public https
// address, unless private targets are allowed. A host name is looked up now
// and refused if it leads inward; what it resolves to later is for the
// authorization server to check when it connects.
func (s *service) validateBackChannelLogoutURI(uri string) *ConfigError {
	if uri == "" {
		return nil
	}
	const hint = "Give the URL of an endpoint on the application's backend that accepts a POST with a logout_token, e.g. https://app.example.com/backchannel-logout."
	refuse := func(message string) *ConfigError {
		return &ConfigError{Code: "invalid_backchannel_logout_uri", Field: "backchannel_logout_uri", Message: message, Hint: hint}
	}
	u, err := url.Parse(uri)
	if err != nil || len(uri) > maxBackChannelLogoutURILength || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Fragment != "" {
		return refuse("backchannel_logout_uri must be an absolute https URL without a fragment")
	}
	if u.Scheme != "https" && !s.outbound.AllowPrivate {
		return refuse("backchannel_logout_uri must use https: the logout token names the user and their session")
	}
	if s.outbound.RefusesName(context.Background(), s.names, u.Hostname()) {
		return refuse("backchannel_logout_uri must not point at a local or private address")
	}
	return nil
}
