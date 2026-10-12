package webhook

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"authway/apps/central/api/pkg/outbound"
)

// TestDeliveryClient_RefusesThisHostUnlessAllowed guards the check at connect
// time: a receiver on loopback is refused before any request is sent, and
// reached when private targets are allowed.
func TestDeliveryClient_RefusesThisHostUnlessAllowed(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer ts.Close()

	_, err := deliveryClient(outbound.Policy{}).Post(ts.URL, "application/json", nil)
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("expected ErrBlockedDestination, got %v", err)
	}
	if hits != 0 {
		t.Fatal("the request must not reach the receiver")
	}

	resp, err := deliveryClient(outbound.Policy{AllowPrivate: true}).Post(ts.URL, "application/json", nil)
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

	resp, err := deliveryClient(outbound.Policy{AllowPrivate: true}).Post(redirect.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || followed {
		t.Fatalf("status %d, followed %v; want 307 and not followed", resp.StatusCode, followed)
	}
}

func TestValidateURL_RefusesLocalAddressesUnlessAllowed(t *testing.T) {
	strict, lax := &service{}, &service{policy: outbound.Policy{AllowPrivate: true}}
	for _, u := range []string{"http://127.0.0.1:8080/hook", "http://localhost/hook", "https://10.0.0.5/hook", "http://[::1]/hook"} {
		if strict.validateURL(u) == nil {
			t.Errorf("%s: expected a refusal", u)
		}
		if err := lax.validateURL(u); err != nil {
			t.Errorf("%s with private targets allowed: %v", u, err)
		}
	}
	// No one's server: refused even where private targets are allowed.
	for _, u := range []string{"http://169.254.169.254/latest", "http://0.0.0.0/hook"} {
		if strict.validateURL(u) == nil || lax.validateURL(u) == nil {
			t.Errorf("%s: must be refused whatever the setting", u)
		}
	}
	if err := strict.validateURL("https://hooks.example.com/authway"); err != nil {
		t.Errorf("public URL refused: %v", err)
	}
}
