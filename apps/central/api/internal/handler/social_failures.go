package handler

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/service/social"
)

// Messages shown to a user whose social sign-in could not finish. They land
// in a browser — either back in the application as an OAuth error or on the
// login UI's error screen — so they say what to do, never how the server is
// configured.
const (
	msgInvalidSignIn   = "This sign-in link is not valid or has expired. Start again from the application."
	msgSignInFailed    = "Signing in with this provider did not complete. Try again or use another sign-in method."
	msgProviderRefused = "The sign-in was cancelled or refused at the provider."
)

// endSignIn finishes a failed provider callback in the browser. Once the
// Hydra login request is known it is rejected, which returns the user to the
// application with a standard OAuth error; before that — or when Hydra
// cannot take the rejection — the login UI's error screen explains it.
func (s *SocialHandler) endSignIn(c *fiber.Ctx, loginChallenge, code, description string) error {
	if loginChallenge != "" {
		resp, err := s.hydraClient.RejectLoginRequest(loginChallenge, code, description)
		if err == nil && resp.RedirectTo != "" {
			return c.Redirect(resp.RedirectTo, fiber.StatusFound)
		}
		s.logger.Error("Failed to reject the login request after a failed social sign-in", zap.Error(err))
	}
	q := url.Values{"error": {code}, "error_description": {description}}
	return c.Redirect(strings.TrimRight(s.frontendURL, "/")+"/error?"+q.Encode(), fiber.StatusFound)
}

// recoverChallenge returns the login request a provider callback belongs to
// when its state is genuine (matches the cookie and is still stored), or ""
// — used when the provider reports an error, so the user can still be sent
// back to the application.
func (s *SocialHandler) recoverChallenge(c *fiber.Ctx, state string) string {
	if state == "" || string([]byte(c.Cookies("oauth_state"))) != state {
		return ""
	}
	stored, found, err := s.stateStore.Consume(c.Context(), state)
	if err != nil || !found {
		return ""
	}
	return stored.LoginChallenge
}

// socialFailure maps a provider-callback error to the OAuth error the
// application receives. An uninvited address is a refusal, not a fault.
func socialFailure(err error) (string, string) {
	if errors.Is(err, social.ErrNotInvited) {
		return "access_denied", social.ErrNotInvited.Error()
	}
	return "server_error", msgSignInFailed
}
