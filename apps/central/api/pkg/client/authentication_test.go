package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func rsaJWK(t *testing.T, extra map[string]any) map[string]any {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := map[string]any{"kty": "RSA", "kid": "r", "n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes())}
	for m, v := range extra {
		jwk[m] = v
	}
	return jwk
}

func ecJWK(t *testing.T, curve elliptic.Curve, crv string, extra map[string]any) map[string]any {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := k.PublicKey.ECDH()
	if err != nil {
		t.Fatal(err)
	}
	raw := pub.Bytes()[1:] // uncompressed point: 0x04 || X || Y
	jwk := map[string]any{"kty": "EC", "kid": "e", "crv": crv, "x": b64(raw[:len(raw)/2]), "y": b64(raw[len(raw)/2:])}
	for m, v := range extra {
		jwk[m] = v
	}
	return jwk
}

func keySet(t *testing.T, keys ...map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"keys": keys})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The algorithm a key-authenticating client signs with is the one the
// authorization server checks it with, so it is taken from the keys or named,
// and must fit them.
func TestValidateAuthentication_SigningAlgorithm(t *testing.T) {
	s := &service{}
	p256 := ecJWK(t, elliptic.P256(), "P-256", nil)
	p384 := ecJWK(t, elliptic.P384(), "P-384", nil)
	for name, c := range map[string]struct {
		keys    json.RawMessage
		alg     string
		want    string
		refusal string
	}{
		"an RSA key":                       {keys: keySet(t, rsaJWK(t, nil)), want: "RS256"},
		"an RSA key for PSS":               {keys: keySet(t, rsaJWK(t, nil)), alg: "PS256", want: "PS256"},
		"an RSA key naming its alg":        {keys: keySet(t, rsaJWK(t, map[string]any{"alg": "RS512"})), want: "RS512"},
		"a P-256 key":                      {keys: keySet(t, p256), want: "ES256"},
		"a P-384 key":                      {keys: keySet(t, p384), want: "ES384"},
		"two keys of one kind":             {keys: keySet(t, p256, ecJWK(t, elliptic.P256(), "P-256", nil)), want: "ES256"},
		"keys of two kinds":                {keys: keySet(t, p256, rsaJWK(t, nil)), refusal: "token_endpoint_auth_signing_alg"},
		"an alg the keys do not sign with": {keys: keySet(t, p256), alg: "RS256", refusal: "token_endpoint_auth_signing_alg"},
		"an alg not checked":               {keys: keySet(t, p256), alg: "HS256", refusal: "token_endpoint_auth_signing_alg"},
		"a key naming a wrong alg":         {keys: keySet(t, ecJWK(t, elliptic.P256(), "P-256", map[string]any{"alg": "ES384"})), refusal: "jwks"},
		"a key for encryption":             {keys: keySet(t, rsaJWK(t, map[string]any{"use": "enc"})), refusal: "jwks"},
		"an Ed25519 key":                   {keys: keySet(t, map[string]any{"kty": "OKP", "crv": "Ed25519", "x": "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"}), refusal: "jwks"},
	} {
		got, refused := s.validateAuthentication(false, AuthMethodPrivateKeyJWT, c.keys, "", c.alg)
		switch {
		case c.refusal != "" && (refused == nil || refused.Field != c.refusal):
			t.Errorf("%s: want a refusal of %s, got %+v %+v", name, c.refusal, got, refused)
		case c.refusal == "" && (refused != nil || got.Alg != c.want):
			t.Errorf("%s: want %s, got %+v %+v", name, c.want, got, refused)
		}
	}

	if _, refused := s.validateAuthentication(false, "", nil, "", "RS256"); refused == nil || refused.Field != "token_endpoint_auth_signing_alg" {
		t.Errorf("a secret client naming an alg: %+v", refused)
	}
}

// The authorization server checks an assertion only with a key marked for
// signatures; a key registered without use is sent marked.
func TestRegisteredKeys_MarksKeysForSignatures(t *testing.T) {
	c := &Client{TokenEndpointAuthMethod: AuthMethodPrivateKeyJWT, JWKS: string(keySet(t, rsaJWK(t, nil)))}
	if got := string(c.registeredKeys()); !strings.Contains(got, `"use":"sig"`) {
		t.Fatalf("keys sent: %s", got)
	}
	if c.AssertionAlg() != "RS256" {
		t.Fatalf("a client registered before the algorithm was kept: %q", c.AssertionAlg())
	}
}
