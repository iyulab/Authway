package database

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// A live invitation token was read out of staging container logs because the
// GORM logger inlined bound parameters. These tests pin the two paths that kept
// printing SQL even after the log level was lowered: slow queries and errors.
//
// The token below has the shape of the real one (base64-ish, from the incident);
// any of it reaching the output means a secret would reach the log pipeline.
const secretToken = "n_l463qvSbDuD0PLAINTEXTTOKEN="

// traceOutput reproduces how gorm actually logs a statement. The fidelity
// matters: gorm applies ParamsFilter *inside* the closure it hands to Trace
// (callbacks.go:140-145) and only then calls Dialector.Explain. A test that
// passes pre-inlined SQL bypasses redaction entirely and reports a false leak.
func traceOutput(t *testing.T, appEnv string, err error, elapsed time.Duration) string {
	t.Helper()

	var buf bytes.Buffer
	l := newGormLogger(appEnv, &buf)

	// Numbered placeholders, because that is what the postgres dialector builds
	// and what its Explain knows how to substitute.
	sql := `UPDATE "invitations" SET "token"=$1 WHERE "id" = $2`
	vars := []any{secretToken, "d32fb0ac"}
	ctx := context.Background()
	dialector := postgres.Dialector{}

	fc := func() (string, int64) {
		s, v := sql, vars
		if filter, ok := l.(gorm.ParamsFilter); ok {
			s, v = filter.ParamsFilter(ctx, sql, vars...)
		}
		return dialector.Explain(s, v...), 1
	}

	l.Trace(ctx, time.Now().Add(-elapsed), fc, err)
	return buf.String()
}

func TestGormLoggerRedactsParametersOnSlowQuery(t *testing.T) {
	// Warn level still prints slow queries in full — this is the case a bare
	// LogMode(Warn) would have missed.
	for _, env := range []string{"production", "staging", ""} {
		out := traceOutput(t, env, nil, 2*time.Second)
		if out == "" {
			t.Fatalf("env=%q: expected the slow query to be logged at all", env)
		}
		if strings.Contains(out, secretToken) {
			t.Errorf("env=%q: bound parameter leaked into slow-query log:\n%s", env, out)
		}
	}
}

func TestGormLoggerRedactsParametersOnError(t *testing.T) {
	for _, env := range []string{"production", "staging", ""} {
		out := traceOutput(t, env, errors.New("boom"), time.Millisecond)
		if out == "" {
			t.Fatalf("env=%q: expected the failing statement to be logged at all", env)
		}
		if strings.Contains(out, secretToken) {
			t.Errorf("env=%q: bound parameter leaked into error log:\n%s", env, out)
		}
	}
}

func TestGormLoggerStaysQuietOnSuccessfulQueries(t *testing.T) {
	// The original leak was every successful INSERT/UPDATE being logged at Info.
	out := traceOutput(t, "production", nil, time.Millisecond)
	if out != "" {
		t.Errorf("successful fast query should not be logged outside development:\n%s", out)
	}
}

func TestGormLoggerKeepsVerboseSQLInDevelopment(t *testing.T) {
	// Redaction must not cost local debuggability, or it gets reverted. In
	// development the bound values are the point, so they stay in full.
	out := traceOutput(t, "development", nil, time.Millisecond)
	if !strings.Contains(out, secretToken) {
		t.Errorf("development should log the statement with values, got:\n%s", out)
	}
}
