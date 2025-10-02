package main

import (
	"log"
	"os"

	"authway/src/server/internal/config"
	"authway/src/server/internal/database"
	"authway/src/server/internal/handler"
	"authway/src/server/internal/hydra"
	"authway/src/server/internal/middleware"
	"authway/src/server/internal/service"
	"authway/src/server/internal/service/social"
	"authway/src/server/pkg/client"
	"authway/src/server/pkg/email"
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

	// Auto-migrate models
	if err := database.Migrate(db, &user.User{}, &client.Client{}, &email.EmailVerification{}, &email.PasswordReset{}); err != nil {
		zapLogger.Warn("Failed to migrate database (may already be migrated)", zap.Error(err))
	} else {
		zapLogger.Info("Database migration completed successfully")
	}

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
	clientService := client.NewService(db, zapLogger)
	googleService := social.NewGoogleService(&cfg.Google, userService, clientService, zapLogger)

	// Initialize email services
	emailService := email.NewService(&cfg.Email, cfg.App.BaseURL, zapLogger)
	emailRepo := email.NewRepository(db, zapLogger)

	// Create services struct for handlers
	services := &service.Services{
		UserService:   userService,
		ClientService: clientService,
	}

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.ErrorHandler,
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:3000,http://localhost:8080", // Default allowed origins
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))
	app.Use(middleware.RequestLogger(zapLogger))

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
	authHandler := handler.NewAuthHandler(userService, hydraClient)
	socialHandler := handler.NewSocialHandler(googleService, userService, hydraClient, zapLogger)
	clientHandler := handler.NewClientHandler(services, zapLogger)
	emailHandler := handler.NewEmailHandler(emailRepo, emailService, userService, validate, zapLogger)

	// Auth routes for Hydra login/consent flow
	app.Get("/login", authHandler.LoginPage)
	app.Post("/login", authHandler.Login)
	app.Get("/consent", authHandler.ConsentPage)
	app.Post("/consent", authHandler.Consent)
	app.Post("/consent/reject", authHandler.RejectConsent)

	// User registration
	app.Post("/register", authHandler.Register)

	// Social login routes
	app.Get("/auth/google/login", socialHandler.GoogleLogin)
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

	// Client Google OAuth configuration routes
	v1.Put("/clients/:id/google-oauth", clientHandler.UpdateGoogleOAuth)
	v1.Delete("/clients/:id/google-oauth", clientHandler.DisableGoogleOAuth)
	v1.Get("/clients/:id/google-oauth/status", clientHandler.GetGoogleOAuthStatus)

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
