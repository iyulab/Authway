package client

import (
	"fmt"
	"net"
	"net/url"
	"strings"
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

		// Public clients use PKCE — `client_credentials` and `password` grants
		// require a confidential client.
		for _, t := range gt {
			if t == "client_credentials" {
				return &ConfigError{
					Code:    "public_client_with_client_credentials",
					Field:   "grant_types",
					Message: "Public clients cannot use the client_credentials grant",
					Hint:    "Machine-to-machine (M2M) clients must be confidential — set public=false.",
				}
			}
			if t == "password" {
				return &ConfigError{
					Code:    "public_client_with_password_grant",
					Field:   "grant_types",
					Message: "Public clients cannot use the password grant",
					Hint:    "The password grant requires a confidential client; consider using authorization_code + PKCE instead.",
				}
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
	// At least one credential-bearing grant must be present, otherwise the
	// secret is meaningless and the client is mis-typed.
	if len(gt) > 0 && !containsAny(gt, "authorization_code", "client_credentials", "refresh_token", "password") {
		return &ConfigError{
			Code:    "confidential_client_unsupported_grants",
			Field:   "grant_types",
			Message: "Confidential clients must use a credential-bearing grant",
			Hint:    "Use one of: authorization_code, client_credentials, refresh_token, password.",
		}
	}

	return validateRedirectURIRequirement(gt, redirectURIs)
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
	if !containsAny(normalizedGrants, "authorization_code", "implicit") {
		return nil
	}
	if len(redirectURIs) > 0 {
		return nil
	}
	return &ConfigError{
		Code:    "redirect_grant_without_redirect_uris",
		Field:   "redirect_uris",
		Message: "Clients using authorization_code or implicit must declare at least one redirect_uri",
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
