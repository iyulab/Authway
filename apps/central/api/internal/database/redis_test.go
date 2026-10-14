package database

import (
	"testing"

	"authway/apps/central/api/internal/config"
)

// A cluster client dials the nodes at the addresses the cluster announces —
// IPs on a managed Redis — so the certificate has to be checked against the
// configured host, or no node can be reached over TLS.
func TestRedisOptions_TLSChecksTheConfiguredHost(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		opts := redisOptions(config.RedisConfig{Host: "cache.example.net", Port: 10000, TLSEnabled: true, Cluster: cluster})
		if opts.TLSConfig == nil {
			t.Fatalf("cluster=%v: TLS is enabled but the options carry no TLS config", cluster)
		}
		if got := opts.TLSConfig.ServerName; got != "cache.example.net" {
			t.Fatalf("cluster=%v: certificate checked against %q, want the configured host", cluster, got)
		}
	}
}

func TestRedisOptions_NoTLSUnlessEnabled(t *testing.T) {
	if opts := redisOptions(config.RedisConfig{Host: "localhost", Port: 6379}); opts.TLSConfig != nil {
		t.Fatal("TLS config set without TLS enabled")
	}
}
