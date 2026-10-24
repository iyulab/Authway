package ephemeral

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"
)

type entry struct {
	value   string
	fields  map[string]string
	expires time.Time
}

type memoryStore struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

// NewMemory is a Store in this process only — for a deployment of one
// instance. Expired keys are dropped as they are met and once a minute.
//
// What it is given is copied: a string read from a request (a route
// parameter, a query value) may share the request's buffer, which the server
// reuses for the next request.
func NewMemory(ctx context.Context) Store {
	s := &memoryStore{entries: map[string]entry{}, now: time.Now}
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweep()
			}
		}
	}()
	return s
}

func (s *memoryStore) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, e := range s.entries {
		if !now.Before(e.expires) {
			delete(s.entries, k)
		}
	}
}

// live returns key's entry if it has not expired. Call with mu held.
func (s *memoryStore) live(key string) (entry, bool) {
	e, ok := s.entries[key]
	if !ok {
		return entry{}, false
	}
	if !s.now().Before(e.expires) {
		delete(s.entries, key)
		return entry{}, false
	}
	return e, true
}

func (s *memoryStore) SetValue(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[strings.Clone(key)] = entry{value: strings.Clone(value), expires: s.now().Add(ttl)}
	return nil
}

func (s *memoryStore) Value(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok || e.fields != nil {
		return "", false, nil
	}
	return e.value, true, nil
}

func (s *memoryStore) Put(_ context.Context, key string, fields map[string]string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make(map[string]string, len(fields))
	for k, v := range fields {
		copied[strings.Clone(k)] = strings.Clone(v)
	}
	s.entries[strings.Clone(key)] = entry{fields: copied, expires: s.now().Add(ttl)}
	return nil
}

func (s *memoryStore) Fields(_ context.Context, key string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok || e.fields == nil {
		return map[string]string{}, nil
	}
	return maps.Clone(e.fields), nil
}

func (s *memoryStore) Take(_ context.Context, key string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	delete(s.entries, key)
	if !ok || e.fields == nil {
		return map[string]string{}, nil
	}
	return e.fields, nil
}

func (s *memoryStore) CountFailure(_ context.Context, key, field string, limit int) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok || e.fields == nil {
		return false, false, nil
	}
	n, _ := strconv.Atoi(e.fields[field])
	n++
	if n >= limit {
		delete(s.entries, key)
		return true, true, nil
	}
	e.fields[field] = strconv.Itoa(n)
	return false, true, nil
}

func (s *memoryStore) MarkOnce(_ context.Context, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.live(key); ok {
		return false, nil
	}
	s.entries[strings.Clone(key)] = entry{value: "1", expires: s.now().Add(ttl)}
	return true, nil
}

func (s *memoryStore) Count(_ context.Context, key string, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, _ := s.live(key)
	n, _ := strconv.ParseInt(e.value, 10, 64)
	n++
	s.entries[strings.Clone(key)] = entry{value: strconv.FormatInt(n, 10), expires: s.now().Add(window)}
	return n, nil
}

func (s *memoryStore) Remaining(_ context.Context, key string) (time.Duration, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok {
		return 0, false, nil
	}
	return e.expires.Sub(s.now()), true, nil
}

func (s *memoryStore) Remove(_ context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.entries, k)
	}
	return nil
}
