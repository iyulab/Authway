package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	"authway/apps/central/api/pkg/audit"
	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/console"
	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/delegation"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/mfa"
	ratelimitmw "authway/apps/central/api/pkg/middleware"
	"authway/apps/central/api/pkg/outbound"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/tenantscope"
	"authway/apps/central/api/pkg/terms"
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

// mailSenders is how many messages are handed to the mail service at once.
const mailSenders = 8

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

	// Initialize the at-rest cipher — for TOTP secrets, and for the credentials
	// clients hold at other identity providers. Fail-closed: a malformed
	// key aborts startup. An empty key is permitted only in non-production (the
	// config validator already rejects an empty key in production).
	totpCipher, err := crypto.NewCipher(cfg.Security.TOTPEncryptionKey, cfg.Security.TOTPPreviousKeys...)
	if err != nil {
		zapLogger.Fatal("Failed to initialize TOTP cipher", zap.Error(err))
	}
	if !totpCipher.Enabled() {
		zapLogger.Warn("At-rest encryption disabled — no AUTHWAY_TOTP_ENCRYPTION_KEY set (TOTP secrets and provider credentials stored as plaintext; development only)")
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

	// Store every TOTP secret sealed with the current key (sealing plaintext,
	// resealing what a previous key sealed). Idempotent and a no-op when no
	// key is configured. Non-fatal by design — Decrypt reads plaintext and
	// opens values sealed with any configured key, so a transient DB error
	// here must not block the whole IdP from starting.
	if err := mfa.ResealTOTPSecrets(db, totpCipher, zapLogger); err != nil {
		zapLogger.Error("Resealing TOTP secrets failed (non-fatal; validation unaffected)", zap.Error(err))
	}

	// Backfill: hash any legacy plaintext invitation tokens left over from
	// before migration 020 (SQL alone could not hash them — see the
	// migration file). Idempotent and non-fatal — a transient DB error here
	// must not block the whole IdP from starting; it retries on next boot.
	if err := invitation.BackfillTokenHashes(db, zapLogger); err != nil {
		zapLogger.Error("Invitation token backfill failed (non-fatal; retried on next boot)", zap.Error(err))
	}

	// Backfill: accounts created by accepting an invitation, from before
	// acceptance marked the invited address verified. Idempotent, non-fatal.
	if err := invitation.MarkAcceptedAddressesVerified(db, zapLogger); err != nil {
		zapLogger.Error("Marking invited addresses verified failed (non-fatal; retried on next boot)", zap.Error(err))
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
	// Sign-in state: in Redis, which every instance shares, or — for a
	// deployment of one instance — in this process.
	var signInState ephemeral.Store
	if cfg.Redis.Enabled {
		redisClient, err := database.ConnectRedis(cfg.Redis)
		if err != nil {
			zapLogger.Fatal("Failed to connect to Redis", zap.Error(err))
		}
		if cfg.Redis.KeyPrefix != "" {
			zapLogger.Info("Redis keys carry this deployment's prefix", zap.String("key_prefix", cfg.Redis.KeyPrefix))
		}
		signInState = ephemeral.NewRedis(redisClient)
	} else {
		zapLogger.Warn("Redis is off (AUTHWAY_REDIS_ENABLED=false): sign-in state is kept in this process — run one instance only, or a sign-in started on one is not found on another")
		signInState = ephemeral.NewMemory(context.Background())
	}

	// Initialize Hydra client
	hydraClient := hydra.NewClient(cfg.Hydra.AdminURL)

	// Initialize validator
	validate := validator.New()

	// Initialize services
	// The audit log records each account as it is created, whichever path
	// created it.
	auditService := audit.NewService(db, zapLogger)
	userService := user.NewService(db, zapLogger, user.PasswordMinLength(tenantService.PasswordMinLength), user.OnCreated(func(u *user.User) {
		auditService.LogAsync(&audit.AuditEntry{
			TenantID:     u.TenantID,
			Action:       audit.ActionUserCreated,
			Severity:     audit.SeverityInfo,
			ActorType:    "system",
			ResourceType: "user",
			ResourceID:   u.ID.String(),
			Success:      true,
		})
	}))
	// The credentials clients hold at other identity providers are sealed with
	// the same at-rest key as the second-factor secrets.
	client.SealSecretsWith(totpCipher)
	// The token exchange of delegations (set up below, once the client
	// service exists): a deleted client's trust at the authorization server
	// goes with it.
	var exchanger *delegation.Exchanger
	clientService := client.NewService(db, zapLogger, hydraClient,
		client.CallableAddresses(outbound.Policy{AllowPrivate: cfg.Security.AllowPrivateTargets}),
		client.OnDeleted(func(clientID string) {
			if exchanger == nil {
				return
			}
			if err := exchanger.Distrust(clientID); err != nil {
				zapLogger.Error("Failed to remove the authorization server's trust for a deleted client", zap.Error(err), zap.String("client_id", clientID))
			}
		}))
	if err := client.ResealStoredSecrets(clientService, zapLogger); err != nil {
		zapLogger.Error("Resealing stored provider secrets failed (non-fatal; they still read, and it is retried on next start)", zap.Error(err))
	}
	// Social sign-in may only create an account for an invited address
	// (invitation-only onboarding). The gate is a read-only view of the
	// invitations table, so it exists well before the invitation service does.
	invitationGate := invitation.NewGate(db)
	googleService := social.NewGoogleService(&cfg.Google, userService, invitationGate, clientService, zapLogger)
	githubService := social.NewGitHubService(&cfg.GitHub, userService, invitationGate, clientService, zapLogger)
	microsoftService := social.NewMicrosoftService(&cfg.Microsoft, userService, invitationGate, clientService, zapLogger)
	appleService := social.NewAppleService(&cfg.Apple, userService, invitationGate, clientService, zapLogger)

	// Initialize Claims Service
	claimsRepo := claims.NewRepository(db)
	claimsService := claims.NewService(claimsRepo, zapLogger)
	termsService := terms.NewService(db)

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
			Senders:     mailSenders,
		}
		emailService = email.NewSendwayEmailService(sendwayConfig, zapLogger)
	} else {
		zapLogger.Info("Sending mail through SMTP",
			zap.String("smtpHost", cfg.Email.SMTPHost), zap.Int("smtpPort", cfg.Email.SMTPPort),
			zap.String("security", cfg.Email.SMTPSecurity), zap.Bool("authenticates", cfg.Email.SMTPUser != ""))

		smtpConfig := email.Config{
			SMTPHost:     cfg.Email.SMTPHost,
			SMTPPort:     cfg.Email.SMTPPort,
			SMTPUsername: cfg.Email.SMTPUser,
			SMTPPassword: cfg.Email.SMTPPassword,
			SMTPSecurity: cfg.Email.SMTPSecurity,
			FromEmail:    cfg.Email.FromEmail,
			FromName:     cfg.Email.FromName,
			FrontendURL:  cfg.App.FrontendURL,
		}
		emailService = email.NewService(smtpConfig, zapLogger)
	}

	// Requests that send mail answer without waiting for the mail service:
	// mailSenders at once, and up to 256 messages waiting for one.
	mailer := email.NewAsync(emailService, zapLogger, mailSenders, 256)

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
	// The deployment's own origins come from configuration; an application's
	// origins come from its client registration, so a new application needs
	// no redeploy.
	app.Use(middleware.CORS(cfg.CORS.AllowedOrigins, middleware.RememberOrigins(clientService.AllowsOrigin, time.Minute)))
	app.Use(middleware.RequestLogger(zapLogger))
	app.Use(telemetry.RequestTracking(telemetryClient, zapLogger))

	// Health check. It names the migrations this binary ships, so a deployment
	// can check the database against what was deployed.
	shippedMigrations, err := database.ShippedVersions()
	if err != nil {
		zapLogger.Fatal("Failed to read the migrations built into the binary", zap.Error(err))
	}
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"service":    "authway",
			"version":    cfg.App.Version,
			"migrations": shippedMigrations,
			"timestamp":  c.Context().Time(),
		})
	})

	// Public configuration endpoint for OIDC discovery
	// Clients can use this to discover the Hydra (OIDC Authority) URL
	// Bootstrap document for an application or the SDK: the OIDC issuer, the
	// backend that serves Authway's own API, and the login UI. Everything else
	// an application needs comes from the issuer's OpenID Connect discovery
	// document. It is the one such document — the contract describes it.
	app.Get("/.well-known/authway-config", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"issuer":  cfg.Hydra.PublicURL,
			"api_url": cfg.App.BaseURL,
			// The login UI's public address. Advertised so applications can
			// link to it, and so the deploy gate can read back the value this
			// instance actually got — a wrong one means every emailed link 404s.
			"auth_ui": cfg.App.FrontendURL,
			"version": cfg.App.Version,
			// Not in the contract: what SDK releases before 0.3.0 read instead
			// of issuer.
			// TODO: remove once no application runs an SDK older than 0.3.0.
			"oauth_url": cfg.Hydra.PublicURL,
		})
	})

	// Feature services (audit, webhooks, invitations, ...) are initialized
	// early so the audit.Service is available to wire into write-path handlers
	// below. Route registration still happens later once jwtAuth/adminAuth are
	// constructed.
	newFeatureServices := InitNewFeatureServices(db, zapLogger, userService, tenantService, mailer, auditService, cfg.App.FrontendURL, cfg.Security.AllowPrivateTargets, totpCipher)

	serviceClientService := serviceclient.NewService(db, zapLogger, hydraClient)
	serviceClientHandler := handler.NewServiceClientHandler(serviceClientService, zapLogger, newFeatureServices.AuditService)

	// Admin handler depends on audit.Service so auth failures surface in
	// audit_logs (see pkg/admin/handler.go logAuthFailure).
	adminHandler := admin.NewHandler(adminService, zapLogger, cfg.App.Version, cfg.Admin.APIKey, newFeatureServices.AuditService)

	// Initialize MFA Service — constructed before authHandler because Login()
	// now needs it to gate acceptance on TOTPEnabled.
	mfaService := mfa.NewService(db, userService, zapLogger, cfg.App.Name, totpCipher)

	// Initialize handlers
	// Every sign-in opens a session as long as its tenant says, and owes the
	// documents its tenant asks its users to accept.
	signInPolicy := handler.SignInPolicy{
		Sessions: handler.SessionPolicy{Timeout: tenantService.SessionTimeout},
		Terms:    termsService,
	}
	socialHandler := handler.NewSocialHandlerWithAllProviders(googleService, githubService, microsoftService, appleService, userService, clientService, hydraClient, zapLogger, newFeatureServices.AuditService, handler.NewOAuthStateStore(signInState), cfg.App.FrontendURL, signInPolicy)
	// A consent to a client's delegation scope grants a delegation.
	delegationService := delegation.NewService(db)
	approvals := delegation.NewApprovals(db)
	delegationHandler := delegation.NewHandler(delegationService, hydraClient, newFeatureServices.AuditService, zapLogger)
	authHandler := handler.NewAuthHandler(userService, clientService, claimsService, mfaService, hydraClient, zapLogger, newFeatureServices.AuditService, signInState, socialHandler, signInPolicy)
	authHandler.GrantsDelegations(delegationService)
	clientHandler := handler.NewClientHandler(services, zapLogger, newFeatureServices.AuditService)
	emailHandler := handler.NewEmailHandler(emailRepo, mailer, userService, clientService, hydraClient, validate, zapLogger, newFeatureServices.AuditService, tenantService.Name)
	logoutFlowHandler := handler.NewLogoutFlowHandler(hydraClient, authHandler, zapLogger)
	userHandler := handler.NewUserHandler(services, hydraClient, zapLogger, newFeatureServices.AuditService)
	mfaHandler := handler.NewMFAHandler(mfaService, userService, zapLogger, newFeatureServices.AuditService, hydraClient)

	// The authorization server sends the browser here to start a login; it is
	// handed on to the login UI with an opaque flow id. The paths sit under
	// /flows so the API and the login UI can share one origin: the UI's own
	// screens are /login, /consent, /logout and /device.
	flowEntryHandler := handler.NewFlowEntryHandler(cfg.App.FrontendURL, authHandler)
	loginRateLimit := ratelimitmw.LoginRateLimit(signInState)
	app.Get("/flows/login", flowEntryHandler.Login)
	app.Get("/flows/consent", flowEntryHandler.Consent)
	app.Get("/flows/logout", flowEntryHandler.Logout)
	app.Get("/flows/device", flowEntryHandler.Device)

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

	// The token endpoint discovery advertises: it passes every grant the
	// authorization server knows on to it unchanged, and answers the ones
	// Authway assembles itself.
	tokenEndpoint := handler.NewTokenEndpoint(cfg.Hydra.EngineURL(), zapLogger)
	// A client exchanges a delegation there (RFC 8693): the authorization
	// server issues the token by its assertion grant, on an assertion signed
	// with a key kept in the database. Without the key, no exchange.
	if signer, err := delegation.LoadSigner(db, totpCipher); err != nil {
		zapLogger.Error("Token exchange is off: the delegation signing key could not be loaded", zap.Error(err))
	} else {
		exchanger = delegation.NewExchanger(delegationService, signer, hydraClient, handler.ClientExchangeScopes(clientService), cfg.App.BaseURL)
		// A client acting for a user may ask them to approve one action; the
		// user answers on the login UI's approval screen.
		exchanger.HandlesApprovals(approvals, handler.NewEngineClientAuth(cfg.Hydra.EngineURL(), clientService, signInState, zapLogger), strings.TrimRight(cfg.App.FrontendURL, "/")+"/approve")
		tokenEndpoint.ExchangesDelegations(exchanger, newFeatureServices.AuditService)
	}
	app.Post("/oauth2/token", tokenEndpoint.Token)
	if exchanger.OffersApprovals() {
		app.Post("/oauth2/approval-requests", tokenEndpoint.ApprovalRequests)
	}
	// The provider's metadata as Authway offers it — reached at the issuer's
	// address where the deployment routes it here (docs/DEPLOYMENT.md).
	discovery := handler.NewDiscovery(cfg.Hydra.EngineURL(), cfg.App.BaseURL, exchanger != nil, exchanger.OffersApprovals(), zapLogger)
	app.Get("/.well-known/openid-configuration", discovery.Serve)
	app.Get("/.well-known/oauth-authorization-server", discovery.Serve)
	// The authorization server calls this before it issues a token. Served
	// only when the deployment set the key the authorization server presents.
	if len(cfg.Hydra.TokenHookKeys) > 0 {
		app.Post("/internal/token-hook", handler.NewTokenHook(cfg.Hydra.TokenHookKeys, exchanger, zapLogger).Hook)
	}

	// API routes
	// API v1 routes
	v1 := app.Group("/api/v1")

	// Email verification and password reset routes
	emailHandler.RegisterRoutes(v1)

	// What this deployment offers, so screens show only what works.
	capabilities := handler.NewCapabilitiesHandler(socialHandler)
	if exchanger != nil {
		capabilities.ExchangesDelegations()
	}
	if exchanger.OffersApprovals() {
		capabilities.TakesApprovalRequests()
	}
	v1.Get("/capabilities", capabilities.Get)
	v1.Get("/account-entry", handler.NewAccountEntryHandler(clientService, cfg.App.FrontendURL, zapLogger).Get)

	// A resource server asks whether a token is still good (RFC 7662). The
	// caller is a confidential client, not a user and not an administrator.
	// Wrong credentials count against the caller's address like failed
	// sign-ins do: each one costs a deliberately slow secret check.
	v1.Post("/oauth2/introspect", ratelimitmw.LoginRateLimit(signInState), handler.NewIntrospectionHandler(clientService, hydraClient, zapLogger).Introspect)

	// What the sign-in screen should do with a login flow: show the form (and
	// which sign-in methods the client allows) or redirect.
	v1.Get("/login-flows/:flow", authHandler.GetLoginFlow)
	v1.Post("/login-flows/:flow/password", loginRateLimit, authHandler.SubmitPassword)
	v1.Post("/login-flows/:flow/mfa", loginRateLimit, authHandler.VerifyMFALogin)
	v1.Post("/login-flows/:flow/mfa/recovery", loginRateLimit, authHandler.VerifyMFARecoveryLogin)
	// The documents the user's tenant asks them to accept, once every factor
	// has passed. The terms_challenge is the credential, as mfa_challenge is.
	v1.Get("/login-flows/:flow/terms", authHandler.GetOwedTerms)
	v1.Post("/login-flows/:flow/terms", authHandler.AcceptTerms)
	v1.Post("/login-flows/:flow/terms/decline", authHandler.DeclineTerms)
	// A page navigation (not fetch): the browser goes on to the provider.
	v1.Get("/login-flows/:flow/social/:provider", socialHandler.StartSocialLogin)

	// Emailed sign-in links: sent from the login screen, redeemed from the
	// link's landing page in the same browser.
	magicLinkHandler := handler.NewMagicLinkFlowHandler(authHandler, newFeatureServices.PasswordlessService)
	v1.Post("/login-flows/:flow/magic-link", ratelimitmw.MagicLinkRateLimit(signInState), magicLinkHandler.Send)
	v1.Post("/magic-links/inspect", magicLinkHandler.Inspect)
	v1.Post("/magic-links/redeem", loginRateLimit, magicLinkHandler.Redeem)
	v1.Post("/login-flows/:flow/magic-link/code", loginRateLimit, magicLinkHandler.RedeemCode)

	// Consent and logout screens: what to ask, and the user's answer.
	v1.Get("/consent-flows/:flow", authHandler.GetConsentFlow)
	v1.Post("/consent-flows/:flow/accept", authHandler.AcceptConsent)
	v1.Post("/consent-flows/:flow/reject", authHandler.RejectConsent)
	v1.Post("/logout-flows/:flow", logoutFlowHandler.CompleteLogout)
	// A device sign-in (RFC 8628): the code the user typed on the device page.
	deviceFlowHandler := handler.NewDeviceFlowHandler(hydraClient, zapLogger)
	v1.Post("/device-flows/:flow", loginRateLimit, deviceFlowHandler.SubmitUserCode)

	// JWT middleware for authenticated routes (supports both JWT and opaque tokens)
	jwtAuth := middleware.JWTAuth(zapLogger, hydraClient, db)

	// The signed-in user's own profile. There is no lookup by id: any signed-in
	// user could read another's email and name with it, whatever their tenant.
	v1.Get("/profile/me", jwtAuth, authHandler.ProfileMe)
	v1.Delete("/profile/me", jwtAuth, authHandler.DeleteMe)
	v1.Post("/users/password", jwtAuth, loginRateLimit, authHandler.ChangePassword)
	v1.Get("/users/connections", jwtAuth, authHandler.ListConnections)
	v1.Delete("/users/connections/:provider", jwtAuth, authHandler.RemoveConnection)
	// What the user let clients do for them while they are away, and taking
	// it back.
	v1.Get("/users/delegations", jwtAuth, delegationHandler.ListMine)
	v1.Delete("/users/delegations/:id", jwtAuth, delegationHandler.RevokeMine)
	// What a client asks the user to approve, and the user's answer — on the
	// approval screen (the account application's token only).
	approvalHandler := handler.NewApprovalHandler(approvals, clientService, newFeatureServices.AuditService, zapLogger)
	v1.Get("/users/approvals/:id", jwtAuth, approvalHandler.Get)
	v1.Post("/users/approvals/:id/approve", jwtAuth, approvalHandler.Approve)
	v1.Post("/users/approvals/:id/deny", jwtAuth, approvalHandler.Deny)

	// Logout route - direct session revocation.
	// Requires a valid bearer token: the subject to revoke is taken from the
	// validated token, so a caller can only revoke their own sessions.
	v1.Post("/logout", jwtAuth, authHandler.Logout)

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

	// Client Hydra sync (admin operation for one-time migration)
	v1.Post("/clients/sync-hydra", adminAuth, tenantscope.RequirePlatform(), clientHandler.SyncToHydra)

	// Claims stored for a user go into the tokens issued to them, so only a
	// trusted party writes them: the admin API, or a tenant's service
	// credential granted admin.claims:write (scoped to that tenant's users).
	claimsHandler := claims.NewHandler(claimsService, userService, auditService, zapLogger)
	claimsAuth := adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.claims:write")
	v1.Get("/users/:id/claims", claimsAuth, claimsHandler.List)
	v1.Patch("/users/:id/claims", claimsAuth, claimsHandler.Patch)

	// The documents a tenant's users accept at sign-in: the tenant's own
	// business, so its service credential may keep them (admin.terms:write),
	// as may the admin API.
	termsHandler := terms.NewHandler(termsService, tenantService, userService, auditService, zapLogger)
	termsAuth := adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.terms:write")
	v1.Get("/tenants/:id/terms", termsAuth, termsHandler.List)
	v1.Put("/tenants/:id/terms/:kind", termsAuth, termsHandler.Publish)
	v1.Delete("/tenants/:id/terms/:kind", termsAuth, termsHandler.Retire)
	v1.Get("/users/:id/terms-acceptances", termsAuth, termsHandler.Acceptances)

	// MFA management routes (authenticated users). Registered before the
	// admin routes on /users/:id: a route matches in the order it was
	// registered, and "mfa" would otherwise be taken for a user id — which
	// is how turning the second factor off was answered by the admin route
	// (401) instead of ever reaching its handler.
	v1.Post("/users/mfa/setup", jwtAuth, mfaHandler.SetupMFA)
	v1.Post("/users/mfa/verify", jwtAuth, mfaHandler.VerifyMFA)
	v1.Delete("/users/mfa", jwtAuth, loginRateLimit, mfaHandler.DisableMFA)
	v1.Get("/users/mfa/status", jwtAuth, mfaHandler.GetMFAStatus)
	v1.Post("/users/mfa/recovery/regenerate", jwtAuth, loginRateLimit, mfaHandler.RegenerateRecoveryCodes)

	// User management routes. A tenant's service credential may read its
	// users (admin.users:read) and change them (admin.users:write) — what a
	// product's own support console needs; deleting an account stays with the
	// admin API.
	usersReadAuth := adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.users:read")
	usersWriteAuth := adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.users:write")
	v1.Get("/delegations", adminAuth, delegationHandler.List)
	v1.Delete("/delegations/:id", adminAuth, delegationHandler.Revoke)
	v1.Get("/users", usersReadAuth, userHandler.List)
	v1.Post("/users/search", usersReadAuth, userHandler.Search)
	v1.Get("/users/:id", usersReadAuth, userHandler.Get)
	v1.Put("/users/:id", usersWriteAuth, userHandler.Update)
	v1.Delete("/users/:id", adminAuth, userHandler.Delete)

	// Tenant Management API routes (Admin only)
	tenantHandler := tenant.NewHandler(tenantService, validate, newFeatureServices.AuditService)
	tenantHandler.OnDeleted(clientService.DeleteSystemClients)
	tenantHandler.OnTurned(clientService.TenantTurned)
	tenantHandler.RegisterRoutes(app, adminAuth)

	// Service client provisioning — admin-only, since
	// this mints new M2M credentials. The credentials it mints are what
	// clientCreateAuth (above) accepts on POST /clients.
	v1.Post("/tenants/:id/service-clients", adminAuth, tenantscope.RequireOwnerOf("id"), serviceClientHandler.Create)
	v1.Get("/tenants/:id/service-clients", adminAuth, tenantscope.RequireOwnerOf("id"), serviceClientHandler.List)
	v1.Delete("/tenants/:id/service-clients/:service_client_id", adminAuth, tenantscope.RequireOwnerOf("id"), serviceClientHandler.Revoke)

	// Console accounts: users of the system tenant whose memberships say what
	// they administer. The admin middleware takes their access tokens beside
	// the admin key and the console session.
	consoleService := console.NewService(db)
	adminHandler.UseConsoleAccounts(hydraClient, consoleService)
	// Public: which client the console signs console accounts in with.
	// Registered before the authenticated /console routes.
	v1.Get("/console/entry", handler.NewConsoleEntryHandler(clientService, consoleService, cfg.App.ConsoleURL, zapLogger).Get)
	consoleHandler := console.NewHandler(consoleService, newFeatureServices.AuditService, zapLogger)
	consoleHandler.RegisterRoutes(v1, adminAuth)
	// Support access lasts an hour; its end goes into the tenant's audit log
	// within the next quarter hour, and at start for any that ended while no
	// instance ran.
	go func() {
		consoleHandler.CloseExpiredSupport(context.Background())
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			consoleHandler.CloseExpiredSupport(context.Background())
		}
	}()

	// Admin Console routes
	adminHandler.RegisterRoutes(app)

	// ======================================
	// Register New Feature Services routes (Phase 1.7)
	// Services were constructed earlier so audit.Service could be injected
	// into ClientHandler; route registration waits for jwtAuth/adminAuth.
	// ======================================
	// adminAuth (declared above) already accepts session tokens and the API key.
	newFeatureServices.RegisterRoutes(v1, jwtAuth, adminAuth,
		adminHandler.GetClientAuth(hydraClient, serviceClientService, "admin.audit:read"))
	newFeatureServices.StartBackgroundCleanupTasks(zapLogger)

	// Cleanup expired admin sessions, and the verification and reset links
	// that can no longer be used, periodically
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := adminService.CleanupExpiredSessions(); err != nil {
				zapLogger.Error("Failed to cleanup expired admin sessions", zap.Error(err))
			}
			if err := emailRepo.CleanupExpiredTokens(); err != nil {
				zapLogger.Error("Failed to cleanup expired email links", zap.Error(err))
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

	go func() {
		if err := app.Listen(":" + port); err != nil {
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
	if err := app.ShutdownWithContext(ctx); err != nil {
		zapLogger.Warn("HTTP shutdown did not finish", zap.Error(err))
	}
	if err := mailer.Close(ctx); err != nil {
		zapLogger.Warn("Mail still being sent at shutdown was dropped", zap.Error(err))
	}
	if err := newFeatureServices.WebhookService.Close(ctx); err != nil {
		zapLogger.Warn("Webhook deliveries still being made at shutdown were dropped", zap.Error(err))
	}
}
