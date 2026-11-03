package pwned

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"authway/apps/central/api/pkg/ephemeral"
)

// SHA-1("password") = 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8.
const (
	passwordPrefix = "5BAA6"
	passwordSuffix = "1E4C9B93F3F0682250B6CF8331B7EE68FD8"
)

// service is a range service answering every prefix with body, and counting
// the calls and what they sent.
type service struct {
	*httptest.Server
	calls atomic.Int32
	paths chan string
}

func newService(t *testing.T, status int, body string) *service {
	t.Helper()
	s := &service{paths: make(chan string, 16)}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		s.paths <- r.URL.RequestURI() + " | Add-Padding=" + r.Header.Get("Add-Padding") + " | " + r.Header.Get("User-Agent")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func checker(t *testing.T, s *service, opts ...Option) *Checker {
	t.Helper()
	c, err := New(s.URL+"/range/", opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A line is SUFFIX:COUNT with CRLF endings, and the service pads an answer with
// made-up entries whose count is 0.
func answer(lines ...string) string { return strings.Join(lines, "\r\n") + "\r\n" }

func TestBreached_FindsAPasswordTheServiceLists(t *testing.T) {
	s := newService(t, http.StatusOK, answer(
		"0000000000000000000000000000000000A:4",
		passwordSuffix+":10434004",
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:0",
	))
	got, err := checker(t, s).Breached(context.Background(), "password")
	if err != nil || !got {
		t.Fatalf("Breached(password) = %v, %v; want true", got, err)
	}
}

func TestBreached_AnUnlistedPasswordPasses(t *testing.T) {
	s := newService(t, http.StatusOK, answer("0000000000000000000000000000000000A:4", "0000000000000000000000000000000000B:2"))
	got, err := checker(t, s).Breached(context.Background(), "password")
	if err != nil || got {
		t.Fatalf("Breached = %v, %v; want false", got, err)
	}
}

// A made-up entry is there to hide how many real ones a prefix has: one with
// the password's own suffix is not a breach.
func TestBreached_APaddingEntryIsNotABreach(t *testing.T) {
	s := newService(t, http.StatusOK, answer(passwordSuffix+":0"))
	got, err := checker(t, s).Breached(context.Background(), "password")
	if err != nil || got {
		t.Fatalf("Breached with a padding entry = %v, %v; want false", got, err)
	}
}

func TestBreached_AMirrorMayAnswerInLowerCase(t *testing.T) {
	s := newService(t, http.StatusOK, answer(strings.ToLower(passwordSuffix)+":3"))
	if got, err := checker(t, s).Breached(context.Background(), "password"); err != nil || !got {
		t.Fatalf("Breached = %v, %v; want true", got, err)
	}
}

// Only the first five characters of the hash go out: not the password, not the
// rest of the hash.
func TestBreached_SendsOnlyTheHashPrefix(t *testing.T) {
	s := newService(t, http.StatusOK, answer("0000000000000000000000000000000000A:4"))
	if _, err := checker(t, s).Breached(context.Background(), "password"); err != nil {
		t.Fatal(err)
	}
	sent := <-s.paths
	if want := "/range/" + passwordPrefix + " | Add-Padding=true | Authway"; sent != want {
		t.Errorf("the service was sent %q, want %q", sent, want)
	}
	for _, leak := range []string{"password", passwordSuffix, passwordSuffix[:10]} {
		if strings.Contains(sent, leak) {
			t.Errorf("the request carries %q: %s", leak, sent)
		}
	}
}

// A prefix asked about again is answered from the cache — for the same
// password and for another that shares the prefix.
func TestBreached_AnswersAPrefixFromTheCacheTheSecondTime(t *testing.T) {
	s := newService(t, http.StatusOK, answer(passwordSuffix+":10434004"))
	c := checker(t, s, WithCache(ephemeral.NewMemory(context.Background()), time.Minute))
	for i := 0; i < 3; i++ {
		if got, err := c.Breached(context.Background(), "password"); err != nil || !got {
			t.Fatalf("call %d: Breached = %v, %v; want true", i, got, err)
		}
	}
	if n := s.calls.Load(); n != 1 {
		t.Errorf("the service was called %d times for one prefix, want 1", n)
	}
}

func TestBreached_AFailureIsAnErrorNotAnAnswer(t *testing.T) {
	t.Run("a refusal", func(t *testing.T) {
		s := newService(t, http.StatusServiceUnavailable, "")
		if got, err := checker(t, s).Breached(context.Background(), "password"); err == nil || got {
			t.Errorf("Breached = %v, %v; want an error", got, err)
		}
	})
	t.Run("a page that is not a list", func(t *testing.T) {
		s := newService(t, http.StatusOK, "<html><body>Bad gateway</body></html>")
		if _, err := checker(t, s).Breached(context.Background(), "password"); !errors.Is(err, ErrUnexpectedAnswer) {
			t.Errorf("err = %v, want ErrUnexpectedAnswer", err)
		}
	})
	t.Run("an answer too large", func(t *testing.T) {
		s := newService(t, http.StatusOK, strings.Repeat("A", maxBody+10))
		if _, err := checker(t, s).Breached(context.Background(), "password"); err == nil {
			t.Error("a body past the limit was accepted")
		}
	})
	t.Run("no answer in time", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}))
		defer slow.Close()
		c, _ := New(slow.URL+"/range/", WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
		started := time.Now()
		if _, err := c.Breached(context.Background(), "password"); err == nil {
			t.Error("a call that timed out was not an error")
		}
		if time.Since(started) > time.Second {
			t.Errorf("waited %v for a service that did not answer", time.Since(started))
		}
	})
	t.Run("a failure is not cached", func(t *testing.T) {
		s := newService(t, http.StatusInternalServerError, "")
		c := checker(t, s, WithCache(ephemeral.NewMemory(context.Background()), time.Minute))
		_, _ = c.Breached(context.Background(), "password")
		_, _ = c.Breached(context.Background(), "password")
		if n := s.calls.Load(); n != 2 {
			t.Errorf("a failed call was remembered: %d calls, want 2", n)
		}
	})
}

