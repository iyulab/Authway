package handler

import (
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// defaultSessionTimeout is a tenant's session_timeout when it has none.
	defaultSessionTimeout = time.Hour

	// keepSignedInFor is how long a session lasts when the user asked to stay
	// signed in on the browser. It is also the longest the authorization
	// server keeps a session unless its own limit is raised.
	keepSignedInFor = 30 * 24 * time.Hour

	// autoConsentRememberedFor is how long a consent given on the user's
	// behalf — the client is registered to skip the question — is remembered.
	// It has to outlast the session: the authorization server answers a
	// silent request (prompt=none) with consent_required unless it finds a
	// remembered consent, whatever the client's registration says. It is
	// bounded so that turning skip_consent off takes effect.
	autoConsentRememberedFor = keepSignedInFor
)

// SessionPolicy says how long the session a sign-in opens lasts.
//
// The authorization server has no session that ends with the browser: a
// sign-in is either not remembered at all or remembered for a set time. So
// every sign-in is remembered, for the tenant's session_timeout — or, when
// the user asked to stay signed in, for keepSignedInFor, which is also the
// most a session_timeout can ask for.
type SessionPolicy struct {
	// Timeout returns a tenant's session_timeout; zero or less means it has
	// none. A nil Timeout gives every tenant the default.
	Timeout func(tenantID uuid.UUID) time.Duration
}

func (p SessionPolicy) lifetime(tenantID uuid.UUID, keepSignedIn bool) time.Duration {
	timeout := defaultSessionTimeout
	if p.Timeout != nil {
		if t := p.Timeout(tenantID); t > 0 {
			timeout = t
		}
	}
	if keepSignedIn || timeout > keepSignedInFor {
		return keepSignedInFor
	}
	return timeout
}

// asksForNoPrompt reports whether the authorization request at requestURL
// carries prompt=none: the application wants an answer without the user being
// shown anything.
func asksForNoPrompt(requestURL string) bool {
	u, err := url.Parse(requestURL)
	if err != nil {
		return false
	}
	for _, p := range strings.Fields(u.Query().Get("prompt")) {
		if p == "none" {
			return true
		}
	}
	return false
}
