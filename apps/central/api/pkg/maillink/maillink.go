// Package maillink builds the URLs Authway puts in outgoing mail.
//
// Every one of these is opened by a human in a browser, so they must point at
// the auth UI — never at this API. Two separate incidents came from getting
// that wrong: the links were once built from the API's own base URL (so all of
// them 404'd everywhere), and the magic-link path once named an API route that
// the UI does not serve.
//
// Centralising them means the route shape is stated once and can be checked
// against the UI's router — see maillink_test.go.
package maillink

import (
	"fmt"
	"net/url"
)

// Paths mirror the client-side routes declared in
// apps/branding/auth-ui/src/App.tsx. Changing one without changing the other
// breaks mail delivery silently, so the contract test pins them together.
const (
	PathInvitationAccept = "/invitation/accept"
	PathMagicLink        = "/magic-link"
	PathVerifyEmail      = "/verify-email"
	PathResetPassword    = "/reset-password"
)

// AllPaths is what the deploy-time and contract checks iterate over.
func AllPaths() []string {
	return []string{PathInvitationAccept, PathMagicLink, PathVerifyEmail, PathResetPassword}
}

func withToken(frontendURL, path, token string) string {
	return fmt.Sprintf("%s%s?token=%s", frontendURL, path, url.QueryEscape(token))
}

// InvitationAccept is where an invited person lands to create their account.
func InvitationAccept(frontendURL, token string) string {
	return withToken(frontendURL, PathInvitationAccept, token)
}

// MagicLink is a login factor: the page POSTs the token back to the API.
func MagicLink(frontendURL, token string) string {
	return withToken(frontendURL, PathMagicLink, token)
}

// VerifyEmail confirms ownership of an address.
func VerifyEmail(frontendURL, token string) string {
	return withToken(frontendURL, PathVerifyEmail, token)
}

// ResetPassword opens the set-a-new-password form.
func ResetPassword(frontendURL, token string) string {
	return withToken(frontendURL, PathResetPassword, token)
}
