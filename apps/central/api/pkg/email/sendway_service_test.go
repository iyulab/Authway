package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// newTestSendwayService points a SendwayEmailService at an httptest TLS server —
// sendEmail hardcodes the "https://" scheme, so a plain httptest.NewServer cannot
// be substituted; the returned service's unexported httpClient is swapped for the
// TLS server's own client (which trusts its self-signed cert), and baseURL is set
// to the server's host:port with the scheme stripped.
func newTestSendwayService(t *testing.T, ts *httptest.Server) *SendwayEmailService {
	t.Helper()
	svc := NewSendwayEmailService(SendwayEmailConfig{
		BaseURL:     strings.TrimPrefix(ts.URL, "https://"),
		APIKey:      "test-key",
		FrontendURL: "https://app.example.com",
	}, zap.NewNop())
	svc.httpClient = ts.Client()
	return svc
}

// TestSendwayEmailService_SendVerificationEmail_SendsExpectedRequest guards the
// request shape reaching Sendway — `to` as an array, plain-text body containing
// the verification link, the api key and idempotency headers.
func TestSendwayEmailService_SendVerificationEmail_SendsExpectedRequest(t *testing.T) {
	var captured sendwayEmailRequest
	var gotAPIKey, gotIdempotencyKey string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-Api-Key")
		gotIdempotencyKey = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sendwaySuccessResponse{ID: "msg-1"})
	}))
	defer ts.Close()

	svc := newTestSendwayService(t, ts)
	if err := svc.SendVerificationEmail("user@example.com", "tok-123"); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}

	if len(captured.To) != 1 || captured.To[0] != "user@example.com" {
		t.Fatalf("expected to=[user@example.com], got %v", captured.To)
	}
	if !strings.Contains(captured.Subject, "이메일 인증") {
		t.Fatalf("expected a verification subject, got %q", captured.Subject)
	}
	if !strings.Contains(captured.Body, "tok-123") {
		t.Fatal("expected the verification link (carrying the token) to be embedded in Body")
	}
	if !strings.Contains(captured.HtmlBody, "tok-123") || !strings.Contains(captured.HtmlBody, "<html") {
		t.Fatalf("expected an HTML alternative carrying the same link, got %q", captured.HtmlBody)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("expected X-Api-Key=test-key, got %q", gotAPIKey)
	}
	if gotIdempotencyKey == "" {
		t.Fatal("expected an Idempotency-Key header to be set")
	}
}

// TestSendwayEmailService_SendEmail_ErrorsOnUpstreamFailure guards both documented
// failure shapes: 400 (rejected recipient/missing field, "error"+"messageId") and
// 502 (send failed upstream, "detail"+"messageId") must both surface as a Go error.
func TestSendwayEmailService_SendEmail_ErrorsOnUpstreamFailure(t *testing.T) {
	t.Run("400 invalid recipient", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(sendwayErrorResponse{Error: "invalid recipient", MessageID: "msg-bad"})
		}))
		defer ts.Close()

		svc := newTestSendwayService(t, ts)
		err := svc.SendVerificationEmail("not-an-email", "tok")
		if err == nil {
			t.Fatal("expected an error on a 400 response")
		}
		if !strings.Contains(err.Error(), "msg-bad") {
			t.Fatalf("expected the error to carry the messageId for lookup, got %q", err.Error())
		}
	})

	t.Run("502 upstream send failure", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(sendwayErrorResponse{Detail: "smtp unreachable", MessageID: "msg-502"})
		}))
		defer ts.Close()

		svc := newTestSendwayService(t, ts)
		err := svc.SendVerificationEmail("user@example.com", "tok")
		if err == nil {
			t.Fatal("expected an error on a 502 response")
		}
		if !strings.Contains(err.Error(), "smtp unreachable") {
			t.Fatalf("expected the error to carry the upstream detail, got %q", err.Error())
		}
	})
}

// TestSendwayEmailService_SendMagicLinkEmail_SubjectVariesByIsNewUser guards the
// one behavioral fork SendMagicLinkEmail exposes over the wire.
func TestSendwayEmailService_SendMagicLinkEmail_SubjectVariesByIsNewUser(t *testing.T) {
	var captured sendwayEmailRequest
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sendwaySuccessResponse{ID: "msg-1"})
	}))
	defer ts.Close()
	svc := newTestSendwayService(t, ts)

	if err := svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", false); err != nil {
		t.Fatalf("SendMagicLinkEmail (returning user): %v", err)
	}
	if strings.Contains(captured.Subject, "가입") {
		t.Fatalf("expected a returning-user subject with no signup wording, got %q", captured.Subject)
	}

	if err := svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", true); err != nil {
		t.Fatalf("SendMagicLinkEmail (new user): %v", err)
	}
	if !strings.Contains(captured.Subject, "가입") {
		t.Fatalf("expected a new-user subject mentioning signup, got %q", captured.Subject)
	}
}

// TestSendwayEmailService_HtmlBody_PopulatedForEveryEmailType guards the docket
// iyulab/Sendway#105 upgrade — every send method must carry an HTML alternative
// alongside its plain-text body, not just SendVerificationEmail.
func TestSendwayEmailService_HtmlBody_PopulatedForEveryEmailType(t *testing.T) {
	var captured sendwayEmailRequest
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sendwaySuccessResponse{ID: "msg-1"})
	}))
	defer ts.Close()
	svc := newTestSendwayService(t, ts)

	cases := []struct {
		name string
		send func() error
	}{
		{"reset", func() error { return svc.SendPasswordResetEmail("user@example.com", "tok") }},
		{"invitation", func() error {
			return svc.SendInvitationEmail("user@example.com", "Alice", "Acme", "welcome", "https://app.example.com/invite?token=t")
		}},
		{"magic-link", func() error {
			return svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", false)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured = sendwayEmailRequest{}
			if err := tc.send(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if captured.HtmlBody == "" || !strings.Contains(captured.HtmlBody, "<html") {
				t.Fatalf("%s: expected a non-empty HTML alternative, got %q", tc.name, captured.HtmlBody)
			}
		})
	}
}
