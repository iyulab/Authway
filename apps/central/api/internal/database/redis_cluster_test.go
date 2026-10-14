package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"authway/apps/central/api/internal/config"
)

// clusterConfig points at a Redis Cluster for the length of a test, or skips
// it. REDIS_CLUSTER_ADDR is host:port of any node (e.g. a three-node cluster
// on 127.0.0.1:7000-7002).
func clusterConfig(t *testing.T) config.RedisConfig {
	t.Helper()
	addr := os.Getenv("REDIS_CLUSTER_ADDR")
	if addr == "" {
		t.Skip("REDIS_CLUSTER_ADDR not set; skipping Redis Cluster tests")
	}
	host, port, ok := strings.Cut(addr, ":")
	p, err := strconv.Atoi(port)
	if !ok || err != nil {
		t.Fatalf("REDIS_CLUSTER_ADDR %q is not host:port", addr)
	}
	return config.RedisConfig{Host: host, Port: p, Cluster: true}
}

// On a cluster the API reads and writes keys that live on different nodes,
// under its prefix, with the same commands and transactions it uses on a
// single Redis — each names one key, so none needs its keys in one slot.
func TestConnectRedis_ClusterServesEveryCommandUnderThePrefix(t *testing.T) {
	cfg := clusterConfig(t)
	cfg.KeyPrefix = "clustertest-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	client, err := ConnectRedis(cfg)
	if err != nil {
		t.Fatalf("ConnectRedis(cluster): %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	// Enough keys that they land on every node.
	for i := 0; i < 64; i++ {
		key := fmt.Sprintf("k%d", i)
		if err := client.Set(ctx, key, i, time.Minute).Err(); err != nil {
			t.Fatalf("SET %s: %v", key, err)
		}
		if got, err := client.Get(ctx, key).Int(); err != nil || got != i {
			t.Fatalf("GET %s: %d, %v", key, got, err)
		}
	}

	// The transaction shapes the stores use: write a hash and its expiry,
	// then read and delete it at once.
	tx := client.TxPipeline()
	tx.HSet(ctx, "state", map[string]any{"login_challenge": "c", "client_id": "x"})
	tx.Expire(ctx, "state", time.Minute)
	if _, err := tx.Exec(ctx); err != nil {
		t.Fatalf("store transaction: %v", err)
	}
	tx = client.TxPipeline()
	got := tx.HGetAll(ctx, "state")
	tx.Del(ctx, "state")
	if _, err := tx.Exec(ctx); err != nil {
		t.Fatalf("consume transaction: %v", err)
	}
	if got.Val()["client_id"] != "x" {
		t.Fatalf("consumed state %v", got.Val())
	}

	// The counter shape the rate limit uses.
	p := client.Pipeline()
	incr := p.Incr(ctx, "ratelimit:ip")
	p.Expire(ctx, "ratelimit:ip", time.Minute)
	if _, err := p.Exec(ctx); err != nil || incr.Val() != 1 {
		t.Fatalf("rate limit pipeline: %d, %v", incr.Val(), err)
	}

	// The keys are where the prefix says, on the nodes that own them.
	raw := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)}})
	defer raw.Close()
	if n, err := raw.Exists(ctx, cfg.KeyPrefix+":k1").Result(); err != nil || n != 1 {
		t.Fatalf("the prefixed key is not there: %d, %v", n, err)
	}
	if n, _ := raw.Exists(ctx, "k1").Result(); n != 0 {
		t.Fatal("a key was written without the prefix")
	}
	for i := 0; i < 64; i++ {
		raw.Del(ctx, fmt.Sprintf("%s:k%d", cfg.KeyPrefix, i))
	}
	raw.Del(ctx, cfg.KeyPrefix+":ratelimit:ip")
}

// Without the cluster setting a client talks to one node and is told MOVED
// for every key another node owns — the failure the setting exists for.
func TestConnectRedis_ASingleNodeClientCannotUseACluster(t *testing.T) {
	cfg := clusterConfig(t)
	cfg.Cluster = false
	client, err := ConnectRedis(cfg)
	if err != nil {
		t.Fatalf("ConnectRedis(single): %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	moved := 0
	for i := 0; i < 64; i++ {
		err := client.Set(ctx, fmt.Sprintf("single-%d", i), i, time.Minute).Err()
		var redisErr redis.Error
		if errors.As(err, &redisErr) && strings.HasPrefix(err.Error(), "MOVED") {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("expected MOVED for keys other nodes own")
	}
}

func TestConnectRedis_ClusterRefusesADatabaseNumber(t *testing.T) {
	_, err := ConnectRedis(config.RedisConfig{Host: "localhost", Port: 7000, Cluster: true, DB: 1})
	if err == nil || !strings.Contains(err.Error(), "database 0 only") {
		t.Fatalf("expected a cluster with DB 1 to be refused, got %v", err)
	}
}
