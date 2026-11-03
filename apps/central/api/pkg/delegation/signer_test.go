package delegation

import (
	"strings"
	"sync"
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

// Replicas starting together on an empty deployment agree on one key: the
// database, not luck, decides who creates it. Each would otherwise find the
// table empty, insert its own, and sign with a different one.
func TestSigner_ReplicasStartingTogetherCreateOneKey(t *testing.T) {
	db := setupPostgres(t)
	sealer, err := crypto.NewCipher(testSealingKey)
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 15; round++ {
		db.Exec(`DELETE FROM delegation_signing_keys`)
		const replicas = 12
		var wg sync.WaitGroup
		kids := make([]string, replicas)
		failures := make([]error, replicas)
		start := make(chan struct{})
		for i := 0; i < replicas; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				signer, err := LoadSigner(db, sealer)
				if err != nil {
					failures[i] = err
					return
				}
				kids[i] = signer.KeyID()
			}()
		}
		close(start)
		wg.Wait()
		for i, err := range failures {
			if err != nil {
				t.Fatalf("round %d: replica %d: %v", round, i, err)
			}
		}
		var stored int64
		db.Raw(`SELECT count(*) FROM delegation_signing_keys`).Scan(&stored)
		if stored != 1 {
			t.Fatalf("round %d: %d keys were created, want 1", round, stored)
		}
		for i := 1; i < replicas; i++ {
			if kids[i] != kids[0] {
				t.Fatalf("round %d: replicas signed with different keys: %q and %q", round, kids[0], kids[i])
			}
		}
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM delegation_signing_keys`) })
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
