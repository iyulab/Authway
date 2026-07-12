package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/internal/database"
	"authway/apps/central/api/internal/handler"
	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/internal/middleware"
	"authway/apps/central/api/internal/service"
	"authway/apps/central/api/internal/service/social"
	"authway/apps/central/api/internal/telemetry"
	"authway/apps/central/api/pkg/admin"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/mfa"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
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

	// Initialize database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		zapLogger.Fatal("Failed to connect to database", zap.Error(err))
	}

	// Run database migrations automatically
	zapLogger.Info("Running database migrations")
	if err := database.RunMigrations(db, zapLogger); err != nil {
		zapLogger.Fatal("Failed to run database migrations", zap.Error(err))
	}
	zapLogger.Info("Database migrations completed successfully")

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

	// Initialize Admin Service (Handler is constructed later, once the audit
	// service is available — admin auth failures must be recorded, so the
	// Handler depends on newFeatureServices.AuditService).
	adminService := admin.NewService(db, zapLogger, cfg.Admin.Password)

	// Initialize Redis
	redisClient, err := database.ConnectRedis(cfg.Redis)
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
	githubService := social.NewGitHubService(&cfg.GitHub, userService, clientService, zapLogger)
	microsoftService := social.NewMicrosoftService(&cfg.Microsoft, userService, clientService, zapLogger)
	appleService := social.NewAppleService(&cfg.Apple, userService, clientService, zapLogger)

	// Initialize Claims Service
	claimsRepo := claims.NewRepository(db, redisClient)
	claimsService := claims.NewService(claimsRepo, cfg.Hydra.PublicURL, zapLogger)
	claimsHandler := claims.NewHandler(claimsService, zapLogger)

	// Initialize email services
	var emailService email.EmailService

	if cfg.Email.UseAzure {
		// Use Azure Functions Email Service (Production)
		zapLogger.Info("Using Azure Functions Email Service",
			zap.String("baseURL", cfg.Email.AzureBaseURL))

		azureConfig := email.AzureEmailConfig{
			BaseURL:     cfg.Email.AzureBaseURL,
			FunctionKey: cfg.Email.AzureFunctionKey,
			ProfileID:   cfg.Email.AzureProfile,
			FromEmail:   cfg.Email.FromEmail,
			FromName:    cfg.Email.FromName,
			FrontendURL: cfg.App.BaseURL,
		}
		emailService = email.NewAzureEmailService(azureConfig, zapLogger)
	} else {
		// Use traditional SMTP (Development/Fallback)
		zapLogger.Info("Using traditional SMTP Email Service",
			zap.String("smtpHost", cfg.Email.SMTPHost))

		smtpConfig := email.Config{
			SMTPHost:     cfg.Email.SMTPHost,
			SMTPPort:     fmt.Sprintf("%d", cfg.Email.SMTPPort),
			SMTPUsername: cfg.Email.SMTPUser,
			SMTPPassword: cfg.Email.SMTPPassword,
			FromEmail:    cfg.Email.FromEmail,
			FromName:     cfg.Email.FromName,
			FrontendURL:  cfg.App.BaseURL,
		}
		emailService = email.NewService(smtpConfig, zapLogger)
	}

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

	// Public configuration endpoint for OIDC discovery
	// Clients can use this to discover the Hydra (OIDC Authority) URL
	app.Get("/api/v1/config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"issuer":      cfg.Hydra.PublicURL,
			"auth_server": cfg.Hydra.PublicURL,
			"api_server":  cfg.App.BaseURL,
			"version":     cfg.App.Version,
		})
	})

	// Feature services (audit, webhooks, invitations, ...) are initialized
	// early so the audit.Service is available to wire into write-path handlers
	// below. Route registration still happens later once jwtAuth/adminAuth are
	// constructed.
	newFeatureServices := InitNewFeatureServices(db, zapLogger, userService, tenantService, emailService, cfg.App.BaseURL)

	// Admin handler depends on audit.Service so auth failures surface in
	// audit_logs (see pkg/admin/handler.go logAuthFailure).
	adminHandler := admin.NewHandler(adminService, zapLogger, cfg.App.Version, cfg.Admin.APIKey, newFeatureServices.AuditService)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(userService, clientService, claimsService, hydraClient, zapLogger, newFeatureServices.AuditService)
	socialHandler := handler.NewSocialHandlerWithAllProviders(googleService, githubService, microsoftService, appleService, userService, hydraClient, zapLogger, newFeatureServices.AuditService)
	clientHandler := handler.NewClientHandler(services, zapLogger, cfg, newFeatureServices.AuditService)
	emailHandler := handler.NewEmailHandler(emailRepo, emailService, userService, hydraClient, validate, zapLogger, newFeatureServices.AuditService)
	docsHandler := handler.NewDocsHandler(zapLogger)
	internalAuthHandler := handler.NewInternalAuthHandler(userService, clientService, zapLogger, newFeatureServices.AuditService)
	userHandler := handler.NewUserHandler(services, zapLogger, newFeatureServices.AuditService)

	// Initialize MFA Service and Handler
	mfaService := mfa.NewService(db, userService, zapLogger, cfg.App.Name)
	mfaHandler := handler.NewMFAHandler(mfaService, userService, zapLogger, newFeatureServices.AuditService)

	// Auth routes for Hydra login/consent flow
	app.Get("/login", authHandler.LoginPage)
	app.Post("/login", authHandler.LoginPage)    // Support POST for long login_challenge
	app.Post("/authenticate", authHandler.Login) // Actual login submission
	app.Get("/consent", authHandler.ConsentPage)
	app.Post("/consent", authHandler.ConsentPage)    // Support POST for long consent_challenge (from auto-submit form)
	app.Post("/consent/accept", authHandler.Consent) // Actual consent submission
	app.Post("/consent/reject", authHandler.RejectConsent)
	app.Get("/logout", authHandler.LogoutPage) // Logout flow (auto-accept)

	// Popup callback for popup-based authentication (@authway/client, @authway/react)
	app.Get("/oauth/popup-callback", authHandler.PopupCallback)

	// User registration
	app.Post("/register", authHandler.Register)

	// Social login routes - Google
	app.Get("/auth/google/login", socialHandler.GoogleLogin)
	app.Post("/auth/google/login", socialHandler.GoogleLogin) // Support POST for long login_challenge
	app.Get("/auth/google/callback", socialHandler.GoogleCallback)
	app.Get("/auth/google/url", socialHandler.GetGoogleAuthURL)

	// Social login routes - GitHub
	app.Get("/auth/github/login", socialHandler.GitHubLogin)
	app.Post("/auth/github/login", socialHandler.GitHubLogin)
	app.Get("/auth/github/callback", socialHandler.GitHubCallback)

	// Social login routes - Microsoft
	app.Get("/auth/microsoft/login", socialHandler.MicrosoftLogin)
	app.Post("/auth/microsoft/login", socialHandler.MicrosoftLogin)
	app.Get("/auth/microsoft/callback", socialHandler.MicrosoftCallback)

	// Social login routes - Apple
	app.Get("/auth/apple/login", socialHandler.AppleLogin)
	app.Post("/auth/apple/login", socialHandler.AppleLogin)
	app.Post("/auth/apple/callback", socialHandler.AppleCallback) // Apple uses POST with form_post response mode
	app.Get("/auth/apple/callback", socialHandler.AppleCallback)  // Also support GET

	// API routes
	api := app.Group("/api")

	// Email verification and password reset routes
	emailHandler.RegisterRoutes(api)

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Internal API routes (authenticated with X-API-Key)
	internalAPI := app.Group("/internal")
	internalAuth := middleware.InternalAPIAuth(cfg.Admin.InternalAPIKey, zapLogger)
	internalAPI.Post("/auth/google", internalAuth, internalAuthHandler.AuthenticateGoogleUser)

	// Internal client lookup by client_id (for Auth API logout handler)
	v1.Get("/clients/by-client-id/:client_id", internalAuth, clientHandler.GetByClientID)

	// JWT middleware for authenticated routes (supports both JWT and opaque tokens)
	jwtAuth := middleware.JWTAuth(zapLogger, hydraClient, db)

	// User profile routes
	// IMPORTANT: /profile/me must come BEFORE /profile/:id to match correctly
	v1.Get("/profile/me", jwtAuth, authHandler.ProfileMe)
	// /profile/:id leaks email/name by UUID — require JWT to prevent PII enumeration.
	v1.Get("/profile/:id", jwtAuth, authHandler.Profile)

	// Logout route - direct session revocation
	v1.Post("/logout", authHandler.Logout)

	// Public client configuration (no auth required)
	v1.Get("/clients/:client_id/config", clientHandler.GetPublicConfig)

	// Admin auth — accepts EITHER the long-lived AUTHWAY_ADMIN_API_KEY (for
	// programmatic callers like CI/curl) or an admin session token issued by
	// /admin/login (used by the Admin Console UI). Fail-closed on missing key.
	adminAuth := adminHandler.GetAdminConsoleAuth()

	// Client management routes (admin-only — config changes affect OAuth security posture)
	v1.Post("/clients", adminAuth, clientHandler.Create)
	v1.Get("/clients/:id", adminAuth, clientHandler.Get)
	v1.Put("/clients/:id", adminAuth, clientHandler.Update)
	v1.Delete("/clients/:id", adminAuth, clientHandler.Delete)
	v1.Get("/clients", adminAuth, clientHandler.List)
	v1.Post("/clients/:id/regenerate-secret", adminAuth, clientHandler.RegenerateSecret)

	// Client Google OAuth configuration routes (admin-only)
	v1.Put("/clients/:id/google-oauth", adminAuth, clientHandler.UpdateGoogleOAuth)
	v1.Delete("/clients/:id/google-oauth", adminAuth, clientHandler.DisableGoogleOAuth)
	v1.Get("/clients/:id/google-oauth/status", adminAuth, clientHandler.GetGoogleOAuthStatus)

	// Client Hydra sync (admin operation for one-time migration)
	v1.Post("/clients/sync-hydra", adminAuth, clientHandler.SyncToHydra)

	// System claims management routes (require re-authentication)
	v1.Post("/claims/update", jwtAuth, claimsHandler.HandleUpdateClaims) // Legacy endpoint
	v1.Patch("/claims", jwtAuth, claimsHandler.HandleUpdateClaims)       // RESTful endpoint
	v1.Get("/claims", jwtAuth, claimsHandler.HandleGetClaims)
	v1.Delete("/claims/:claim_key", jwtAuth, claimsHandler.HandleDeleteClaim)

	// User claims management routes (no re-authentication required)
	v1.Patch("/claims/user", jwtAuth, claimsHandler.HandleUpdateUserClaims)
	v1.Get("/claims/user", jwtAuth, claimsHandler.HandleGetUserClaims)

	// Documentation API routes (public read, admin write)
	v1.Get("/docs", docsHandler.ListDocs)
	v1.Get("/docs/search", docsHandler.SearchDocs)
	v1.Get("/docs/download/*", docsHandler.DownloadDoc)
	v1.Get("/docs/*", docsHandler.GetDoc)
	// Admin only docs operations
	v1.Put("/docs/*", adminAuth, docsHandler.UpdateDoc)
	v1.Delete("/docs/*", adminAuth, docsHandler.DeleteDoc)
	v1.Post("/docs/upload", adminAuth, docsHandler.UploadDoc)

	// User management routes (Admin only)
	v1.Get("/users", adminAuth, userHandler.List)
	v1.Get("/users/:id", adminAuth, userHandler.Get)
	v1.Put("/users/:id", adminAuth, userHandler.Update)
	v1.Delete("/users/:id", adminAuth, userHandler.Delete)

	// MFA management routes (authenticated users)
	v1.Post("/users/mfa/setup", jwtAuth, mfaHandler.SetupMFA)
	v1.Post("/users/mfa/verify", jwtAuth, mfaHandler.VerifyMFA)
	v1.Delete("/users/mfa", jwtAuth, mfaHandler.DisableMFA)
	v1.Get("/users/mfa/status", jwtAuth, mfaHandler.GetMFAStatus)
	v1.Post("/users/mfa/recovery", jwtAuth, mfaHandler.VerifyRecoveryCode)
	v1.Post("/users/mfa/recovery/regenerate", jwtAuth, mfaHandler.RegenerateRecoveryCodes)

	// Tenant Management API routes (Admin only)
	tenantHandler := tenant.NewHandler(tenantService, validate, newFeatureServices.AuditService)
	tenantHandler.RegisterRoutes(app, adminAuth)

	// Admin Console routes
	adminHandler.RegisterRoutes(app)

	// ======================================
	// Register New Feature Services routes (Phase 1.7)
	// Services were constructed earlier so audit.Service could be injected
	// into ClientHandler; route registration waits for jwtAuth/adminAuth.
	// ======================================
	// adminAuth (declared above) already accepts session tokens and the API key.
	newFeatureServices.RegisterRoutes(v1, jwtAuth, adminAuth)
	newFeatureServices.StartBackgroundCleanupTasks(zapLogger)

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
