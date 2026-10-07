package hydra

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every call on a client-supplied challenge must tell "the caller sent a bad
// or spent id" apart from "Hydra failed", so handlers can answer 4xx vs 5xx.
func TestChallengeLookup_ClassifiesFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unknown challenge", http.StatusNotFound, ErrFlowNotFound},
		{"already handled challenge", http.StatusGone, ErrFlowExpired},
		{"hydra failure", http.StatusInternalServerError, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"x"}`))
			}))
			defer server.Close()
			c := NewClient(server.URL)

			calls := map[string]func() error{
				"get login":      func() error { _, err := c.GetLoginRequest("x"); return err },
				"accept login":   func() error { _, err := c.AcceptLoginRequest("x", &AcceptLoginRequest{}); return err },
				"reject login":   func() error { _, err := c.RejectLoginRequest("x", "access_denied", ""); return err },
				"get consent":    func() error { _, err := c.GetConsentRequest("x"); return err },
				"accept consent": func() error { _, err := c.AcceptConsentRequest("x", &AcceptConsentRequest{}); return err },
				"reject consent": func() error { _, err := c.RejectConsentRequest("x", "access_denied", ""); return err },
				"get logout":     func() error { _, err := c.GetLogoutRequest("x"); return err },
				"accept logout":  func() error { _, err := c.AcceptLogoutRequest("x"); return err },
			}
			for name, call := range calls {
				err := call()
				if err == nil {
					t.Fatalf("%s: want error for status %d", name, tc.status)
				}
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Errorf("%s: err = %v, want %v", name, err, tc.want)
				}
				if tc.want == nil && (errors.Is(err, ErrFlowNotFound) || errors.Is(err, ErrFlowExpired)) {
					t.Errorf("%s: a Hydra failure was classified as a client error: %v", name, err)
				}
			}
		})
	}
}
