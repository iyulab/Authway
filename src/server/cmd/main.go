package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"authway/src/server/internal/config"
	"authway/src/server/internal/database"
	"authway/src/server/internal/handler"
	"authway/src/server/internal/hydra"
	"authway/src/server/internal/middleware"
	"authway/src/server/internal/service"
	"authway/src/server/internal/service/social"
	"authway/src/server/internal/telemetry"
	"authway/src/server/pkg/admin"
	"authway/src/server/pkg/client"
	"authway/src/server/pkg/email"
	adminMiddleware "authway/src/server/pkg/middleware"
	"authway/src/server/pkg/tenant"
	"authway/src/server/pkg/user"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

func main() {
	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Initialize logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer zapLogger.Sync()

	// Initialize database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		zapLogger.Fatal("Failed to connect to database", zap.Error(err))
	}

	// NOTE: Database migrations are handled by scripts/migrate.go during startup
	// GORM AutoMigrate is disabled to prevent conflicts with SQL migrations
	// If you need to add new tables, update scripts/migrations/*.sql files

	// Initialize Tenant Service
	tenantService := tenant.NewService(db)

	// Tenant initialization based on mode
	if cfg.Tenant.SingleTenantMode {
		// Single Tenant Mode: Create dedicated tenant
		zapLogger.Info("Starting in Single Tenant Mode",
			zap.String("tenant_name", cfg.Tenant.TenantName),
			zap.String("tenant_slug", cfg.Tenant.TenantSlug))

		if cfg.Tenant.TenantName == "" || cfg.Tenant.TenantSlug == "" {
			zapLogger.Fatal("Single Tenant Mode requires TENANT_NAME and TENANT_SLUG")
		}

		_, err := tenantService.CreateSingleTenant(cfg.Tenant.TenantName, cfg.Tenant.TenantSlug)
		if err != nil {
			zapLogger.Fatal("Failed to create single tenant", zap.Error(err))
		}
	} else {
		// Multi-Tenant Mode: Ensure default tenant exists
		zapLogger.Info("Starting in Multi-Tenant Mode")

		if err := tenantService.EnsureDefaultTenant(); err != nil {
			zapLogger.Fatal("Failed to ensure default tenant", zap.Error(err))
		}
	}

	// Initialize Admin Service
	adminService := admin.NewService(db, zapLogger, cfg.Admin.Password)
	adminHandler := admin.NewHandler(adminService, zapLogger, cfg.App.Version)

	// Initialize Redis
	_, err = database.ConnectRedis(cfg.Redis)
	if err != nil {
		zapLogger.Fatal("Failed to connect to Redis", zap.Error(err))
	}

	// Initialize Hydra client
	hydraClient := hydra.NewClient(cfg.Hydra.AdminURL)

	// Initialize validator
	validate := validator.New()

	// Initialize services
	userService := user.NewService(db, zapLogger)
	clientService := client.NewService(db, zapLogger, hydraClient)
	googleService := social.NewGoogleService(&cfg.Google, userService, clientService, zapLogger)

	// Initialize email services
	emailConfig := email.Config{
		SMTPHost:     cfg.Email.SMTPHost,
		SMTPPort:     fmt.Sprintf("%d", cfg.Email.SMTPPort),
		SMTPUsername: cfg.Email.SMTPUser,
		SMTPPassword: cfg.Email.SMTPPassword,
		FromEmail:    cfg.Email.FromEmail,
		FromName:     cfg.Email.FromName,
		FrontendURL:  cfg.App.BaseURL,
	}
	emailService := email.NewService(emailConfig, zapLogger)
	emailRepo := email.NewRepository(db)

	// Create services struct for handlers
	services := &service.Services{
		UserService:   userService,
		ClientService: clientService,
	}

	// Initialize Application Insights telemetry client (optional)
	telemetryClient := telemetry.NewClient(&cfg.ApplicationInsights, zapLogger)
	defer telemetryClient.Flush()

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(cfg.CORS.AllowedOrigins, ","),
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Admin-API-Key,X-Admin-Token,Request-Id,Traceparent,Tracestate",
		AllowCredentials: true,
	}))
	app.Use(middleware.RequestLogger(zapLogger))
	app.Use(telemetry.RequestTracking(telemetryClient, zapLogger))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "ok",
			"service":   "authway",
			"version":   cfg.App.Version,
			"timestamp": c.Context().Time(),
		})
	})

	// Initialize handlers
	authHandler := handler.NewAuthHandler(userService, clientService, hydraClient, zapLogger)
	socialHandler := handler.NewSocialHandler(googleService, userService, hydraClient, zapLogger)
	clientHandler := handler.NewClientHandler(services, zapLogger)
	emailHandler := handler.NewEmailHandler(emailRepo, emailService, userService, hydraClient, validate, zapLogger)

	// Auth routes for Hydra login/consent flow
	app.Get("/login", authHandler.LoginPage)
	app.Post("/login", authHandler.LoginPage) // Support POST for long login_challenge
	app.Post("/authenticate", authHandler.Login) // Actual login submission
	app.Get("/consent", authHandler.ConsentPage)
	app.Post("/consent", authHandler.ConsentPage) // Support POST for long consent_challenge (from auto-submit form)
	app.Post("/consent/accept", authHandler.Consent) // Actual consent submission
	app.Post("/consent/reject", authHandler.RejectConsent)

	// User registration
	app.Post("/register", authHandler.Register)

	// Social login routes
	app.Get("/auth/google/login", socialHandler.GoogleLogin)
	app.Post("/auth/google/login", socialHandler.GoogleLogin) // Support POST for long login_challenge
	app.Get("/auth/google/callback", socialHandler.GoogleCallback)
	app.Get("/auth/google/url", socialHandler.GetGoogleAuthURL)

	// API routes
	api := app.Group("/api")

	// Email verification and password reset routes
	emailHandler.RegisterRoutes(api)

	// API v1 routes
	v1 := app.Group("/api/v1")

	// User profile routes
	v1.Get("/profile/:id", authHandler.Profile)

	// Client management routes
	v1.Post("/clients", clientHandler.Create)
	v1.Get("/clients/:id", clientHandler.Get)
	v1.Put("/clients/:id", clientHandler.Update)
	v1.Delete("/clients/:id", clientHandler.Delete)
	v1.Get("/clients", clientHandler.List)
	v1.Post("/clients/:id/regenerate-secret", clientHandler.RegenerateSecret)

	// Client Google OAuth configuration routes
	v1.Put("/clients/:id/google-oauth", clientHandler.UpdateGoogleOAuth)
	v1.Delete("/clients/:id/google-oauth", clientHandler.DisableGoogleOAuth)
	v1.Get("/clients/:id/google-oauth/status", clientHandler.GetGoogleOAuthStatus)

	// Tenant Management API routes (Admin only)
	tenantHandler := tenant.NewHandler(tenantService, validate)
	adminAuth := adminMiddleware.AdminAuth(cfg.Admin.APIKey)
	tenantHandler.RegisterRoutes(app, adminAuth)

	// Admin Console routes
	adminHandler.RegisterRoutes(app)

	// Cleanup expired admin sessions periodically
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := adminService.CleanupExpiredSessions(); err != nil {
				zapLogger.Error("Failed to cleanup expired admin sessions", zap.Error(err))
			}
		}
	}()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	zapLogger.Info("Starting Authway server",
		zap.String("port", port),
		zap.String("environment", cfg.App.Environment),
	)

	if err := app.Listen(":" + port); err != nil {
		zapLogger.Fatal("Failed to start server", zap.Error(err))
	}
}
