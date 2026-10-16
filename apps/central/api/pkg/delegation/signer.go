package delegation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"authway/apps/central/api/pkg/crypto"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// SigningKey is a stored key Authway signs delegation assertions with. The
// private key is PKCS#8, base64, sealed at rest.
type SigningKey struct {
	KID        string    `gorm:"column:kid;primaryKey"`
	PrivateKey string    `gorm:"column:private_key;not null"`
	CreatedAt  time.Time `gorm:"not null"`
}

// TableName names SigningKey's table.
func (SigningKey) TableName() string { return "delegation_signing_keys" }

// Signer signs the assertions Authway hands the authorization server when a
// client exchanges a delegation (RFC 7523). They never leave the deployment.
type Signer struct {
	kid string
	key *ecdsa.PrivateKey
}

// LoadSigner returns the signer of the newest stored key, creating the first
// one when there is none. Replicas starting together agree on one key: each
// inserts its own only if no key exists, and all of them then read the
// newest. A key sealed with a key the cipher has since replaced is sealed
// again with the current one.
func LoadSigner(db *gorm.DB, sealer crypto.Cipher) (*Signer, error) {
	var stored []SigningKey
	if err := db.Order("created_at DESC, kid DESC").Limit(1).Find(&stored).Error; err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		fresh, err := newSigningKey(sealer)
		if err != nil {
			return nil, err
		}
		err = db.Exec(`INSERT INTO delegation_signing_keys (kid, private_key, created_at)
			SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM delegation_signing_keys)`,
			fresh.KID, fresh.PrivateKey, fresh.CreatedAt).Error
		if err != nil {
			return nil, err
		}
		if err := db.Order("created_at DESC, kid DESC").Limit(1).Find(&stored).Error; err != nil {
			return nil, err
		}
		if len(stored) == 0 {
			return nil, errors.New("delegation: no signing key after creating one")
		}
	}
	k := stored[0]
	resealed, changed, err := sealer.Reseal(k.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("delegation: the signing key cannot be read: %w", err)
	}
	if changed {
		if err := db.Model(&SigningKey{}).Where("kid = ?", k.KID).Update("private_key", resealed).Error; err != nil {
			return nil, err
		}
	}
	opened, err := sealer.Decrypt(k.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("delegation: the signing key cannot be read: %w", err)
	}
	return signerFrom(k.KID, opened)
}

// signerFrom is the signer of an opened key (PKCS#8, base64).
func signerFrom(kid, opened string) (*Signer, error) {
	der, err := base64.StdEncoding.DecodeString(opened)
	if err != nil {
		return nil, fmt.Errorf("delegation: the signing key is not base64: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("delegation: the signing key cannot be parsed: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("delegation: the signing key is not a P-256 key")
	}
	return &Signer{kid: kid, key: key}, nil
}

func newSigningKey(sealer crypto.Cipher) (*SigningKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	sealed, err := sealer.Encrypt(base64.StdEncoding.EncodeToString(der))
	if err != nil {
		return nil, err
	}
	return &SigningKey{KID: thumbprint(&key.PublicKey), PrivateKey: sealed, CreatedAt: time.Now().UTC()}, nil
}

// publicJWK is k's public key as a JWK (RFC 7517), without a key id.
func publicJWK(k *ecdsa.PublicKey) map[string]string {
	x, y := make([]byte, 32), make([]byte, 32) // P-256 coordinates, fixed width
	k.X.FillBytes(x)
	k.Y.FillBytes(y)
	return map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(x),
		"y":   base64.RawURLEncoding.EncodeToString(y),
	}
}

// thumbprint is k's JWK thumbprint (RFC 7638), the key's id.
func thumbprint(k *ecdsa.PublicKey) string {
	j := publicJWK(k)
	// RFC 7638: the required members, in lexicographic order, no whitespace.
	canonical := fmt.Sprintf(`{"crv":%q,"kty":%q,"x":%q,"y":%q}`, j["crv"], j["kty"], j["x"], j["y"])
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// KeyID is the id of the key in use.
func (s *Signer) KeyID() string { return s.kid }

// PublicJWK is the key in use, as the authorization server is told to trust
// it.
func (s *Signer) PublicJWK() json.RawMessage {
	j := publicJWK(&s.key.PublicKey)
	j["kid"] = s.kid
	j["alg"] = "ES256"
	j["use"] = "sig"
	out, _ := json.Marshal(j)
	return out
}

// Sign signs claims as an ES256 JWT with the key in use.
func (s *Signer) Sign(claims jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	t.Header["kid"] = s.kid
	return t.SignedString(s.key)
}

// Verify checks that token is an assertion this signer signed and returns its
// claims. Expiry is checked; the audience is the caller's to check.
func (s *Signer) Verify(token string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if kid, _ := t.Header["kid"].(string); kid != s.kid {
			return nil, errors.New("not signed with the key in use")
		}
		return &s.key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"ES256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}
