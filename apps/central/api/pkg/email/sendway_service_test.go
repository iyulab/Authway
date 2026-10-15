package email

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

// The mail service answers 202 Accepted once it has queued a message and
// delivers it afterwards; an older deployment answered 200 after sending.
// Either is the message handed over.
func TestSendwayEmailService_TakesAnAcceptedMessageAsHandedOver(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusAccepted} {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Location", "/messages/msg-1")
			w.WriteHeader(status)
			w.Write([]byte(`{"id":"msg-1","status":"Queued"}`))
		}))
		err := newTestSendwayService(t, ts).SendVerificationEmail("user@example.com", "tok")
		ts.Close()
		if err != nil {
			t.Errorf("status %d: %v, want the message taken as handed over", status, err)
		}
	}
}

// TestSendwayEmailService_SendEmail_ErrorsOnUpstreamFailure guards both error
// bodies the service reads: "error" (400 — rejected recipient or missing field)
// and "detail" (a 5xx from the service or a gateway in front of it). Both must
// surface as a Go error carrying the reason.
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

	if err := svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", "BCDF-GHJK", false); err != nil {
		t.Fatalf("SendMagicLinkEmail (returning user): %v", err)
	}
	if strings.Contains(captured.Subject, "가입") {
		t.Fatalf("expected a returning-user subject with no signup wording, got %q", captured.Subject)
	}

	if err := svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", "BCDF-GHJK", true); err != nil {
		t.Fatalf("SendMagicLinkEmail (new user): %v", err)
	}
	if !strings.Contains(captured.Subject, "가입") {
		t.Fatalf("expected a new-user subject mentioning signup, got %q", captured.Subject)
	}
}

// TestSendwayEmailService_HtmlBody_PopulatedForEveryEmailType: every send method
// must carry an HTML alternative alongside its plain-text body, not just
// SendVerificationEmail.
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
			return svc.SendMagicLinkEmail("user@example.com", "https://app.example.com/magic?token=t", "BCDF-GHJK", false)
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

// A retry of the same message must carry the same Idempotency-Key, and two
// different messages different keys — otherwise retrying either sends twice
// or collapses distinct mail into one.
func TestSendwayEmailService_IdempotencyKeyFollowsTheMessage(t *testing.T) {
	var keys []string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		json.NewEncoder(w).Encode(sendwaySuccessResponse{ID: "msg"})
	}))
	defer ts.Close()
	svc := newTestSendwayService(t, ts)

	for _, token := range []string{"tok-a", "tok-a", "tok-b"} {
		if err := svc.SendPasswordResetEmail("user@example.com", token); err != nil {
			t.Fatal(err)
		}
	}
	if keys[0] == "" || keys[0] != keys[1] {
		t.Errorf("same message sent twice carried keys %q and %q, want one non-empty key", keys[0], keys[1])
	}
	if keys[2] == keys[0] {
		t.Errorf("a different message reused key %q", keys[2])
	}
}

// Only failures worth retrying are marked transient: no answer, 429 and 5xx.
// A 4xx means the request itself is wrong and will fail again.
func TestSendwayEmailService_MarksTransientFailures(t *testing.T) {
	for status, transient := range map[int]bool{
		http.StatusBadGateway:         true,
		http.StatusServiceUnavailable: true,
		http.StatusTooManyRequests:    true,
		http.StatusBadRequest:         false,
		http.StatusUnauthorized:       false,
	} {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(sendwayErrorResponse{Error: "nope"})
		}))
		err := newTestSendwayService(t, ts).SendVerificationEmail("user@example.com", "tok")
		ts.Close()
		if err == nil {
			t.Fatalf("status %d: want an error", status)
		}
		if got := errors.Is(err, ErrTransient); got != transient {
			t.Errorf("status %d: transient = %v, want %v (%v)", status, got, transient, err)
		}
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	svc := newTestSendwayService(t, ts)
	ts.Close() // nothing answers
	if err := svc.SendVerificationEmail("user@example.com", "tok"); !errors.Is(err, ErrTransient) {
		t.Errorf("unreachable mail service: %v, want ErrTransient", err)
	}
}

// How long an attempt took is the only record of what the path to the mail
// service costs, so every outcome logs it: a message taken, a refusal, and no
// answer at all.
func TestSendwayEmailService_LogsHowLongEachAttemptTook(t *testing.T) {
	outcomes := map[string]http.HandlerFunc{
		"Email accepted by Sendway": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"id":"msg-1"}`))
		},
		"Sendway returned an error": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
		"Failed to send email via Sendway": nil, // the server is gone before the attempt
	}
	for message, handler := range outcomes {
		ts := httptest.NewTLSServer(handler)
		core, logs := observer.New(zap.InfoLevel)
		svc := newTestSendwayService(t, ts)
		svc.logger = zap.New(core)
		if handler == nil {
			ts.Close()
		}
		_ = svc.SendVerificationEmail("user@example.com", "tok")
		ts.Close()

		entries := logs.FilterMessage(message).All()
		if len(entries) != 1 {
			t.Errorf("%q: logged %d times, want once", message, len(entries))
			continue
		}
		if _, ok := entries[0].ContextMap()["took"]; !ok {
			t.Errorf("%q: the entry does not say how long the attempt took", message)
		}
	}
}
