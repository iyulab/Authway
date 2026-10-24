package ephemeral

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisStore struct {
	client redis.UniversalClient
}

// NewRedis is a Store on client, shared by every instance that reaches it.
func NewRedis(client redis.UniversalClient) Store {
	return &redisStore{client: client}
}

func (s *redisStore) SetValue(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisStore) Value(ctx context.Context, key string) (string, bool, error) {
	v, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *redisStore) Put(ctx context.Context, key string, fields map[string]string, ttl time.Duration) error {
	values := make(map[string]any, len(fields))
	for k, v := range fields {
		values[k] = v
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) Fields(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

func (s *redisStore) Take(ctx context.Context, key string) (map[string]string, error) {
	pipe := s.client.TxPipeline()
	get := pipe.HGetAll(ctx, key)
	pipe.Del(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	return get.Val(), nil
}

// countFailureScript checks that the key exists before HINCRBY, which would
// otherwise bring an expired key back as a one-field hash with no expiry.
var countFailureScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return -1
end
local n = redis.call('HINCRBY', KEYS[1], ARGV[1], 1)
if n >= tonumber(ARGV[2]) then
	redis.call('DEL', KEYS[1])
	return 1
end
return 0
`)

func (s *redisStore) CountFailure(ctx context.Context, key, field string, limit int) (bool, bool, error) {
	n, err := countFailureScript.Run(ctx, s.client, []string{key}, field, limit).Int()
	if err != nil {
		return false, false, err
	}
	return n == 1, n != -1, nil
}

func (s *redisStore) MarkOnce(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, 1, ttl).Result()
}

func (s *redisStore) Count(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := s.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (s *redisStore) Remaining(ctx context.Context, key string) (time.Duration, bool, error) {
	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, false, err
	}
	// -2: no such key. -1: no expiry, which no key here is written without.
	if ttl < 0 {
		return 0, ttl == -1, nil
	}
	return ttl, true, nil
}

func (s *redisStore) Remove(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}
