package hydra

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// sessionsServer stands in for the authorization server's session admin API
// and records the calls it receives, in order.
func sessionsServer(t *testing.T, listStatus int, listBody string) (*Client, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Method == http.MethodGet {
			w.WriteHeader(listStatus)
			_, _ = w.Write([]byte(listBody))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL), &calls
}

// Applications are told about a logout only when a session is ended by its
// id, so each of the user's sessions is ended that way before the by-subject
// calls remove what is left.
func TestRevokeUserSessions_EndsEachSessionByIDBeforeTheRest(t *testing.T) {
	client, calls := sessionsServer(t, http.StatusOK, `[
		{"consent_request": {"login_session_id": "session-1"}},
		{"consent_request": {"login_session_id": "session-1"}},
		{"consent_request": {"login_session_id": "session-2"}},
		{"consent_request": {}}
	]`)

	if err := client.RevokeUserSessions("user 1&x"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /admin/oauth2/auth/sessions/consent?page_size=500&subject=user+1%26x",
		"DELETE /admin/oauth2/auth/sessions/login?sid=session-1",
		"DELETE /admin/oauth2/auth/sessions/login?sid=session-2",
		"DELETE /admin/oauth2/auth/sessions/login?subject=user+1%26x",
		"DELETE /admin/oauth2/auth/sessions/consent?subject=user+1%26x&all=true",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls:\n%q\nwant:\n%q", *calls, want)
	}
}

// When the sessions cannot be listed they are still ended; what is lost is
// telling the applications, and the caller hears that as its own error.
func TestRevokeUserSessions_StillEndsTheSessionsWhenTheyCannotBeListed(t *testing.T) {
	client, calls := sessionsServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	err := client.RevokeUserSessions("user-1")
	if !errors.Is(err, ErrApplicationsNotNotified) {
		t.Errorf("error = %v, want ErrApplicationsNotNotified", err)
	}
	want := []string{
		"GET /admin/oauth2/auth/sessions/consent?page_size=500&subject=user-1",
		"DELETE /admin/oauth2/auth/sessions/login?subject=user-1",
		"DELETE /admin/oauth2/auth/sessions/consent?subject=user-1&all=true",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls:\n%q\nwant:\n%q", *calls, want)
	}
}
