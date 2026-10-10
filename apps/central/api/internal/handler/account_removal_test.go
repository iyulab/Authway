package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/user"
)

// deletingUsers is a user service whose accounts can be deleted, recording
// the order of what happened alongside the authorization server stub.
type deletingUsers struct {
	*fakeUserService
	log *eventLog
}

func (d *deletingUsers) Delete(id uuid.UUID) error {
	d.log.add("delete account")
	kept := d.users[:0]
	for _, u := range d.users {
		if u.ID != id {
			kept = append(kept, u)
		}
	}
	d.users = kept
	return nil
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

// revokingHydra answers the session revocation calls with status, logging
// each.
func revokingHydra(t *testing.T, log *eventLog, status int) *hydra.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/oauth2/auth/sessions/") {
			log.add("revoke " + strings.TrimPrefix(r.URL.Path, "/admin/oauth2/auth/sessions/") + " of " + r.URL.Query().Get("subject"))
			w.WriteHeader(status)
			return
		}
		t.Errorf("unexpected request to the authorization server: %s %s", r.Method, r.URL)
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL)
}

// An account's sessions, consents and tokens must end before the account is
// deleted — and if they cannot be ended the account must stay, so the removal
// can be tried again rather than leaving tokens nobody can find to revoke.
func TestRemoveAccount_RevokesBeforeDeletingAndKeepsTheAccountIfItCannot(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "leaving@example.com"}

	log := &eventLog{}
	users := &deletingUsers{fakeUserService: newFakeUserService(u), log: log}
	if err := removeAccount(revokingHydra(t, log, http.StatusNoContent), users, u.ID); err != nil {
		t.Fatalf("removeAccount: %v", err)
	}
	want := []string{"revoke login of " + u.ID.String(), "revoke consent of " + u.ID.String(), "delete account"}
	if strings.Join(log.events, " | ") != strings.Join(want, " | ") {
		t.Errorf("order = %v, want %v", log.events, want)
	}

	log = &eventLog{}
	users = &deletingUsers{fakeUserService: newFakeUserService(u), log: log}
	if err := removeAccount(revokingHydra(t, log, http.StatusInternalServerError), users, u.ID); err == nil {
		t.Fatal("removeAccount succeeded although the sessions could not be revoked")
	}
	for _, e := range log.events {
		if e == "delete account" {
			t.Errorf("the account was deleted although its sessions could not be revoked: %v", log.events)
		}
	}
}

// deleteMeApp serves DELETE /profile/me as the signed-in user, whose token
// says they authenticated at authTime (zero: the token does not say).
func deleteMeApp(t *testing.T, h *AuthHandler, userID uuid.UUID, authTime time.Time) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Delete("/profile/me", func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		if !authTime.IsZero() {
			c.Locals("auth_time", authTime)
		}
		return c.Next()
	}, h.DeleteMe)
	return app
}

