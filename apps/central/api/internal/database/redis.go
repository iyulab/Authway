package database

import (
	"context"
	"crypto/tls"
	"fmt"

	"authway/apps/central/api/internal/config"
	"github.com/redis/go-redis/v9"
)

// ConnectRedis connects to a single Redis, or with cfg.Cluster to a Redis
// Cluster through any of its nodes. Either way what uses the client sees the
// same interface; every command it sends names one key, so none of them
// needs its keys to share a cluster slot.
func ConnectRedis(cfg config.RedisConfig) (redis.UniversalClient, error) {
	keyPrefix, err := NormalizeKeyPrefix(cfg.KeyPrefix)
	if err != nil {
		return nil, err
	}
	if cfg.Cluster && cfg.DB != 0 {
		return nil, fmt.Errorf("redis: a cluster has database 0 only, not %d — keep deployments apart with a key prefix (AUTHWAY_REDIS_KEY_PREFIX)", cfg.DB)
	}

	client := redis.NewUniversalClient(redisOptions(cfg))
	UseKeyPrefix(client, keyPrefix)

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}

func redisOptions(cfg config.RedisConfig) *redis.UniversalOptions {
	opts := &redis.UniversalOptions{
		Addrs:         []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Password:      cfg.Password,
		DB:            cfg.DB,
		IsClusterMode: cfg.Cluster,
	}

	// The certificate is checked against the host the deployment names, not
	// the address a node is reached at: a cluster announces its nodes by IP
	// (CLUSTER SLOTS), and a managed Redis's certificate names hosts only.
	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: cfg.Host,
		}
	}
	return opts
}
