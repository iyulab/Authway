package tokenhash

import (
	"encoding/base64"
	"regexp"
	"testing"
)

var hexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestHash_Deterministic(t *testing.T) {
	const tok = "some-reset-token-value"
	h1 := Hash(tok)
	h2 := Hash(tok)
	if h1 != h2 {
		t.Fatalf("Hash not deterministic: %q != %q", h1, h2)
	}
	if !hexRe.MatchString(h1) {
		t.Fatalf("Hash is not 64-char lowercase hex: %q", h1)
	}
}

func TestHash_DistinctInputsDistinctOutputs(t *testing.T) {
	if Hash("token-a") == Hash("token-b") {
		t.Fatal("distinct tokens produced identical hash")
	}
}

func TestHash_KnownVector(t *testing.T) {
	// SHA-256("") — guards against accidental algorithm/encoding change.
	const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := Hash(""); got != emptyDigest {
		t.Fatalf("Hash(\"\") = %q, want %q", got, emptyDigest)
	}
}

func TestGenerate_UniqueAndUsable(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := Generate()
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}
		if tok == "" {
			t.Fatal("Generate returned empty token")
		}
		if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
			t.Fatalf("token not valid unpadded base64url: %q (%v)", tok, err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("Generate produced duplicate token: %q", tok)
		}
		seen[tok] = struct{}{}
	}
}

// TestRoundTrip mirrors the store/verify cycle: a generated token, once
// hashed for storage, must re-hash to the same value on lookup.
func TestRoundTrip(t *testing.T) {
	tok, err := Generate()
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	stored := Hash(tok)          // written at rest
	lookup := Hash(tok)          // computed from the token presented on verify
	if stored != lookup {
		t.Fatalf("round-trip mismatch: stored %q != lookup %q", stored, lookup)
	}
}
