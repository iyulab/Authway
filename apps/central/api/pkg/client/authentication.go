package client

import (
	"authway/apps/central/api/pkg/apierror"
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"slices"
)

// How a client proves who it is at the token endpoint.
const (
	// AuthMethodNone: a public client proves nothing, and relies on PKCE.
	AuthMethodNone = "none"
	// AuthMethodSecret: the client sends the secret it was given.
	AuthMethodSecret = "client_secret_post"
	// AuthMethodPrivateKeyJWT: the client signs an assertion with a key whose
	// public half is registered (RFC 7523). Nothing secret is shared with the
	// provider.
	AuthMethodPrivateKeyJWT = "private_key_jwt"
)

// AuthMethod is how the client authenticates at the token endpoint. Only a
// method other than the one its kind implies is stored.
func (c *Client) AuthMethod() string {
	switch {
	case c.Public:
		return AuthMethodNone
	case c.TokenEndpointAuthMethod != "":
		return c.TokenEndpointAuthMethod
	default:
		return AuthMethodSecret
	}
}

// AuthenticatesWithKey reports whether the client holds a key rather than a
// secret.
func (c *Client) AuthenticatesWithKey() bool {
	return c.AuthMethod() == AuthMethodPrivateKeyJWT
}

// AssertionAlg is the algorithm the client signs its assertion with (RFC 7591
// token_endpoint_auth_signing_alg) — for a client that authenticates with a
// key, "" otherwise. One registered before it was kept is RS256: what the
// authorization server took for it.
func (c *Client) AssertionAlg() string {
	switch {
	case !c.AuthenticatesWithKey():
		return ""
	case c.TokenEndpointAuthSigningAlg != "":
		return c.TokenEndpointAuthSigningAlg
	default:
		return defaultAssertionAlg
	}
}

// registeredKeys is the client's key set as the authorization server takes
// it, or nil: every key marked for signatures (use "sig"), which the server
// requires of a key it checks an assertion with.
func (c *Client) registeredKeys() json.RawMessage {
	if c.JWKS == "" {
		return nil
	}
	var set struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal([]byte(c.JWKS), &set); err != nil {
		return json.RawMessage(c.JWKS)
	}
	for _, key := range set.Keys {
		if _, present := key["use"]; !present {
			key["use"] = json.RawMessage(`"sig"`)
		}
	}
	out, err := json.Marshal(set)
	if err != nil {
		return json.RawMessage(c.JWKS)
	}
	return out
}

const (
	maxJWKSBytes = 16 * 1024
	maxJWKSKeys  = 10
	maxJWKSURI   = 2048
)

// The members of a JSON Web Key that only its private half has (RFC 7518 §6),
// and those its public half must have, by key type.
var (
	privateKeyMembers = []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"}
	publicKeyMembers  = map[string][]string{
		"RSA": {"n", "e"},
		"EC":  {"crv", "x", "y"},
	}
)

// defaultAssertionAlg is the algorithm of a client whose keys are fetched
// from an address and that names none, as the authorization server assumes.
const defaultAssertionAlg = "RS256"

// assertionAlgs are the algorithms the authorization server checks a client
// assertion with (RFC 7518 §3.1), and the key each one takes. Ed25519
// (EdDSA) is not among them: the server does not register such a client.
var assertionAlgs = map[string]struct{ kty, crv string }{
	"RS256": {"RSA", ""}, "RS384": {"RSA", ""}, "RS512": {"RSA", ""},
	"PS256": {"RSA", ""}, "PS384": {"RSA", ""}, "PS512": {"RSA", ""},
	"ES256": {"EC", "P-256"}, "ES384": {"EC", "P-384"}, "ES512": {"EC", "P-521"},
}

// keyAlgs are the algorithms a key can sign with, its own (alg) first when it
// names one.
func keyAlgs(kty, crv, own string) []string {
	if own != "" {
		return []string{own}
	}
	switch {
	case kty == "RSA":
		return []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512"}
	case kty == "EC" && crv == "P-256":
		return []string{"ES256"}
	case kty == "EC" && crv == "P-384":
		return []string{"ES384"}
	case kty == "EC" && crv == "P-521":
		return []string{"ES512"}
	}
	return nil
}

