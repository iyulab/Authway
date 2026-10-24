package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"authway/apps/central/api/internal/config"
	"go.uber.org/zap"
)

// version is set at build time via -ldflags "-X main.version=<value>"
// (see Dockerfile). Overrides cfg.App.Version so `/health` reports the actual
// deployed build, not the stale viper default. If left as "dev", config/env
// wins — keeping local dev and explicit overrides functional.
var version = "dev"

func main() {
	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Prefer build-time version over the viper default so deploy drift is
	// observable via /health. Explicit env (APP_VERSION) still wins.
	if version != "dev" && os.Getenv("APP_VERSION") == "" {
		cfg.App.Version = version
	}

	// Initialize logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer zapLogger.Sync()

	srv, err := newServer(cfg, zapLogger)
	if err != nil {
		zapLogger.Fatal("Failed to start", zap.Error(err))
	}
	srv.startBackground()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	zapLogger.Info("Starting Authway server",
		zap.String("port", port),
		zap.String("environment", cfg.App.Environment),
	)

	go func() {
		if err := srv.app.Listen(":" + port); err != nil {
			zapLogger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// On SIGTERM (a new revision taking over) stop taking requests, let the
	// ones in progress finish, and send the mail they handed over.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	zapLogger.Info("Shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	srv.shutdown(ctx)
}
