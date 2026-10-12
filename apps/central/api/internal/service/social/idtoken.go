package social

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// idTokenClaims decodes the claims of an ID token into out. The services
// receive an ID token straight from the provider's token endpoint over TLS,
// which is what authenticates it (OpenID Connect Core 3.1.3.7); the signature
// is not checked again.
func idTokenClaims(idToken string, out any) error {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid ID token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return fmt.Errorf("failed to decode ID token payload: %w", err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("failed to parse ID token claims: %w", err)
	}
	return nil
}

// claimIsTrue reads a boolean claim that providers send either as a JSON
// boolean or as the string "true".
func claimIsTrue(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return false
}