const jwksHint = "Register the public half of the key the client signs its assertion with: {\"keys\": [{\"kty\": \"RSA\", \"kid\": \"…\", \"n\": \"…\", \"e\": \"AQAB\"}]}. Give either jwks or jwks_uri."

const signingAlgHint = "Name one of RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512 — or leave it out: it is taken from the keys (RS256 for an RSA key, ES256 for a P-256 key)."

// Authentication is how a client is to authenticate at the token endpoint,
// as it will be stored: for a client that does so with a key, the key set
// (or none, when it is fetched from jwks_uri) and the algorithm.
type Authentication struct {
	Keys string
	Alg  string
}

// validateAuthentication checks how a client is to authenticate at the token
// endpoint against what kind of client it is. method and alg are "" when the
// caller named none; alg is then taken from the keys.
func (s *service) validateAuthentication(public bool, method string, jwks json.RawMessage, jwksURI, alg string) (Authentication, *apierror.Refusal) {
	refuseMethod := func(message, hint string) *apierror.Refusal {
		return &apierror.Refusal{Code: "invalid_token_endpoint_auth_method", Field: "token_endpoint_auth_method", Message: message, Hint: hint}
	}
	refuseKeys := func(message string) *apierror.Refusal {
		return &apierror.Refusal{Code: "invalid_jwks", Field: "jwks", Message: message, Hint: jwksHint}
	}
	refuseAlg := func(message string) *apierror.Refusal {
		return &apierror.Refusal{Code: "invalid_token_endpoint_auth_signing_alg", Field: "token_endpoint_auth_signing_alg", Message: message, Hint: signingAlgHint}
	}
	hasKeys := len(bytes.TrimSpace(jwks)) > 0 && !bytes.Equal(bytes.TrimSpace(jwks), []byte("null"))

	switch method {
	case "", AuthMethodSecret, AuthMethodNone:
		if method == AuthMethodNone && !public {
			return Authentication{}, refuseMethod("token_endpoint_auth_method none is for a public client", "Set public: true, or leave the method out: a confidential client is given a secret.")
		}
		if method == AuthMethodSecret && public {
			return Authentication{}, refuseMethod("a public client has no secret to authenticate with", "Leave the method out, or register a confidential client.")
		}
		if hasKeys || jwksURI != "" {
			return Authentication{}, refuseKeys("a key is registered only for a client that authenticates with one")
		}
		if alg != "" {
			return Authentication{}, refuseAlg("a signing algorithm is named only for a client that authenticates with a key")
		}
		return Authentication{}, nil
	case AuthMethodPrivateKeyJWT:
		if public {
			return Authentication{}, refuseMethod("a public client cannot authenticate with a key", "A client that can keep a key is a confidential client: set public: false.")
		}
	default:
		return Authentication{}, refuseMethod("token_endpoint_auth_method must be client_secret_post, private_key_jwt or none", "Leave it out for the default: a secret for a confidential client, none for a public one.")
	}
	if _, known := assertionAlgs[alg]; alg != "" && !known {
		return Authentication{}, refuseAlg("token_endpoint_auth_signing_alg is not one the provider checks assertions with")
	}

	// private_key_jwt: exactly one of a key set and an address to fetch it from.
	if hasKeys == (jwksURI != "") {
		return Authentication{}, refuseKeys("a client that authenticates with a key registers either jwks or jwks_uri")
	}
	if jwksURI != "" {
		if alg == "" {
			alg = defaultAssertionAlg
		}
		return Authentication{Alg: alg}, s.validateJWKSURI(jwksURI)
	}

	if len(jwks) > maxJWKSBytes {
		return Authentication{}, refuseKeys("jwks is too large")
	}
	var set struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(jwks, &set); err != nil || len(set.Keys) == 0 || len(set.Keys) > maxJWKSKeys {
		return Authentication{}, refuseKeys("jwks must be a JSON Web Key Set with 1 to 10 keys")
	}
	// What every key can sign with; the client's algorithm is one all share.
	var common []string
	for i, key := range set.Keys {
		var kty, crv, use, own string
		if err := json.Unmarshal(key["kty"], &kty); err != nil {
			return Authentication{}, refuseKeys("every key in jwks names its type (kty)")
		}
		if kty == "OKP" {
			return Authentication{}, refuseKeys("Ed25519 keys are not taken: register an RSA or EC key")
		}
		required, known := publicKeyMembers[kty]
		if !known {
			return Authentication{}, refuseKeys("jwks holds keys of type RSA or EC")
		}
		for _, member := range privateKeyMembers {
			if _, present := key[member]; present {
				return Authentication{}, refuseKeys("jwks must hold public keys only: a private key was given, and is now to be considered disclosed")
			}
		}
		for _, member := range required {
			if _, present := key[member]; !present {
				return Authentication{}, refuseKeys("a key in jwks lacks a member its type requires")
			}
		}
		_ = json.Unmarshal(key["crv"], &crv)
		if raw, present := key["use"]; present && (json.Unmarshal(raw, &use) != nil || use != "sig") {
			return Authentication{}, refuseKeys(`a key in jwks is for signatures: its use, when given, is "sig"`)
		}
		if raw, present := key["alg"]; present {
			if json.Unmarshal(raw, &own) != nil || assertionAlgs[own].kty != kty || assertionAlgs[own].crv != crv {
				return Authentication{}, refuseKeys("a key in jwks names an algorithm (alg) its type does not sign with, or one the provider does not check assertions with")
			}
		}
		can := keyAlgs(kty, crv, own)
		if len(can) == 0 {
			return Authentication{}, refuseKeys("an EC key in jwks is on P-256, P-384 or P-521")
		}
		if i == 0 {
			common = can
		} else {
			common = slices.DeleteFunc(common, func(a string) bool { return !slices.Contains(can, a) })
		}
	}
	switch {
	case alg != "" && !slices.Contains(common, alg):
		return Authentication{}, refuseAlg("the keys in jwks do not sign with token_endpoint_auth_signing_alg")
	case alg == "" && len(common) == 0:
		return Authentication{}, refuseAlg("the keys in jwks sign with different algorithms, and a client checks assertions with one: register keys of one kind")
	case alg == "":
		alg = common[0]
	}
	// Stored as given, compacted: the authorization server receives the same
	// document the caller registered, each key marked for signatures
	// (registeredKeys).
	var compact bytes.Buffer
	if err := json.Compact(&compact, jwks); err != nil {
		return Authentication{}, refuseKeys("jwks must be a JSON Web Key Set with 1 to 10 keys")
	}
	return Authentication{Keys: compact.String(), Alg: alg}, nil
}

// validateJWKSURI checks the address the authorization server fetches a
// client's keys from. The server calls it from inside its own network, so it
// is held to what a logout address is held to: a public https address.
func (s *service) validateJWKSURI(uri string) *apierror.Refusal {
	refuse := func(message string) *apierror.Refusal {
		return &apierror.Refusal{
			Code: "invalid_jwks_uri", Field: "jwks_uri", Message: message,
			Hint: "Give the public https URL the client's key set is served at, e.g. https://app.example.com/.well-known/jwks.json.",
		}
	}
	u, err := url.Parse(uri)
	if err != nil || len(uri) > maxJWKSURI || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Fragment != "" {
		return refuse("jwks_uri must be an absolute https URL without a fragment")
	}
	if u.Scheme != "https" && !s.outbound.AllowPrivate {
		return refuse("jwks_uri must use https: whoever answers there decides who the client is")
	}
	if s.outbound.RefusesName(context.Background(), s.names, u.Hostname()) {
		return refuse("jwks_uri must not point at a local or private address")
	}
	return nil
}
