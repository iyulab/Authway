package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func prefixed(t *testing.T, prefix string) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	UseKeyPrefix(client, prefix)
	return client, mr
}

// TestKeyPrefix_EveryKeyCarriesIt: two deployments that share one Redis are
// kept apart by their prefix, so no command may reach a key without it —
// whichever way the key is written.
func TestKeyPrefix_EveryKeyCarriesIt(t *testing.T) {
	client, mr := prefixed(t, "staging:")
	ctx := context.Background()

	if err := client.Set(ctx, "plain", "1", time.Minute).Err(); err != nil {
		t.Fatalf("SET: %v", err)
	}
	if err := client.Incr(ctx, "counter").Err(); err != nil {
		t.Fatalf("INCR: %v", err)
	}
	if err := client.HSet(ctx, "hash", "field", "value").Err(); err != nil {
		t.Fatalf("HSET: %v", err)
	}
	pipe := client.TxPipeline()
	pipe.HSet(ctx, "in-transaction", "field", "value")
	pipe.Expire(ctx, "in-transaction", time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("MULTI/EXEC: %v", err)
	}
	plain := client.Pipeline()
	plain.Incr(ctx, "in-pipeline")
	plain.Expire(ctx, "in-pipeline", time.Minute)
	if _, err := plain.Exec(ctx); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	script := redis.NewScript(`return redis.call('HINCRBY', KEYS[1], 'n', ARGV[1])`)
	if n, err := script.Run(ctx, client, []string{"by-script"}, 2).Int(); err != nil || n != 2 {
		t.Fatalf("script = %d, %v", n, err)
	}

	keys := mr.Keys()
	if len(keys) != 6 {
		t.Fatalf("keys = %v, want six", keys)
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "staging:") {
			t.Errorf("key %q was written without the prefix", key)
		}
	}
}

// TestKeyPrefix_ReadsFindWhatWasWritten: the prefix is the connection's
// business — callers name keys the way they did before.
func TestKeyPrefix_ReadsFindWhatWasWritten(t *testing.T) {
	client, mr := prefixed(t, "staging:")
	ctx := context.Background()

	client.Set(ctx, "name", "value", time.Minute)
	if got, err := client.Get(ctx, "name").Result(); err != nil || got != "value" {
		t.Fatalf("GET = %q, %v", got, err)
	}
	if ttl, err := client.TTL(ctx, "name").Result(); err != nil || ttl <= 0 {
		t.Fatalf("TTL = %v, %v", ttl, err)
	}
	client.HSet(ctx, "hash", "field", "value")
	if got, err := client.HGetAll(ctx, "hash").Result(); err != nil || got["field"] != "value" {
		t.Fatalf("HGETALL = %v, %v", got, err)
	}
	if n, err := client.Del(ctx, "name", "hash").Result(); err != nil || n != 2 {
		t.Fatalf("DEL = %d, %v", n, err)
	}
	if left := mr.Keys(); len(left) != 0 {
		t.Fatalf("keys left after DEL: %v", left)
	}
}

// TestKeyPrefix_KeepsDeploymentsApart: the same key under two prefixes is two
// keys.
func TestKeyPrefix_KeepsDeploymentsApart(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	open := func(prefix string) *redis.Client {
		c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { c.Close() })
		UseKeyPrefix(c, prefix)
		return c
	}
	ctx := context.Background()
	one, other := open("one:"), open("other:")

	one.Set(ctx, "session", "of one", time.Minute)
	if err := other.Get(ctx, "session").Err(); err != redis.Nil {
		t.Fatalf("the other deployment read the key: %v", err)
	}
	other.Del(ctx, "session")
	if got, _ := one.Get(ctx, "session").Result(); got != "of one" {
		t.Fatalf("the other deployment deleted the key: %q", got)
	}
}

// TestKeyPrefix_RefusesWhatItCannotPlace: a command whose keys the prefix
// does not know how to find would reach the shared keyspace unprefixed, or
// answer with keys of every deployment. It fails instead.
func TestKeyPrefix_RefusesWhatItCannotPlace(t *testing.T) {
	client, _ := prefixed(t, "staging:")
	ctx := context.Background()

	if err := client.Keys(ctx, "*").Err(); err == nil || !strings.Contains(err.Error(), "key prefix") {
		t.Fatalf("KEYS = %v, want a refusal that names the key prefix", err)
	}
	if err := client.FlushDB(ctx).Err(); err == nil {
		t.Fatal("FLUSHDB went through: it would clear every deployment's keys")
	}
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("PING, which has no key, was refused: %v", err)
	}
}

// TestKeyPrefix_NoneLeavesKeysAsTheyAre: a deployment with a Redis of its own
// sets no prefix, and its keys stay where they were.
func TestKeyPrefix_NoneLeavesKeysAsTheyAre(t *testing.T) {
	client, mr := prefixed(t, "")
	ctx := context.Background()

	client.Set(ctx, "name", "value", time.Minute)
	if keys := mr.Keys(); len(keys) != 1 || keys[0] != "name" {
		t.Fatalf("keys = %v, want [name]", keys)
	}
	if err := client.Keys(ctx, "*").Err(); err != nil {
		t.Fatalf("KEYS without a prefix: %v", err)
	}
}

func TestNormalizeKeyPrefix(t *testing.T) {
	for given, want := range map[string]string{
		"":            "",
		"  ":          "",
		"staging":     "staging:",
		"staging:":    "staging:",
		"authway.eu1": "authway.eu1:",
		"a_b-c":       "a_b-c:",
	} {
		got, err := NormalizeKeyPrefix(given)
		if err != nil || got != want {
			t.Errorf("NormalizeKeyPrefix(%q) = %q, %v; want %q", given, got, err, want)
		}
	}
	for _, bad := range []string{"with space", "glob*", "a:b", "[x]", strings.Repeat("x", 40), ":"} {
		if got, err := NormalizeKeyPrefix(bad); err == nil {
			t.Errorf("NormalizeKeyPrefix(%q) = %q, want a refusal", bad, got)
		}
	}
}
