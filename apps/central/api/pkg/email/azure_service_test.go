package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// newTestAzureService points an AzureEmailService at an httptest TLS server —
// sendEmail hardcodes the "https://" scheme (see its url := fmt.Sprintf(...)
// call), so a plain httptest.NewServer cannot be substituted; the returned
// service's unexported httpClient is swapped for the TLS server's own client
// (which trusts its self-signed cert), and baseURL is set to the server's
// host:port with the scheme stripped.
func newTestAzureService(t *testing.T, ts *httptest.Server) *AzureEmailService {
	t.Helper()
	svc := NewAzureEmailService(AzureEmailConfig{
		BaseURL:     strings.TrimPrefix(ts.URL, "https://"),
		FunctionKey: "test-key",
		FromEmail:   "noreply@authway.test",
		FromName:    "Authway Test",
		FrontendURL: "https://app.example.com",
	}, zap.NewNop())
	svc.httpClient = ts.Client()
	return svc
}

// TestAzureEmailService_SendVerificationEmail_SendsExpectedRequest guards the
// request shape reaching Azure Functions — subject, recipient, and an HTML
// body that actually contains the verification link.
func TestAzureEmailService_SendVerificationEmail_SendsExpectedRequest(t *testing.T) {
	var captured AzureEmailRequest
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AzureEmailResponse{Success: true, MessageID: "msg-1"})
	}))
	defer ts.Close()

	svc := newTestAzureService(t, ts)
	if err := svc.SendVerificationEmail("user@example.com", "tok-123"); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}

	if len(captured.To) != 1 || captured.To[0] != "user@example.com" {
		t.Fatalf("expected To=[user@example.com], got %v", captured.To)
	}
	if !strings.Contains(captured.Subject, "이메일 인증") {
		t.Fatalf("expected a verification subject, got %q", captured.Subject)
	}
	if !strings.Contains(captured.HtmlBody, "tok-123") {
		t.Fatal("expected the verification link (carrying the token) to be embedded in HtmlBody")
	}
	if captured.FromEmail != "noreply@authway.test" {
		t.Fatalf("expected the service's configured FromEmail to fill an unset request field, got %q", captured.FromEmail)
	}
}

// TestAzureEmailService_SendEmail_ErrorsOnUpstreamFailure guards both
// failure signals sendEmail checks: a non-200 status, and a 200 that itself
// reports success=false — either must surface as a Go error, never a silent
// nil (which would make the caller believe an email was actually sent).
func TestAzureEmailService_SendEmail_ErrorsOnUpstreamFailure(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(AzureEmailResponse{Success: false, Message: "internal error"})
		}))
		defer ts.Close()

		svc := newTestAzureService(t, ts)
		if err := svc.SendVerificationEmail("user@example.com", "tok"); err == nil {
			t.Fatal("expected an error on a non-200 upstream response")
		}
	})

	t.Run("200 with success=false", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(AzureEmailResponse{Success: false, Message: "provider rejected recipient"})
		}))
		defer ts.Close()

		svc := newTestAzureService(t, ts)
		if err := svc.SendVerificationEmail("user@example.com", "tok"); err == nil {
			t.Fatal("expected an error when the upstream body reports success=false even with HTTP 200")
		}
	})
}

// TestAzureEmailService_SendMagicLinkEmail_SubjectVariesByIsNewUser guards
// the one behavioral fork SendMagicLinkEmail exposes over the wire.
func TestAzureEmailService_SendMagicLinkEmail_SubjectVariesByIsNewUser(t *testing.T) {
	var captured AzureEmailRequest
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AzureEmailResponse{Success: true})
	}))
	defer ts.Close()
	svc := newTestAzureService(t, ts)

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
