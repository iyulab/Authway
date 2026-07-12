package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"authway/apps/branding/auth-api/internal/config"
	"authway/apps/branding/auth-api/internal/handler"
	"authway/apps/branding/auth-api/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	var zapLogger *zap.Logger
	if cfg.App.Environment == "production" {
		zapLogger, err = zap.NewProduction()
	} else {
		zapLogger, err = zap.NewDevelopment()
	}
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer zapLogger.Sync()

	zapLogger.Info("Starting Auth Backend",
		zap.String("environment", cfg.App.Environment),
		zap.String("port", cfg.App.Port))

	// Initialize services
	googleService := service.NewGoogleService(&cfg.Google, zapLogger)
	centralAPI := service.NewCentralAPIClient(&cfg.CentralAPI, zapLogger)
	hydraClient := service.NewHydraClient(&cfg.Hydra, zapLogger)

	// Initialize handlers
	oauthHandler := handler.NewOAuthHandler(googleService, centralAPI, hydraClient, zapLogger)
	healthHandler := handler.NewHealthHandler()
	consentHandler := handler.NewConsentHandler(cfg.CentralAPI.BaseURL, cfg.CentralAPI.InternalKey, zapLogger)
	claimsHandler := handler.NewClaimsHandler(cfg.CentralAPI.BaseURL, cfg.CentralAPI.InternalKey, zapLogger)
	profileHandler := handler.NewProfileHandler(cfg.CentralAPI.BaseURL, cfg.CentralAPI.InternalKey, zapLogger)
	logoutHandler := handler.NewLogoutHandler(cfg.CentralAPI.BaseURL, cfg.CentralAPI.InternalKey, cfg.Hydra.AdminURL, zapLogger)
	authHandler := handler.NewAuthHandler(cfg.CentralAPI.BaseURL, cfg.CentralAPI.InternalKey, zapLogger)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:         "Auth Backend",
		ErrorHandler:    errorHandler(zapLogger),
		BodyLimit:       10 * 1024 * 1024, // 10MB body limit
		ReadBufferSize:  16384,            // 16KB read buffer
		WriteBufferSize: 16384,            // 16KB write buffer
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))

	// CORS configuration - Dynamic origin with credentials support
	// In production, this should be restricted to known domains
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			// In development, allow all localhost origins
			// In production, check against whitelist
			return true
		},
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
	}))

	// Health check endpoint
	app.Get("/health", healthHandler.Health)
	
	// Config endpoint for clients
	app.Get("/.well-known/authway-config", healthHandler.Config)

	// Generic login endpoint - redirects to Login UI
	app.Get("/login", func(c *fiber.Ctx) error {
		loginChallenge := c.Query("login_challenge")
		if loginChallenge == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "login_challenge query parameter is required",
			})
		}

		// Redirect to Login UI with login_challenge
		loginUIURL := fmt.Sprintf("%s/login?login_challenge=%s", cfg.App.LoginUIURL, loginChallenge)
		return c.Redirect(loginUIURL, fiber.StatusFound)
	})

	// OAuth endpoints
	app.Post("/auth/google/login", oauthHandler.GoogleLogin)
	app.Get("/auth/google/login", oauthHandler.GoogleLoginGet)
	app.Get("/auth/google/callback", oauthHandler.GoogleCallback)

	// Auth endpoints (proxy to Central API)
	// /register removed — onboarding is invitation-only (D-a/B).
	app.Post("/authenticate", authHandler.Authenticate)

	// Consent endpoints (proxy to Central API)
	app.Post("/consent", consentHandler.GetConsentInfo)
	app.Post("/consent/accept", consentHandler.AcceptConsent)
	app.Post("/consent/reject", consentHandler.RejectConsent)

	// Logout endpoint
	app.Get("/logout", logoutHandler.HandleLogout)

	// Claims endpoints (proxy to Central API)
	app.Get("/api/v1/claims", claimsHandler.GetClaims)
	app.Patch("/api/v1/claims", claimsHandler.UpdateClaims)
	app.Get("/api/v1/claims/user", claimsHandler.GetUserClaims)
	app.Patch("/api/v1/claims/user", claimsHandler.UpdateUserClaims)

	// Profile endpoints (proxy to Central API)
	app.Get("/api/v1/profile/me", profileHandler.GetProfile)

	// Static file serving for Auth UI
	app.Static("/", cfg.App.StaticPath)

	// Graceful shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		zapLogger.Info("Shutting down server...")
		if err := app.Shutdown(); err != nil {
			zapLogger.Error("Server shutdown error", zap.Error(err))
		}
	}()

	// Start server
	addr := fmt.Sprintf(":%s", cfg.App.Port)
	zapLogger.Info("Server listening", zap.String("address", addr))

	if err := app.Listen(addr); err != nil {
		zapLogger.Fatal("Server error", zap.Error(err))
	}
}

// errorHandler handles Fiber errors
func errorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		logger.Error("Request error",
			zap.Int("status", code),
			zap.String("path", c.Path()),
			zap.Error(err))

		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
}
