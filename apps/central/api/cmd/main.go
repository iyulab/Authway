package main

import (
	"fmt"
	"log"
	"os"
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
	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/mfa"
	ratelimitmw "authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
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

	// Initialize the at-rest cipher for TOTP secrets. Fail-closed: a malformed
	// key aborts startup. An empty key is permitted only in non-production (the
	// config validator already rejects an empty key in production).
	totpCipher, err := crypto.NewCipher(cfg.Security.TOTPEncryptionKey)
	if err != nil {
		zapLogger.Fatal("Failed to initialize TOTP cipher", zap.Error(err))
	}
	if !totpCipher.Enabled() {
		zapLogger.Warn("TOTP encryption disabled — no AUTHWAY_TOTP_ENCRYPTION_KEY set (secrets stored as plaintext; development only)")
	}

	// Initialize database
	db, err := database.Connect(cfg.Database, cfg.App.Environment)
	if err != nil {
		zapLogger.Fatal("Failed to connect to database", zap.Error(err))
	}

	// Run database migrations automatically
	zapLogger.Info("Running database migrations")
	if err := database.RunMigrations(db, zapLogger); err != nil {
		zapLogger.Fatal("Failed to run database migrations", zap.Error(err))
	}
	zapLogger.Info("Database migrations completed successfully")

	// Backfill: encrypt any legacy plaintext TOTP secrets. Idempotent and a
	// no-op when no key is configured. Non-fatal by design — the lazy
	// pass-through in Decrypt keeps validation working even if this never runs,
	// so a transient DB error here must not block the whole IdP from starting.
	if err := mfa.BackfillTOTPSecrets(db, totpCipher, zapLogger); err != nil {
		zapLogger.Error("TOTP secret backfill failed (non-fatal; validation unaffected)", zap.Error(err))
	}

	// Backfill: hash any legacy plaintext invitation tokens left over from
	// before migration 020 (SQL alone could not hash them — see the
	// migration file). Idempotent and non-fatal — a transient DB error here
	// must not block the whole IdP from starting; it retries on next boot.
	if err := invitation.BackfillTokenHashes(db, zapLogger); err != nil {
		zapLogger.Error("Invitation token backfill failed (non-fatal; retried on next boot)", zap.Error(err))
	}

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
	// Social sign-in may only create an account for an invited address
	// (invitation-only onboarding). The gate is a read-only view of the
	// invitations table, so it exists well before the invitation service does.
	invitationGate := invitation.NewGate(db)
	googleService := social.NewGoogleService(&cfg.Google, userService, invitationGate, clientService, zapLogger)
	githubService := social.NewGitHubService(&cfg.GitHub, userService, invitationGate, clientService, zapLogger)
	microsoftService := social.NewMicrosoftService(&cfg.Microsoft, userService, invitationGate, clientService, zapLogger)
	appleService := social.NewAppleService(&cfg.Apple, userService, invitationGate, clientService, zapLogger)

	// Initialize Claims Service
	claimsRepo := claims.NewRepository(db, redisClient)
	claimsService := claims.NewService(claimsRepo, cfg.Hydra.PublicURL, zapLogger)
	claimsHandler := claims.NewHandler(claimsService, zapLogger)

	// Initialize email services
	var emailService email.EmailService

	if cfg.Email.UseSendway {
		// Use Sendway (Production)
		zapLogger.Info("Using Sendway Email Service",
			zap.String("baseURL", cfg.Email.SendwayBaseURL))

		sendwayConfig := email.SendwayEmailConfig{
			BaseURL:     cfg.Email.SendwayBaseURL,
			APIKey:      cfg.Email.SendwayAPIKey,
			FrontendURL: cfg.App.FrontendURL,
		}
		emailService = email.NewSendwayEmailService(sendwayConfig, zapLogger)
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
			FrontendURL:  cfg.App.FrontendURL,
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
		// Login, consent and logout flow ids are Hydra challenges of 1-2 KB and
		// travel in URLs. Fiber's 4 KB default for the request line plus headers
		// leaves too little room once cookies are added.
		ReadBufferSize: 16 * 1024,
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(middleware.CORS(cfg.CORS.AllowedOrigins))
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
	// Bootstrap document for the Authway SDK: where the OAuth server is and
	// which backend serves the login screens and Authway's own APIs.
	app.Get("/.well-known/authway-config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"oauth_url": cfg.Hydra.PublicURL,
			"issuer":    cfg.Hydra.PublicURL,
			"api_url":   cfg.App.BaseURL,
			"version":   cfg.App.Version,
		})
	})

	app.Get("/api/v1/config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"issuer":      cfg.Hydra.PublicURL,
			"auth_server": cfg.Hydra.PublicURL,
			"api_server":  cfg.App.BaseURL,
			// The auth UI's public address. Advertised so consumers can link to
			// it, and so the deploy gate can read back the value this instance
			// actually got — a wrong one here means every emailed link 404s.
			"auth_ui": cfg.App.FrontendURL,
			"version": cfg.App.Version,
		})
	})

	// Feature services (audit, webhooks, invitations, ...) are initialized
	// early so the audit.Service is available to wire into write-path handlers
	// below. Route registration still happens later once jwtAuth/adminAuth are
	// constructed.
	newFeatureServices := InitNewFeatureServices(db, zapLogger, userService, tenantService, emailService, cfg.App.FrontendURL, cfg.Security.WebhookAllowPrivateTargets)

	serviceClientService := serviceclient.NewService(db, zapLogger, hydraClient)
	serviceClientHandler := handler.NewServiceClientHandler(serviceClientService, zapLogger, newFeatureServices.AuditService)

	// Admin handler depends on audit.Service so auth failures surface in
	// audit_logs (see pkg/admin/handler.go logAuthFailure).
	adminHandler := admin.NewHandler(adminService, zapLogger, cfg.App.Version, cfg.Admin.APIKey, newFeatureServices.AuditService)

	// Initialize MFA Service — constructed before authHandler because Login()
	// now needs it to gate acceptance on TOTPEnabled.
	mfaService := mfa.NewService(db, userService, zapLogger, cfg.App.Name, totpCipher)

	// Initialize handlers
	socialHandler := handler.NewSocialHandlerWithAllProviders(googleService, githubService, microsoftService, appleService, userService, clientService, hydraClient, zapLogger, newFeatureServices.AuditService, handler.NewOAuthStateStore(redisClient), cfg.App.FrontendURL)
	authHandler := handler.NewAuthHandler(userService, clientService, claimsService, mfaService, hydraClient, zapLogger, newFeatureServices.AuditService, redisClient, socialHandler)
	clientHandler := handler.NewClientHandler(services, zapLogger, cfg, newFeatureServices.AuditService)
	emailHandler := handler.NewEmailHandler(emailRepo, emailService, userService, clientService, hydraClient, validate, zapLogger, newFeatureServices.AuditService)
	docsHandler := handler.NewDocsHandler(zapLogger)
	logoutFlowHandler := handler.NewLogoutFlowHandler(hydraClient, zapLogger)
	userHandler := handler.NewUserHandler(services, zapLogger, newFeatureServices.AuditService)
	mfaHandler := handler.NewMFAHandler(mfaService, userService, zapLogger, newFeatureServices.AuditService)

	// The authorization server sends the browser here to start a login; it is
	// handed on to the login UI with an opaque flow id.
	flowEntryHandler := handler.NewFlowEntryHandler(cfg.App.FrontendURL)
	loginRateLimit := ratelimitmw.LoginRateLimit(redisClient)
	app.Get("/login", flowEntryHandler.Login)
	app.Get("/consent", flowEntryHandler.Consent)
	app.Get("/logout", flowEntryHandler.Logout)

	// Popup callback for popup-based authentication (@authway/client, @authway/react)
	app.Get("/oauth/popup-callback", authHandler.PopupCallback)

	// Onboarding is invitation-only — public self-registration removed (D-a/B).
	// Users are created by accepting an invitation (POST
	// /api/v1/invitations/accept). There is deliberately no admin "create user"
	// endpoint: an admin onboards someone by issuing an invitation with the
	// admin API key (system-actor invite, NULL inviter_id — migration 016).
	// First-time social and magic-link sign-ins pass the same gate
	// (invitation.Gate.MayProvision), which a tenant can open with
	// signup_mode=open.

	// Social provider callbacks. Sign-in starts at
	// /api/v1/login-flows/:flow/social/:provider.
	app.Get("/auth/google/callback", socialHandler.GoogleCallback)
	app.Get("/auth/github/callback", socialHandler.GitHubCallback)
	app.Get("/auth/microsoft/callback", socialHandler.MicrosoftCallback)
	app.Post("/auth/apple/callback", socialHandler.AppleCallback) // Apple uses POST with form_post response mode
	app.Get("/auth/apple/callback", socialHandler.AppleCallback)

	// API routes
	api := app.Group("/api")

	// Email verification and password reset routes
	emailHandler.RegisterRoutes(api)

	// API v1 routes
	v1 := app.Group("/api/v1")

	// What this deployment offers, so screens show only what works.
	v1.Get("/capabilities", handler.NewCapabilitiesHandler(socialHandler).Get)

	// What the sign-in screen should do with a login flow: show the form (and
	// which sign-in methods the client allows) or redirect.
	v1.Get("/login-flows/:flow", authHandler.GetLoginFlow)
	v1.Post("/login-flows/:flow/password", loginRateLimit, authHandler.SubmitPassword)
	v1.Post("/login-flows/:flow/mfa", loginRateLimit, authHandler.VerifyMFALogin)
	v1.Post("/login-flows/:flow/mfa/recovery", loginRateLimit, authHandler.VerifyMFARecoveryLogin)
	// A page navigation (not fetch): the browser goes on to the provider.
	v1.Get("/login-flows/:flow/social/:provider", socialHandler.StartSocialLogin)

	// Emailed sign-in links: sent from the login screen, redeemed from the
	// link's landing page in the same browser.
	magicLinkHandler := handler.NewMagicLinkFlowHandler(authHandler, newFeatureServices.PasswordlessService)
	v1.Post("/login-flows/:flow/magic-link", ratelimitmw.MagicLinkRateLimit(redisClient), magicLinkHandler.Send)
	v1.Post("/magic-links/inspect", magicLinkHandler.Inspect)
	v1.Post("/magic-links/redeem", loginRateLimit, magicLinkHandler.Redeem)

	// Consent and logout screens: what to ask, and the user's answer.
	v1.Get("/consent-flows/:flow", authHandler.GetConsentFlow)
	v1.Post("/consent-flows/:flow/accept", authHandler.AcceptConsent)
	v1.Post("/consent-flows/:flow/reject", authHandler.RejectConsent)
	v1.Post("/logout-flows/:flow", logoutFlowHandler.CompleteLogout)

	// JWT middleware for authenticated routes (supports both JWT and opaque tokens)
	jwtAuth := middleware.JWTAuth(zapLogger, hydraClient, db)

	// The signed-in user's own profile. There is no lookup by id: any signed-in
	// user could read another's email and name with it, whatever their tenant.
	v1.Get("/profile/me", jwtAuth, authHandler.ProfileMe)

	// Logout route - direct session revocation.
	// Requires a valid bearer token: the subject to revoke is taken from the
	// validated token, so a caller can only revoke their own sessions.
	v1.Post("/logout", jwtAuth, authHandler.Logout)

	// Public client configuration (no auth required)
	v1.Get("/clients/:client_id/config", clientHandler.GetPublicConfig)

	// Admin auth — accepts EITHER the long-lived AUTHWAY_ADMIN_API_KEY (for
	// programmatic callers like CI/curl) or an admin session token issued by
	// /admin/login (used by the Admin Console UI). Fail-closed on missing key.
	adminAuth := adminHandler.GetAdminConsoleAuth()

	// Accepts either full admin auth or a scoped service_client credential —
	// see admin.Handler.GetClientAuth and ClientHandler.createScoped.
	clientCreateAuth := adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.clients:write")

	// Client management routes (admin-only — config changes affect OAuth security posture)
	v1.Post("/clients", clientCreateAuth, clientHandler.Create)
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

	// Service client provisioning — admin-only, since
	// this mints new M2M credentials. The credentials it mints are what
	// clientCreateAuth (above) accepts on POST /clients.
	v1.Post("/tenants/:id/service-clients", adminAuth, serviceClientHandler.Create)
	v1.Get("/tenants/:id/service-clients", adminAuth, serviceClientHandler.List)
	v1.Delete("/tenants/:id/service-clients/:service_client_id", adminAuth, serviceClientHandler.Revoke)

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
