package webhook

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1":       true,
		"::1":             true,
		"10.1.2.3":        true,
		"172.16.0.1":      true,
		"192.168.1.1":     true,
		"169.254.169.254": true,
		"100.100.0.229":   true,
		"0.0.0.0":         true,
		"fd00::1":         true,
		"fe80::1":         true,
		"::ffff:10.0.0.1": true,
		"224.0.0.1":       true,
		"93.184.216.34":   false,
		"2606:4700::1111": false,
	} {
		if got := blockedIP(net.ParseIP(addr)); got != want {
			t.Errorf("blockedIP(%s) = %v, want %v", addr, got, want)
		}
	}
}

// TestDeliveryClient_RefusesThisHostUnlessAllowed guards the check at connect
// time: a receiver on loopback is refused before any request is sent, and
// reached when private targets are allowed.
func TestDeliveryClient_RefusesThisHostUnlessAllowed(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer ts.Close()

	_, err := deliveryClient(false).Post(ts.URL, "application/json", nil)
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("expected ErrBlockedDestination, got %v", err)
	}
	if hits != 0 {
		t.Fatal("the request must not reach the receiver")
	}

	resp, err := deliveryClient(true).Post(ts.URL, "application/json", nil)
	if err != nil {
		t.Fatalf("allowed client: %v", err)
	}
	resp.Body.Close()
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

// TestDeliveryClient_DoesNotFollowRedirects guards that a redirect is the
// receiver's answer, not a way to send the delivery somewhere else.
func TestDeliveryClient_DoesNotFollowRedirects(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	resp, err := deliveryClient(true).Post(redirect.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || followed {
		t.Fatalf("status %d, followed %v; want 307 and not followed", resp.StatusCode, followed)
	}
}

func TestValidateURL_RefusesLocalAddressesUnlessAllowed(t *testing.T) {
	strict, lax := &service{}, &service{allowPrivateTargets: true}
	for _, u := range []string{"http://127.0.0.1:8080/hook", "http://localhost/hook", "https://10.0.0.5/hook", "http://[::1]/hook", "http://169.254.169.254/latest"} {
		if strict.validateURL(u) == nil {
			t.Errorf("%s: expected a refusal", u)
		}
		if err := lax.validateURL(u); err != nil {
			t.Errorf("%s with private targets allowed: %v", u, err)
		}
	}
	if err := strict.validateURL("https://hooks.example.com/authway"); err != nil {
		t.Errorf("public URL refused: %v", err)
	}
}
