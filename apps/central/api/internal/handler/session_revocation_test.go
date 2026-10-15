package handler

import (
	"errors"
	"fmt"
	"testing"

	"authway/apps/central/api/internal/hydra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Sessions that ended without every application being told are a warning;
// sessions that could not be ended are an error. Nothing is logged when the
// sessions ended and everyone was told.
func TestLogBestEffortRevocationLevels(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want []zapcore.Level
	}{
		{"ended and told", nil, nil},
		{"ended, not every application told", fmt.Errorf("%w: timeout", hydra.ErrApplicationsNotNotified), []zapcore.Level{zapcore.WarnLevel}},
		{"not ended", errors.New("failed to revoke login sessions: 503"), []zapcore.Level{zapcore.ErrorLevel}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			logBestEffortRevocation(zap.New(core), "Signing out", "user-1", tt.err)
			var got []zapcore.Level
			for _, e := range logs.All() {
				got = append(got, e.Level)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("levels %v, want %v", got, tt.want)
			}
		})
	}
}