// A service that answers with a redirect is not followed: the address is the
// deployment's own, and a redirect would send this process wherever the service
// likes.
func TestBreached_DoesNotFollowARedirect(t *testing.T) {
	target := newService(t, http.StatusOK, answer(passwordSuffix+":1"))
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/range/"+passwordPrefix, http.StatusFound)
	}))
	defer redirecting.Close()
	c, err := New(redirecting.URL + "/range/")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Breached(context.Background(), "password"); err == nil || got {
		t.Errorf("Breached = %v, %v; want an error for a redirect", got, err)
	}
	if n := target.calls.Load(); n != 0 {
		t.Errorf("the redirect was followed: the target was called %d times", n)
	}
}

// An address that carries a query, a fragment or credentials would have the
// prefix appended to it and would be written to the log on every failure.
func TestNew_RefusesAnAddressThatIsNotAPlainPath(t *testing.T) {
	for _, bad := range []string{
		"https://pwned.example/range/?key=secret",
		"https://pwned.example/range/#top",
		"https://user:secret@pwned.example/range/",
	} {
		_, err := New(bad)
		if err == nil {
			t.Errorf("New(%q) did not refuse the address", bad)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("the refusal for %q repeats the address: %v", bad, err)
		}
	}
}

// A real service pads every answer; an empty one is something else answering
// for it, and must not switch the check off for the prefix for an hour.
func TestBreached_AnAnswerThatListsNothingIsNotRemembered(t *testing.T) {
	s := newService(t, http.StatusOK, "")
	c := checker(t, s, WithCache(ephemeral.NewMemory(context.Background()), time.Minute))
	for i := 0; i < 2; i++ {
		if got, err := c.Breached(context.Background(), "password"); err != nil || got {
			t.Fatalf("Breached = %v, %v; want false, no error", got, err)
		}
	}
	if n := s.calls.Load(); n != 2 {
		t.Errorf("an empty answer was cached: %d calls, want 2", n)
	}
}

