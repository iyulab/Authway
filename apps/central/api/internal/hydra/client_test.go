package hydra

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient_RevokeUserSessions(t *testing.T) {
	tests := []struct {
		name          string
		subject       string
		loginStatus   int
		consentStatus int
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful session revocation",
			subject:       "user-123",
			loginStatus:   http.StatusNoContent,
			consentStatus: http.StatusNoContent,
			expectError:   false,
		},
		{
			name:          "successful revocation with 404 (no sessions)",
			subject:       "user-456",
			loginStatus:   http.StatusNotFound,
			consentStatus: http.StatusNotFound,
			expectError:   false,
		},
		{
			name:          "login session revocation fails with 500",
			subject:       "user-789",
			loginStatus:   http.StatusInternalServerError,
			consentStatus: http.StatusNoContent,
			expectError:   true,
			errorContains: "failed to revoke login sessions",
		},
		{
			name:          "consent session revocation fails with 500",
			subject:       "user-999",
			loginStatus:   http.StatusNoContent,
			consentStatus: http.StatusInternalServerError,
			expectError:   true,
			errorContains: "failed to revoke consent sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			loginCalled := false
			consentCalled := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The subject's sessions are listed first; here it has none.
				if r.Method == http.MethodGet && r.URL.Path == "/admin/oauth2/auth/sessions/consent" {
					_, _ = w.Write([]byte("[]"))
					return
				}
				assert.Equal(t, http.MethodDelete, r.Method)

				// Check which endpoint was called
				switch r.URL.Path {
				case "/admin/oauth2/auth/sessions/login":
					loginCalled = true
					assert.Equal(t, tt.subject, r.URL.Query().Get("subject"))
					w.WriteHeader(tt.loginStatus)
				case "/admin/oauth2/auth/sessions/consent":
					consentCalled = true
					assert.Equal(t, tt.subject, r.URL.Query().Get("subject"))
					w.WriteHeader(tt.consentStatus)
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer server.Close()

			// Create client with mock server URL
			client := NewClient(server.URL)

			// Execute revocation
			err := client.RevokeUserSessions(tt.subject)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.True(t, loginCalled, "login session revocation should be called")
				assert.True(t, consentCalled, "consent session revocation should be called")
			}
		})
	}
}

func TestClient_RevokeUserSessions_EmptySubject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)

	// Revoke with empty subject should still call the endpoints
	err := client.RevokeUserSessions("")
	assert.NoError(t, err)
}

func TestClient_RevokeUserSessions_InvalidURL(t *testing.T) {
	client := NewClient("http://invalid-url-that-does-not-exist:99999")

	err := client.RevokeUserSessions("user-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to revoke")
}

func TestClient_RevokeUserSessions_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		subject string
	}{
		{
			name:    "UUID subject",
			subject: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:    "subject with spaces",
			subject: "user with spaces",
		},
		{
			name:    "subject with special chars",
			subject: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedSubject := ""

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedSubject = r.URL.Query().Get("subject")
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte("[]"))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			client := NewClient(server.URL)
			err := client.RevokeUserSessions(tt.subject)

			assert.NoError(t, err)
			assert.Equal(t, tt.subject, receivedSubject)
		})
	}
}

// Ending every session but one: the consents of the other sessions are
// revoked each by its own id — that is what ends their tokens — and their
// login sessions ended by id. The kept session's consent and login session
// are not touched, and no call names the whole subject.
func TestClient_RevokeOtherSessions_KeepsTheNamedSession(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"consent_request_id":"consent-kept","consent_request":{"login_session_id":"session-kept"}},
				{"consent_request_id":"consent-other","consent_request":{"login_session_id":"session-other"}},
				{"consent_request_id":"consent-other-2","consent_request":{"login_session_id":"session-other"}},
				{"consent_request_id":"consent-orphan","consent_request":{"login_session_id":""}}
			]`))
			return
		}
		deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/admin/oauth2/auth/sessions/")+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL).RevokeOtherSessions("user-1", "session-kept"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"login?sid=session-other",
		"consent?consent_request_id=consent-other",
		"consent?consent_request_id=consent-other-2",
		"consent?consent_request_id=consent-orphan",
	}
	if strings.Join(deleted, " ") != strings.Join(want, " ") {
		t.Errorf("ended\n  %v\nwant\n  %v", deleted, want)
	}
}

func TestClient_RevokeOtherSessions_WithNoSessionToKeepEndsThemAll(t *testing.T) {
	var bySubject int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Query().Get("subject") == "user-1" {
			bySubject++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient(server.URL).RevokeOtherSessions("user-1", ""); err != nil {
		t.Fatal(err)
	}
	if bySubject != 2 {
		t.Errorf("%d calls named the whole subject, want the login sessions and the consents", bySubject)
	}
}
