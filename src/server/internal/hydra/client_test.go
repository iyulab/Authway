package hydra

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_RevokeUserSessions(t *testing.T) {
	tests := []struct {
		name           string
		subject        string
		loginStatus    int
		consentStatus  int
		expectError    bool
		errorContains  string
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
			name:           "login session revocation fails with 500",
			subject:        "user-789",
			loginStatus:    http.StatusInternalServerError,
			consentStatus:  http.StatusNoContent,
			expectError:    true,
			errorContains:  "failed to revoke login sessions",
		},
		{
			name:           "consent session revocation fails with 500",
			subject:        "user-999",
			loginStatus:    http.StatusNoContent,
			consentStatus:  http.StatusInternalServerError,
			expectError:    true,
			errorContains:  "failed to revoke consent sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			loginCalled := false
			consentCalled := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)

				// Check which endpoint was called
				if r.URL.Path == "/admin/oauth2/auth/sessions/login" {
					loginCalled = true
					assert.Equal(t, tt.subject, r.URL.Query().Get("subject"))
					w.WriteHeader(tt.loginStatus)
				} else if r.URL.Path == "/admin/oauth2/auth/sessions/consent" {
					consentCalled = true
					assert.Equal(t, tt.subject, r.URL.Query().Get("subject"))
					w.WriteHeader(tt.consentStatus)
				} else {
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

func TestClient_RevokeUserSessions_BothEndpointsCalled(t *testing.T) {
	loginCalls := 0
	consentCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/oauth2/auth/sessions/login" {
			loginCalls++
		} else if r.URL.Path == "/admin/oauth2/auth/sessions/consent" {
			consentCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.RevokeUserSessions("user-123")

	assert.NoError(t, err)
	assert.Equal(t, 1, loginCalls, "login endpoint should be called exactly once")
	assert.Equal(t, 1, consentCalls, "consent endpoint should be called exactly once")
}

func TestClient_RevokeUserSessions_OrderOfCalls(t *testing.T) {
	callOrder := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/oauth2/auth/sessions/login" {
			callOrder = append(callOrder, "login")
		} else if r.URL.Path == "/admin/oauth2/auth/sessions/consent" {
			callOrder = append(callOrder, "consent")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.RevokeUserSessions("user-123")

	require.NoError(t, err)
	require.Len(t, callOrder, 2)
	assert.Equal(t, "login", callOrder[0], "login session should be revoked first")
	assert.Equal(t, "consent", callOrder[1], "consent session should be revoked second")
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
