package delegation

import (
	"strings"
	"testing"
	"time"

	"authway/apps/central/api/pkg/crypto"

	"github.com/golang-jwt/jwt/v5"
)

const testSealingKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="

func TestSigner_LoadsOneKeySealedAndSignsWithIt(t *testing.T) {
	db := setupPostgres(t)
	db.Exec(`DELETE FROM delegation_signing_keys`)
	t.Cleanup(func() { db.Exec(`DELETE FROM delegation_signing_keys`) })
	sealer, err := crypto.NewCipher(testSealingKey)
	if err != nil {
		t.Fatal(err)
	}

	first, err := LoadSigner(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadSigner(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyID() == "" || first.KeyID() != again.KeyID() {
		t.Fatalf("key ids %q and %q: every load reads the one stored key", first.KeyID(), again.KeyID())
	}
	var stored string
	db.Raw(`SELECT private_key FROM delegation_signing_keys WHERE kid = ?`, first.KeyID()).Scan(&stored)
	if !crypto.IsEncrypted(stored) {
		t.Fatal("the private key is stored in the clear")
	}

	token, err := first.Sign(jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := again.Verify(token)
	if err != nil || claims["sub"] != "user-1" {
		t.Fatalf("verify: %v %v", claims, err)
	}
	if !strings.Contains(string(first.PublicJWK()), `"kid":"`+first.KeyID()+`"`) {
		t.Fatalf("public JWK %s does not name its key", first.PublicJWK())
	}
}

func TestSigner_RefusesWhatItDidNotSign(t *testing.T) {
	sealer, _ := crypto.NewCipher(testSealingKey)
	a, _ := newSigningKey(sealer)
	b, _ := newSigningKey(sealer)
	sa, sb := mustSigner(t, sealer, a), mustSigner(t, sealer, b)

	token, _ := sa.Sign(jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Minute).Unix()})
	if _, err := sb.Verify(token); err == nil {
		t.Fatal("verified an assertion another key signed")
	}
	expired, _ := sa.Sign(jwt.MapClaims{"sub": "u", "exp": time.Now().Add(-time.Minute).Unix()})
	if _, err := sa.Verify(expired); err == nil {
		t.Fatal("verified an expired assertion")
	}
	noExpiry, _ := sa.Sign(jwt.MapClaims{"sub": "u"})
	if _, err := sa.Verify(noExpiry); err == nil {
		t.Fatal("verified an assertion without an expiry")
	}
}

func mustSigner(t *testing.T, sealer crypto.Cipher, k *SigningKey) *Signer {
	t.Helper()
	opened, err := sealer.Decrypt(k.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	s, err := signerFrom(k.KID, opened)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
