package webhook

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The reference vector of the Standard Webhooks libraries: a receiver using
// any of them verifies what Sign produces.
const (
	refSecret    = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	refMsgID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
	refTimestamp = 1614265330
	refPayload   = `{"test": 2432232314}`
	refSignature = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
)

func TestSignMatchesReferenceVector(t *testing.T) {
	got, err := Sign(refSecret, refMsgID, refTimestamp, []byte(refPayload))
	if err != nil {
		t.Fatal(err)
	}
	if got != refSignature {
		t.Fatalf("Sign = %q, want %q", got, refSignature)
	}
}

func headers(id string, t int64, sig string) http.Header {
	h := http.Header{}
	h.Set(HeaderID, id)
	h.Set(HeaderTimestamp, strconv.FormatInt(t, 10))
	h.Set(HeaderSignature, sig)
	return h
}

func TestVerify(t *testing.T) {
	body := []byte(refPayload)
	at := time.Unix(refTimestamp, 0)
	good := headers(refMsgID, refTimestamp, refSignature)

	if !Verify(refSecret, good, body, 5*time.Minute, at.Add(time.Minute)) {
		t.Fatal("expected the reference delivery to verify")
	}
	// The prefix is optional, as in the reference libraries.
	if !Verify(strings.TrimPrefix(refSecret, "whsec_"), good, body, 5*time.Minute, at) {
		t.Fatal("expected the secret without its prefix to verify too")
	}
	// A secret change sends several signatures; one match is enough.
	if !Verify(refSecret, headers(refMsgID, refTimestamp, "v1,bm90LXRoaXMtb25l "+refSignature), body, 5*time.Minute, at) {
		t.Fatal("expected a matching signature among several to verify")
	}

	for name, ok := range map[string]bool{
		"tampered body":     Verify(refSecret, good, []byte(`{"test": 1}`), 5*time.Minute, at),
		"wrong secret":      Verify("whsec_"+strings.Repeat("A", 32), good, body, 5*time.Minute, at),
		"stale":             Verify(refSecret, good, body, 5*time.Minute, at.Add(6*time.Minute)),
		"from the future":   Verify(refSecret, good, body, 5*time.Minute, at.Add(-6*time.Minute)),
		"other event id":    Verify(refSecret, headers("msg_other", refTimestamp, refSignature), body, 5*time.Minute, at),
		"time changed":      Verify(refSecret, headers(refMsgID, refTimestamp+1, refSignature), body, 5*time.Minute, at),
		"unknown version":   Verify(refSecret, headers(refMsgID, refTimestamp, strings.Replace(refSignature, "v1,", "v2,", 1)), body, 5*time.Minute, at),
		"old t=,v1= form":   Verify(refSecret, headers(refMsgID, refTimestamp, "t=1614265330,v1=deadbeef"), body, 5*time.Minute, at),
		"no headers":        Verify(refSecret, http.Header{}, body, 5*time.Minute, at),
		"secret not base64": Verify("whsec_not base64!", good, body, 5*time.Minute, at),
		"empty secret":      Verify("whsec_", good, body, 5*time.Minute, at),
	} {
		if ok {
			t.Errorf("%s: expected Verify to fail", name)
		}
	}
}

func TestGeneratedSecretIsStandardForm(t *testing.T) {
	secret, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	key, err := signingKey(secret)
	if !strings.HasPrefix(secret, "whsec_") || err != nil || len(key) != 32 {
		t.Fatalf("generateSecret = %q (key %d bytes, err %v), want whsec_ + base64 of 32 bytes", secret, len(key), err)
	}
	if _, err := Sign("not-a-secret!", refMsgID, refTimestamp, nil); err == nil {
		t.Fatal("expected Sign to refuse a secret that is not whsec_<base64>")
	}
}
