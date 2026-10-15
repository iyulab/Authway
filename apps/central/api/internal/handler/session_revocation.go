package handler

import (
	"errors"

	"authway/apps/central/api/internal/hydra"
	"go.uber.org/zap"
)

// logBestEffortRevocation records the outcome of ending a user's sessions
// where the request goes on either way. Sessions that ended without every
// application being told are a warning; sessions that could not be ended are
// an error — they still work.
func logBestEffortRevocation(logger *zap.Logger, what, subject string, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, hydra.ErrApplicationsNotNotified):
		logger.Warn(what+": sessions ended, but not every application was told", zap.String("subject", subject), zap.Error(err))
	default:
		logger.Error(what+": sessions could not be ended", zap.String("subject", subject), zap.Error(err))
	}
}
