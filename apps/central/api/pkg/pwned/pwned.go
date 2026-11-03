// Package pwned asks a range service — the Pwned Passwords range API, or a
// mirror of it — whether a password is in a known data breach.
//
// The password never leaves this process. A password is hashed with SHA-1 (the
// hash the service is keyed by; it is a lookup key here, not protection), only
// the first five characters of the hash are sent, and the service answers with
// every hash suffix that starts the same way. The remaining thirty-five
// characters are compared here (k-anonymity), so the service cannot tell which
// password was asked about.
package pwned

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// prefixLength is how much of the hash is sent.
	prefixLength = 5
	// suffixLength is the rest of a SHA-1 hash in hexadecimal.
	suffixLength = 40 - prefixLength

	// requestTimeout bounds one call to the service while a person waits.
	requestTimeout = 2 * time.Second
	// maxBody bounds what is read of an answer. A real one is about 40 KiB.
	maxBody = 4 << 20
	// DefaultCacheTTL is how long an answer for a prefix is kept.
	DefaultCacheTTL = time.Hour

	cacheKeyPrefix = "pwned:range:"
)

// ErrUnexpectedAnswer is an answer that is not a list of hash suffixes.
var ErrUnexpectedAnswer = errors.New("the range service did not answer with a list of hash suffixes")

// Cache keeps what the service answered for a prefix, so a prefix asked about
// again is not asked of the service again. ephemeral.Store is one.
type Cache interface {
	SetValue(ctx context.Context, key, value string, ttl time.Duration) error
	Value(ctx context.Context, key string) (value string, ok bool, err error)
}

// Checker asks a range service about passwords.
type Checker struct {
	baseURL  string
	client   *http.Client
	cache    Cache
	cacheTTL time.Duration
}

// Option configures a Checker.
type Option func(*Checker)

// WithCache has answers kept in cache for ttl (DefaultCacheTTL when ttl is not
// positive).
func WithCache(cache Cache, ttl time.Duration) Option {
	return func(c *Checker) {
		c.cache = cache
		if ttl > 0 {
			c.cacheTTL = ttl
		}
	}
}

// WithHTTPClient makes the calls with client instead of the default one.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Checker) { c.client = client }
}

// New returns a Checker for the range service at baseURL, whose address plus a
// five-character prefix is the address of an answer
// (https://api.pwnedpasswords.com/range/).
//
// The address is the deployment's own setting, not something a caller gave:
// a mirror inside a closed network is the reason to set it, so it may name a
// private address.
func New(baseURL string, opts ...Option) (*Checker, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("the range service address %q is not an http or https address", baseURL)
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	c := &Checker{
		baseURL:  baseURL,
		client:   &http.Client{Timeout: requestTimeout},
		cacheTTL: DefaultCacheTTL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Breached reports whether password is in the service's list. An error means
// the service could not be asked or did not answer sensibly: nothing is known,
// and whether to go on without knowing is the caller's to decide.
func (c *Checker) Breached(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hash[:prefixLength], hash[prefixLength:]

	suffixes, err := c.suffixes(ctx, prefix)
	if err != nil {
		return false, err
	}
	return strings.Contains(suffixes, "\n"+suffix+"\n"), nil
}

// suffixes returns the hash suffixes the service lists for prefix, one per
// line between newlines, so that one can be looked for with its newlines
// around it.
func (c *Checker) suffixes(ctx context.Context, prefix string) (string, error) {
	key := cacheKeyPrefix + prefix
	if c.cache != nil {
		if cached, ok, err := c.cache.Value(ctx, key); err == nil && ok {
			return cached, nil
		}
	}

	body, err := c.fetch(ctx, prefix)
	if err != nil {
		return "", err
	}
	list, err := parse(body)
	if err != nil {
		return "", err
	}
	if c.cache != nil {
		// A cache that cannot be written costs only another call next time.
		_ = c.cache.SetValue(ctx, key, list, c.cacheTTL)
	}
	return list, nil
}

func (c *Checker) fetch(ctx context.Context, prefix string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+prefix, nil)
	if err != nil {
		return nil, err
	}
	// Asks the service to add made-up entries, so that the size of the answer
	// does not say how many real ones a prefix has.
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "Authway")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the range service answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("the range service's answer is larger than %d bytes", maxBody)
	}
	return body, nil
}

// parse keeps the suffixes of an answer whose count is above zero: a line is
// "SUFFIX:COUNT", and the made-up entries added for padding have a count of 0.
// An answer with text in it and no line in that form is not an answer.
func parse(body []byte) (string, error) {
	var kept strings.Builder
	kept.WriteString("\n")
	lines, valid := 0, 0
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines++
		suffix, count, found := strings.Cut(line, ":")
		if !found || len(suffix) != suffixLength {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if err != nil {
			continue
		}
		valid++
		if n > 0 {
			kept.WriteString(strings.ToUpper(suffix))
			kept.WriteString("\n")
		}
	}
	if lines > 0 && valid == 0 {
		return "", ErrUnexpectedAnswer
	}
	return kept.String(), nil
}
