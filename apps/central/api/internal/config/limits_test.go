package config

import "testing"

// A deployment keeps the ceiling people run with unless it moves it: the
// conformance suite onboards a user per scenario from one address and raises it.
func TestLoadReadsTheRedemptionCeilingFromTheEnvironment(t *testing.T) {
	t.Setenv("AUTHWAY_APP_ENVIRONMENT", "development")
	t.Setenv("AUTHWAY_ADMIN_PASSWORD", "x")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Limits.RedemptionsPer15Minutes; got != 60 {
		t.Fatalf("default ceiling = %d, want 60", got)
	}

	t.Setenv("AUTHWAY_LIMITS_REDEMPTIONS_PER_15_MINUTES", "1000")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Limits.RedemptionsPer15Minutes; got != 1000 {
		t.Fatalf("ceiling from the environment = %d, want 1000", got)
	}
}

// An app on a database server it shares holds open at most the share it was
// given, and one on a server of its own keeps the default.
func TestLoadReadsTheConnectionCeilingFromTheEnvironment(t *testing.T) {
	t.Setenv("AUTHWAY_APP_ENVIRONMENT", "development")
	t.Setenv("AUTHWAY_ADMIN_PASSWORD", "x")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Database.MaxOpenConns; got != 100 {
		t.Fatalf("default connection ceiling = %d, want 100", got)
	}

	t.Setenv("AUTHWAY_DATABASE_MAX_OPEN_CONNS", "12")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Database.MaxOpenConns; got != 12 {
		t.Fatalf("connection ceiling from the environment = %d, want 12", got)
	}
}