// The cache takes a bounded number of answers in a lifetime, so that a stream of
// random passwords cannot fill the memory of a deployment that keeps its state
// in the process.
func TestBreached_PutsOnlyAFewAnswersInTheCache(t *testing.T) {
	s := newService(t, http.StatusOK, answer("0000000000000000000000000000000000A:4"))
	c := checker(t, s, WithCache(ephemeral.NewMemory(context.Background()), time.Hour))
	c.maxFills = 2
	passwords := []string{"alpha-1", "bravo-2", "charlie-3"} // three different prefixes
	for round := 0; round < 2; round++ {
		for _, password := range passwords {
			if _, err := c.Breached(context.Background(), password); err != nil {
				t.Fatal(err)
			}
		}
	}
	// First round: three calls, two answers kept. Second round: the third asks again.
	if n := s.calls.Load(); n != 4 {
		t.Errorf("%d calls, want 4 (3, then 1 for the answer that was not kept)", n)
	}
}

// A service held by calls that do not finish: the places for calls are taken,
// and one more call waits for a place only as long as its caller's time allows.
// It is not left waiting behind them, and it is not made to look like an answer.
func TestBreached_WaitsForAPlaceOnlyAsLongAsTheCallerCanAfford(t *testing.T) {
	c, release, entered := busyService(t)
	done := make(chan error, 1)
	go func() { _, err := c.Breached(context.Background(), "password"); done <- err }()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := c.Breached(ctx, "another"); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want ErrBusy when no place frees up in time", err)
	}
	if waited := time.Since(started); waited < 100*time.Millisecond || waited > time.Second {
		t.Errorf("waited %v for a place, want about the caller's 150ms", waited)
	}
	close(release)
	if err := <-done; err != nil {
		t.Errorf("the first call: %v", err)
	}
}

// A short burst is still checked: a call that finds every place taken gets one
// when a call ahead of it finishes.
func TestBreached_ABurstWaitsItsTurn(t *testing.T) {
	c, release, entered := busyService(t)
	done := make(chan error, 2)
	go func() { _, err := c.Breached(context.Background(), "password"); done <- err }()
	<-entered
	go func() { _, err := c.Breached(context.Background(), "another"); done <- err }()

	time.Sleep(100 * time.Millisecond) // the second is waiting for the one place
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("a call in the burst: %v", err)
		}
	}
}

// busyService is a Checker with one place for calls, against a service that
// answers only once release is closed; entered signals each call that reaches it.
func busyService(t *testing.T) (c *Checker, release chan struct{}, entered chan struct{}) {
	t.Helper()
	release = make(chan struct{})
	entered = make(chan struct{}, 4)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(answer("0000000000000000000000000000000000A:4")))
	}))
	t.Cleanup(slow.Close)
	c, err := New(slow.URL+"/range/", WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	c.slots = make(chan struct{}, 1)
	return c, release, entered
}

func TestNew_TakesOnlyAnHTTPAddressAndAddsTheSlash(t *testing.T) {
	for _, bad := range []string{"", "pwned.example/range/", "ftp://pwned.example/range/", "https://"} {
		if _, err := New(bad); err == nil {
			t.Errorf("New(%q) did not refuse the address", bad)
		}
	}
	s := newService(t, http.StatusOK, answer(passwordSuffix+":1"))
	c, err := New(s.URL + "/range") // no trailing slash
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Breached(context.Background(), "password"); err != nil || !got {
		t.Errorf("Breached = %v, %v; want true", got, err)
	}
	if sent := <-s.paths; !strings.HasPrefix(sent, "/range/"+passwordPrefix) {
		t.Errorf("the request went to %q", sent)
	}
}
