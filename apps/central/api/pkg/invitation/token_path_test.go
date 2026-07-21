package invitation

import (
	"net/url"
	"strings"
	"testing"
)

// Invitation tokens travel through a URL path segment
// (GET /api/v1/invitations/token/:token). Fiber does not percent-decode path
// params, so a token containing a character a client must escape never matched
// the stored value, and every invitation opened from an email reported
// "invitation not found or expired". The handler decodes explicitly; these
// tests pin the two halves of that contract.
//
// The deploy-time mail-link check cannot cover this: it proves the page is
// reachable, not that the token survives the round trip.

func TestGeneratedTokensSurviveAPathSegmentRoundTrip(t *testing.T) {
	for i := 0; i < 50; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}

		// What a correct client sends, and what the handler must recover.
		escaped := url.PathEscape(token)
		decoded, err := url.PathUnescape(escaped)
		if err != nil {
			t.Fatalf("PathUnescape(%q): %v", escaped, err)
		}
		if decoded != token {
			t.Fatalf("token did not survive the round trip: got %q, want %q", decoded, token)
		}
	}
}

func TestPathUnescapeIsIdempotentForUnescapedTokens(t *testing.T) {
	// Older invitations were emailed with the raw token, so the handler still
	// receives unescaped values. Decoding must not corrupt them.
	for _, raw := range []string{
		"Thb-jaZjfMHEfYxOcg52pvTlhmq6HBJ8MEUjOO5y5UY=", // padded, as issued before
		"Thb-jaZjfMHEfYxOcg52pvTlhmq6HBJ8MEUjOO5y5UY",  // unpadded
		"abc_-123",
	} {
		got, err := url.PathUnescape(raw)
		if err != nil {
			t.Errorf("PathUnescape(%q) errored: %v", raw, err)
			continue
		}
		if got != raw {
			t.Errorf("PathUnescape(%q) = %q, want it unchanged", raw, got)
		}
	}
}

func TestEscapedPaddingDecodesBackToPadding(t *testing.T) {
	// The exact failure seen in staging: "...UY%3D" reached the handler as a
	// literal and never matched "...UY=" in the database.
	const stored = "Thb-jaZjfMHEfYxOcg52pvTlhmq6HBJ8MEUjOO5y5UY="
	const asSentByTheBrowser = "Thb-jaZjfMHEfYxOcg52pvTlhmq6HBJ8MEUjOO5y5UY%3D"

	got, err := url.PathUnescape(asSentByTheBrowser)
	if err != nil {
		t.Fatalf("PathUnescape: %v", err)
	}
	if got != stored {
		t.Errorf("got %q, want %q", got, stored)
	}
}

func TestGeneratedTokensCarryNoPadding(t *testing.T) {
	// Hardening on top of the decode: tokens now use unpadded base64url, so
	// nothing in them needs escaping in the first place. Belt and braces —
	// the decode above is what makes already-issued tokens work.
	for i := 0; i < 50; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}
		if strings.ContainsAny(token, "=+/") {
			t.Errorf("token %q contains a character that needs URL escaping", token)
		}
		if url.PathEscape(token) != token {
			t.Errorf("token %q is not already path-safe", token)
		}
	}
}
