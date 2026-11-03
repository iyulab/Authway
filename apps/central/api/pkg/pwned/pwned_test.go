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
