package database

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"authway/apps/central/api/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect opens the pool. appEnv selects the SQL logging policy — see gormLogger.
func Connect(cfg config.DatabaseConfig, appEnv string) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger(appEnv),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// gormLogger keeps bound parameters out of the logs everywhere but development.
//
// This used to be an unconditional logger.Info, which inlines bound values into
// every statement it prints. That put live secrets on the container console —
// invitation tokens (which grant account creation) and magic-link tokens (which
// are a login factor) were both read straight out of staging logs. Log retention
// outlives the tokens, so the exposure is not momentary.
//
// Dropping to Warn is not sufficient on its own: GORM still prints the full SQL
// for slow queries (Warn) and for errors (Error). ParameterizedQueries is what
// actually redacts the values, so both are set together.
func gormLogger(appEnv string) logger.Interface {
	return newGormLogger(appEnv, os.Stdout)
}

func newGormLogger(appEnv string, w io.Writer) logger.Interface {
	cfg := logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	}
	if appEnv == "development" {
		cfg.LogLevel = logger.Info
		cfg.IgnoreRecordNotFoundError = false
		cfg.ParameterizedQueries = false
		cfg.Colorful = true
	}
	return logger.New(log.New(w, "\r\n", log.LstdFlags), cfg)
}