func TestDeleteMe_RequiresARecentSignIn(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "member@example.com"}

	for name, authTime := range map[string]time.Time{
		"a token that does not say when its user signed in": {},
		"a sign-in longer ago than allowed":                 time.Now().Add(-recentAuthentication - time.Minute),
	} {
		log := &eventLog{}
		users := &deletingUsers{fakeUserService: newFakeUserService(u), log: log}
		h := &AuthHandler{userService: users, hydraClient: revokingHydra(t, log, http.StatusNoContent), logger: zap.NewNop()}

		res, err := deleteMeApp(t, h, u.ID, authTime).Test(httptest.NewRequest(http.MethodDelete, "/profile/me", nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		var answer struct {
			Code   string `json:"code"`
			MaxAge int    `json:"max_age"`
		}
		_ = json.Unmarshal(body, &answer)
		if res.StatusCode != http.StatusUnauthorized || answer.Code != "insufficient_user_authentication" || answer.MaxAge != int(recentAuthentication.Seconds()) {
			t.Errorf("%s: answered %d %s, want 401 insufficient_user_authentication with max_age", name, res.StatusCode, body)
		}
		if challenge := res.Header.Get("WWW-Authenticate"); !strings.Contains(challenge, `error="insufficient_user_authentication"`) || !strings.Contains(challenge, `max_age="600"`) {
			t.Errorf("%s: WWW-Authenticate = %q, want the step-up challenge with max_age", name, challenge)
		}
		if len(log.events) != 0 {
			t.Errorf("%s: something was revoked or deleted without a recent sign-in: %v", name, log.events)
		}
	}
}

func TestDeleteMe_DeletesTheAccountOfAUserWhoJustSignedIn(t *testing.T) {
	u := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "member@example.com"}
	log := &eventLog{}
	users := &deletingUsers{fakeUserService: newFakeUserService(u), log: log}
	h := &AuthHandler{userService: users, hydraClient: revokingHydra(t, log, http.StatusNoContent), logger: zap.NewNop()}

	res, err := deleteMeApp(t, h, u.ID, time.Now().Add(-time.Minute)).Test(httptest.NewRequest(http.MethodDelete, "/profile/me", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("answered %d %s, want 200", res.StatusCode, body)
	}
	if _, err := users.GetByID(u.ID); err == nil {
		t.Error("the account still exists")
	}
	if len(log.events) != 3 || log.events[2] != "delete account" {
		t.Errorf("events = %v, want both revocations and then the deletion", log.events)
	}
}

func newAuthTimeStore(t *testing.T) (*AuthTimeStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return NewAuthTimeStore(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestAuthTimeStore_RemembersWhenASessionAuthenticated(t *testing.T) {
	store, mr := newAuthTimeStore(t)
	at := time.Now().Add(-3 * time.Minute).Truncate(time.Second)

	if _, ok := store.Lookup("session-1"); ok {
		t.Fatal("an unknown session has an authentication time")
	}
	store.Record("session-1", at)
	if got, ok := store.Lookup("session-1"); !ok || !got.Equal(at) {
		t.Fatalf("Lookup = %v %v, want %v", got, ok, at)
	}

	mr.FastForward(authTimeTTL + time.Minute)
	if _, ok := store.Lookup("session-1"); ok {
		t.Error("the authentication time outlived its limit")
	}

	// A store without Redis, and a request without a session id, record nothing.
	var none *AuthTimeStore
	none.Record("session-2", at)
	if _, ok := none.Lookup("session-2"); ok {
		t.Error("a nil store returned a time")
	}
	store.Record("", at)
	if _, ok := store.Lookup(""); ok {
		t.Error("an empty session id has an authentication time")
	}
}

// The access token gets auth_time; the map shared with the ID token must not,
// because the authorization server sets that token's auth_time itself.
func TestWithAuthTime_AddsItToACopy(t *testing.T) {
	store, _ := newAuthTimeStore(t)
	at := time.Now().Truncate(time.Second)
	store.Record("session-1", at)
	shared := map[string]any{"tenant_id": "t"}

	access := withAuthTime(shared, store, "session-1")
	if access["auth_time"] != at.Unix() || access["tenant_id"] != "t" {
		t.Errorf("access token claims = %v", access)
	}
	if _, leaked := shared["auth_time"]; leaked {
		t.Error("auth_time was written into the map shared with the ID token")
	}
	if _, has := withAuthTime(shared, store, "unknown-session")["auth_time"]; has {
		t.Error("a session without a known authentication time got auth_time")
	}
}

// acceptAuthenticatedLogin records the time only for a login the
// authorization server accepted.
func TestAcceptAuthenticatedLogin_RecordsOnlyAnAcceptedLogin(t *testing.T) {
	for name, tc := range map[string]struct {
		acceptStatus int
		wantRecorded bool
	}{
		"accepted": {http.StatusOK, true},
		"refused":  {http.StatusInternalServerError, false},
	} {
		store, _ := newAuthTimeStore(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/requests/login"):
				json.NewEncoder(w).Encode(map[string]any{"challenge": "c", "session_id": "session-9", "client": map[string]any{"client_id": "x"}})
			case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/requests/login/accept"):
				w.WriteHeader(tc.acceptStatus)
				json.NewEncoder(w).Encode(map[string]any{"redirect_to": "https://example.com/next"})
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			}
		}))
		_, err := acceptAuthenticatedLogin(hydra.NewClient(srv.URL), store, "c", &hydra.AcceptLoginRequest{Subject: "u"})
		srv.Close()
		if (err == nil) != tc.wantRecorded {
			t.Fatalf("%s: err = %v", name, err)
		}
		if _, ok := store.Lookup("session-9"); ok != tc.wantRecorded {
			t.Errorf("%s: recorded = %v, want %v", name, ok, tc.wantRecorded)
		}
	}
}
