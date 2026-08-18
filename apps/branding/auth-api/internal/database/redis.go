package database

import (
	"context"
	"crypto/tls"
	"fmt"

	"authway/apps/branding/auth-api/internal/config"
	"github.com/redis/go-redis/v9"
)

// ConnectRedis mirrors apps/central/api/internal/database/redis.go — same
// shared Redis instance, same connection shape, kept identical across the
// two modules since there is nothing module-specific about how to open the
// connection.
func ConnectRedis(cfg config.RedisConfig) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}
