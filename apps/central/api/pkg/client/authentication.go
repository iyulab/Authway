package client

import (
	"authway/apps/central/api/pkg/apierror"
	"bytes"
	"context"
	"encoding/json"
	"net/url"
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

// registeredKeys is the client's key set as a JSON document, or nil.
func (c *Client) registeredKeys() json.RawMessage {
	if c.JWKS == "" {
		return nil
	}
	return json.RawMessage(c.JWKS)
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
		"OKP": {"crv", "x"},
	}
)

const jwksHint = "Register the public half of the key the client signs its assertion with: {\"keys\": [{\"kty\": \"RSA\", \"kid\": \"…\", \"n\": \"…\", \"e\": \"AQAB\"}]}. Give either jwks or jwks_uri."

// validateAuthentication checks how a client is to authenticate at the token
// endpoint against what kind of client it is, and returns the key set as it
// will be stored. method is "" when the caller named none.
func (s *service) validateAuthentication(public bool, method string, jwks json.RawMessage, jwksURI string) (string, *apierror.Refusal) {
	refuseMethod := func(message, hint string) *apierror.Refusal {
		return &apierror.Refusal{Code: "invalid_token_endpoint_auth_method", Field: "token_endpoint_auth_method", Message: message, Hint: hint}
	}
	refuseKeys := func(message string) *apierror.Refusal {
		return &apierror.Refusal{Code: "invalid_jwks", Field: "jwks", Message: message, Hint: jwksHint}
	}
	hasKeys := len(bytes.TrimSpace(jwks)) > 0 && !bytes.Equal(bytes.TrimSpace(jwks), []byte("null"))

	switch method {
	case "", AuthMethodSecret, AuthMethodNone:
		if method == AuthMethodNone && !public {
			return "", refuseMethod("token_endpoint_auth_method none is for a public client", "Set public: true, or leave the method out: a confidential client is given a secret.")
		}
		if method == AuthMethodSecret && public {
			return "", refuseMethod("a public client has no secret to authenticate with", "Leave the method out, or register a confidential client.")
		}
		if hasKeys || jwksURI != "" {
			return "", refuseKeys("a key is registered only for a client that authenticates with one")
		}
		return "", nil
	case AuthMethodPrivateKeyJWT:
		if public {
			return "", refuseMethod("a public client cannot authenticate with a key", "A client that can keep a key is a confidential client: set public: false.")
		}
	default:
		return "", refuseMethod("token_endpoint_auth_method must be client_secret_post, private_key_jwt or none", "Leave it out for the default: a secret for a confidential client, none for a public one.")
	}

	// private_key_jwt: exactly one of a key set and an address to fetch it from.
	if hasKeys == (jwksURI != "") {
		return "", refuseKeys("a client that authenticates with a key registers either jwks or jwks_uri")
	}
	if jwksURI != "" {
		return "", s.validateJWKSURI(jwksURI)
	}

	if len(jwks) > maxJWKSBytes {
		return "", refuseKeys("jwks is too large")
	}
	var set struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(jwks, &set); err != nil || len(set.Keys) == 0 || len(set.Keys) > maxJWKSKeys {
		return "", refuseKeys("jwks must be a JSON Web Key Set with 1 to 10 keys")
	}
	for _, key := range set.Keys {
		var kty string
		if err := json.Unmarshal(key["kty"], &kty); err != nil {
			return "", refuseKeys("every key in jwks names its type (kty)")
		}
		required, known := publicKeyMembers[kty]
		if !known {
			return "", refuseKeys("jwks holds keys of type RSA, EC or OKP")
		}
		for _, member := range privateKeyMembers {
			if _, present := key[member]; present {
				return "", refuseKeys("jwks must hold public keys only: a private key was given, and is now to be considered disclosed")
			}
		}
		for _, member := range required {
			if _, present := key[member]; !present {
				return "", refuseKeys("a key in jwks lacks a member its type requires")
			}
		}
	}
	// Stored as given, compacted: the authorization server receives the same
	// document the caller registered.
	var compact bytes.Buffer
	if err := json.Compact(&compact, jwks); err != nil {
		return "", refuseKeys("jwks must be a JSON Web Key Set with 1 to 10 keys")
	}
	return compact.String(), nil
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
