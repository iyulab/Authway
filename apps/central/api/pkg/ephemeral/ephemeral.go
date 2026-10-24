// Package ephemeral keeps what a sign-in needs for minutes or hours — a
// challenge between two steps, a state value between a redirect and its
// callback, a counter of failed attempts — with an expiry on every key.
//
// Two stores: Redis, which every instance of a deployment shares, and memory,
// which only the one process sees. A deployment that runs more than one
// instance needs Redis: a challenge created on one instance must be found by
// whichever instance serves the next request.
package ephemeral

import (
	"context"
	"time"
)

// Store is a key space where every key expires.
type Store interface {
	// SetValue stores value under key for ttl.
	SetValue(ctx context.Context, key, value string, ttl time.Duration) error
	// Value returns what key holds; ok is false when it holds nothing.
	Value(ctx context.Context, key string) (value string, ok bool, err error)

	// Put stores fields under key for ttl, replacing what it held.
	Put(ctx context.Context, key string, fields map[string]string, ttl time.Duration) error
	// Fields returns the fields key holds; empty when none.
	Fields(ctx context.Context, key string) (map[string]string, error)
	// Take returns the fields key holds and removes them in one step, so two
	// callers never both get them.
	Take(ctx context.Context, key string) (map[string]string, error)
	// CountFailure adds one to the field of key and, once it reaches limit,
	// removes key. found is false when key holds nothing — nothing is
	// created then.
	CountFailure(ctx context.Context, key, field string, limit int) (removed, found bool, err error)

	// MarkOnce records key for ttl; first is false when it was already
	// recorded.
	MarkOnce(ctx context.Context, key string, ttl time.Duration) (first bool, err error)
	// Count adds one to the counter at key, which expires window after the
	// last addition, and returns the new count.
	Count(ctx context.Context, key string, window time.Duration) (int64, error)
	// Remaining returns how long key has left; ok is false when it holds
	// nothing.
	Remaining(ctx context.Context, key string) (left time.Duration, ok bool, err error)

	// Remove removes keys.
	Remove(ctx context.Context, keys ...string) error
}
