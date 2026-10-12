package claims

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// reserved are the claim names a stored claim can never supply: those JWT and
// OpenID Connect define (the authorization server sets them), and those
// Authway itself asserts about the user. A relying party decides who the user
// is and which tenant they belong to from these, so a value the user wrote
// must never stand in for one.
var reserved = map[string]struct{}{
	// RFC 7519, RFC 9068, OpenID Connect Core, RFC 8693, RFC 7800.
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "nbf": {}, "iat": {}, "jti": {},
	"azp": {}, "nonce": {}, "at_hash": {}, "c_hash": {}, "s_hash": {},
	"acr": {}, "amr": {}, "auth_time": {}, "sid": {},
	"scp": {}, "scope": {}, "client_id": {}, "cnf": {}, "act": {}, "may_act": {},
	// Where the authorization server nests session claims.
	"ext": {},
	// What Authway asserts about the user.
	"tenant_id": {}, "email": {}, "email_verified": {}, "name": {},
}

// IsReserved reports whether key names a claim only the issuer may set. Names
// starting with an underscore are reserved too, for Authway's own bookkeeping.
func IsReserved(key string) bool {
	if strings.HasPrefix(key, "_") {
		return true
	}
	_, ok := reserved[key]
	return ok
}

// ReservedClaimError refuses a write that names reserved claims.
type ReservedClaimError struct {
	Keys []string
}

func (e *ReservedClaimError) Error() string {
	return fmt.Sprintf("these claims are set by the issuer and cannot be written: %s", strings.Join(e.Keys, ", "))
}

// checkWritable refuses claims that name reserved keys.
func checkWritable(claims ClaimMap) error {
	var keys []string
	for key := range claims {
		if IsReserved(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return &ReservedClaimError{Keys: keys}
}

// withoutReserved copies claims without the reserved keys — a stored claim
// written before writes were checked, or one that reached Redis some other
// way, stays out of the token.
func withoutReserved(claims ClaimMap) ClaimMap {
	out := make(ClaimMap, len(claims))
	for key, value := range claims {
		if !IsReserved(key) {
			out[key] = value
		}
	}
	return out
}

// Issued is what Authway asserts about the user a token is issued to.
type Issued struct {
	TenantID      uuid.UUID
	Email         string
	Name          *string
	EmailVerified bool
}

// AccessToken returns the claims for an access token: the stored claims, with
// the issuer's on top.
func (i Issued) AccessToken(stored ClaimMap) map[string]any {
	out := make(map[string]any, len(stored)+3)
	for key, value := range withoutReserved(stored) {
		out[key] = value
	}
	out["tenant_id"] = i.TenantID.String()
	out["email"] = i.Email
	out["name"] = i.Name
	return out
}

// IDToken returns the claims for an ID token: those of the access token, plus
// whether the email address is verified.
func (i Issued) IDToken(stored ClaimMap) map[string]any {
	out := i.AccessToken(stored)
	out["email_verified"] = i.EmailVerified
	return out
}
