package ephemeral

import (
	"context"
	"testing"
	"time"
	"unsafe"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// stores runs fn against each Store, so both answer the same way. advance
// moves the store's clock.
func stores(t *testing.T, fn func(t *testing.T, s Store, advance func(time.Duration))) {
	t.Run("memory", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		m := NewMemory(ctx).(*memoryStore)
		now := time.Now()
		m.now = func() time.Time { return now }
		fn(t, m, func(d time.Duration) { now = now.Add(d) })
	})
	t.Run("redis", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { client.Close() })
		fn(t, NewRedis(client), mr.FastForward)
	})
}

func TestValues(t *testing.T) {
	stores(t, func(t *testing.T, s Store, advance func(time.Duration)) {
		ctx := context.Background()
		if _, ok, _ := s.Value(ctx, "v"); ok {
			t.Fatal("an unset key holds a value")
		}
		s.SetValue(ctx, "v", "one", time.Minute)
		if v, ok, err := s.Value(ctx, "v"); err != nil || !ok || v != "one" {
			t.Fatalf("Value = %q %v %v", v, ok, err)
		}
		advance(2 * time.Minute)
		if _, ok, _ := s.Value(ctx, "v"); ok {
			t.Fatal("an expired value is still there")
		}
	})
}

func TestFieldsAndTake(t *testing.T) {
	stores(t, func(t *testing.T, s Store, advance func(time.Duration)) {
		ctx := context.Background()
		s.Put(ctx, "h", map[string]string{"a": "1", "b": "two"}, time.Minute)
		s.Put(ctx, "h", map[string]string{"a": "3"}, time.Minute)
		if f, _ := s.Fields(ctx, "h"); len(f) != 1 || f["a"] != "3" {
			t.Fatalf("Put must replace the fields, got %v", f)
		}
		if f, _ := s.Take(ctx, "h"); f["a"] != "3" {
			t.Fatalf("Take = %v", f)
		}
		if f, _ := s.Take(ctx, "h"); len(f) != 0 {
			t.Fatalf("a second Take got %v", f)
		}
		s.Put(ctx, "e", map[string]string{"a": "1"}, time.Minute)
		advance(2 * time.Minute)
		if f, _ := s.Fields(ctx, "e"); len(f) != 0 {
			t.Fatalf("expired fields are still there: %v", f)
		}
	})
}

func TestCountFailure(t *testing.T) {
	stores(t, func(t *testing.T, s Store, _ func(time.Duration)) {
		ctx := context.Background()
		if removed, found, _ := s.CountFailure(ctx, "c", "attempts", 3); found || removed {
			t.Fatal("a missing key was counted")
		}
		if f, _ := s.Fields(ctx, "c"); len(f) != 0 {
			t.Fatalf("counting a missing key created it: %v", f)
		}
		s.Put(ctx, "c", map[string]string{"user": "u", "attempts": "0"}, time.Minute)
		for i := 1; i < 3; i++ {
			if removed, found, _ := s.CountFailure(ctx, "c", "attempts", 3); !found || removed {
				t.Fatalf("failure %d: removed=%v found=%v", i, removed, found)
			}
		}
		if f, _ := s.Fields(ctx, "c"); f["attempts"] != "2" || f["user"] != "u" {
			t.Fatalf("after two failures: %v", f)
		}
		if removed, found, _ := s.CountFailure(ctx, "c", "attempts", 3); !found || !removed {
			t.Fatal("the third failure must remove the key")
		}
		if f, _ := s.Fields(ctx, "c"); len(f) != 0 {
			t.Fatalf("still there after the limit: %v", f)
		}
	})
}

func TestMarkOnceCountAndRemaining(t *testing.T) {
	stores(t, func(t *testing.T, s Store, advance func(time.Duration)) {
		ctx := context.Background()
		if first, _ := s.MarkOnce(ctx, "m", time.Minute); !first {
			t.Fatal("the first mark is not first")
		}
		if first, _ := s.MarkOnce(ctx, "m", time.Minute); first {
			t.Fatal("a second mark is first")
		}
		advance(2 * time.Minute)
		if first, _ := s.MarkOnce(ctx, "m", time.Minute); !first {
			t.Fatal("a mark after expiry is not first")
		}

		for want := int64(1); want <= 3; want++ {
			if n, _ := s.Count(ctx, "n", time.Minute); n != want {
				t.Fatalf("Count = %d, want %d", n, want)
			}
		}
		if left, ok, _ := s.Remaining(ctx, "n"); !ok || left <= 0 || left > time.Minute {
			t.Fatalf("Remaining = %v %v", left, ok)
		}
		s.Remove(ctx, "n")
		if _, ok, _ := s.Remaining(ctx, "n"); ok {
			t.Fatal("a removed key has time left")
		}
		if n, _ := s.Count(ctx, "n", time.Minute); n != 1 {
			t.Fatalf("Count after Remove = %d", n)
		}
	})
}

// TestMemoryCopiesWhatItIsGiven: a string that shares a buffer the caller
// reuses — as one read from a request does — is kept as it was when stored.
func TestMemoryCopiesWhatItIsGiven(t *testing.T) {
	ctx := context.Background()
	s := NewMemory(ctx)
	buf := []byte("key-1|value-1")
	key := unsafe.String(&buf[0], 5)
	value := unsafe.String(&buf[6], 7)
	s.SetValue(ctx, key, value, time.Minute)
	s.Put(ctx, "fields", map[string]string{key: value}, time.Minute)
	copy(buf, "XXXXXXXXXXXXX")

	if v, ok, _ := s.Value(ctx, "key-1"); !ok || v != "value-1" {
		t.Fatalf("Value = %q %v — the stored key or value changed with the caller's buffer", v, ok)
	}
	if f, _ := s.Fields(ctx, "fields"); f["key-1"] != "value-1" {
		t.Fatalf("Fields = %v — a stored field changed with the caller's buffer", f)
	}
}
