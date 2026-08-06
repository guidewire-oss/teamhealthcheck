package main

import (
	"context"
	"database/sql"
	"mime"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/XSAM/otelsql"
	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/application/trends"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/email"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/middleware"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
	"github.com/agopalakrishnan/teams360/backend/pkg/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

func main() {
	ctx := context.Background()

	// Initialize logger
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	prettyLogs := os.Getenv("LOG_PRETTY") == "true"

	logger.Init(logger.Config{
		Level:  logLevel,
		Pretty: prettyLogs,
	})

	log := logger.Get()

	// Initialize OpenTelemetry
	otelCfg := telemetry.DefaultConfig()
	shutdownTelemetry, err := telemetry.Init(ctx, otelCfg)
	if err != nil {
		log.WithError(err).Warn("failed to initialize telemetry, continuing without it")
	} else {
		defer func() {
			if err := shutdownTelemetry(ctx); err != nil {
				log.WithError(err).Warn("error shutting down telemetry")
			}
		}()
	}

	// Set Gin mode based on environment
	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.DebugMode
	}
	gin.SetMode(mode)

	// Connect to database with OTel instrumentation
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable"
	}

	// Register the otelsql driver wrapper
	db, err := otelsql.Open("postgres", databaseURL,
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
			semconv.DBNameKey.String("teams360"),
		),
	)
	if err != nil {
		log.WithError(err).Fatal("failed to connect to database")
	}
	defer db.Close()

	// Verify database connection - if database doesn't exist, create it
	if err := db.Ping(); err != nil {
		// Try to create the database if it doesn't exist
		log.Info("database doesn't exist, attempting to create it")
		adminDB, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
		if err != nil {
			log.WithError(err).Fatal("failed to connect to postgres database")
		}
		_, err = adminDB.Exec("CREATE DATABASE teams360")
		adminDB.Close()

		if err != nil {
			log.WithError(err).Fatal("failed to create database")
		}

		log.Info("database created successfully")

		// Reconnect to the new database with OTel instrumentation
		db, err = otelsql.Open("postgres", databaseURL,
			otelsql.WithAttributes(
				semconv.DBSystemPostgreSQL,
				semconv.DBNameKey.String("teams360"),
			),
		)
		if err != nil {
			log.WithError(err).Fatal("failed to connect to newly created database")
		}

		if err := db.Ping(); err != nil {
			log.WithError(err).Fatal("failed to ping newly created database")
		}
	}
	log.Info("database connection established")

	// Store APP_ENV in app_config table for runtime queries
	if err := postgres.EnsureAppConfig(db); err != nil {
		log.WithError(err).Fatal("failed to set app_config")
	}

	// Run migrations
	driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
	if err != nil {
		log.WithError(err).Fatal("failed to create migration driver")
	}

	migrationEngine, err := migrate.NewWithDatabaseInstance(
		"file://infrastructure/persistence/postgres/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		log.WithError(err).Fatal("failed to create migration engine")
	}

	if err := migrationEngine.Up(); err != nil && err != migrate.ErrNoChange {
		log.WithError(err).Fatal("failed to run migrations")
	}
	log.Info("database migrations completed")

	// Seed demo data only when APP_ENV=demo
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "dev"
	}
	if appEnv == "demo" {
		if err := postgres.SeedDemoData(db); err != nil {
			log.WithError(err).Fatal("failed to seed demo data")
		}
	}

	// Initialize repositories
	healthCheckRepo := postgres.NewHealthCheckRepository(db)
	userRepo := postgres.NewUserRepository(db)
	teamRepo := postgres.NewTeamRepository(db)
	orgRepo := postgres.NewOrganizationRepository(db)

	// Initialize services
	trendsService := trends.NewService(db)
	jwtService := services.NewJWTService()

	// Initialize email service: SES > SMTP > disabled
	var emailSender email.Sender
	if sesCfg := email.LoadSESConfig(); sesCfg != nil {
		ses, err := email.NewSESEmailService(sesCfg)
		if err != nil {
			log.WithError(err).Fatal("failed to initialize SES email service")
		}
		emailSender = ses
		log.WithField("region", sesCfg.Region).Info("AWS SES email service configured")
	} else if smtpCfg := email.LoadConfig(); smtpCfg != nil {
		emailSender = email.NewSMTPEmailService(smtpCfg)
		log.WithField("host", smtpCfg.Host).Info("SMTP email service configured")
	} else {
		log.Info("No email service configured, email notifications disabled")
	}

	// Initialize notification service
	notificationService := services.NewNotificationService(emailSender, teamRepo, userRepo, orgRepo)

	// Initialize password reset service
	passwordResetRepo := postgres.NewPasswordResetRepository(db)
	passwordResetService := services.NewPasswordResetService(passwordResetRepo, userRepo, emailSender)

	// Initialize router (use gin.New() instead of gin.Default() to disable default logger)
	router := gin.New()
	router.Use(gin.Recovery()) // Keep panic recovery

	// Add OpenTelemetry middleware for distributed tracing
	router.Use(otelgin.Middleware("teams360-api"))

	// Add request ID and logging middleware (our zerolog-based logger replaces Gin's default)
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.RequestLoggerMiddleware())

	// Add security middleware
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.ContentTypeValidator())
	router.Use(middleware.MaxBodySizeMiddleware(10 * 1024 * 1024)) // 10MB max body size

	// Health check endpoint (used by tests and load balancers)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Setup API routes with repository injection
	v1.SetupHealthCheckRoutes(router, healthCheckRepo, orgRepo, jwtService, notificationService)
	v1.SetupAuthRoutes(router, userRepo, orgRepo, jwtService)
	v1.SetupSSORoutes(router, userRepo, jwtService, orgRepo)
	v1.SetupManagerRoutes(router, healthCheckRepo, trendsService, jwtService, userRepo)
	v1.SetupTeamRoutes(router, healthCheckRepo, teamRepo, jwtService)
	v1.SetupTeamDashboardRoutes(router, db, jwtService) // Dashboard routes with JWT + team membership
	v1.SetupActionItemRoutes(router, db, jwtService)    // Action item CRUD routes
	v1.SetupUserRoutes(router, db, jwtService)          // User routes with JWT + same-user-or-manager
	v1.SetupProtectedUserRoutes(router, db, jwtService) // Protected routes requiring JWT
	v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, jwtService)
	v1.SetupPasswordResetRoutes(router, passwordResetService, userRepo)

	// Static file serving for frontend SPA
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "./web"
	}

	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		log.WithField("dir", webDir).Info("serving static frontend files")

		// Register a mime type for .js files (some alpine images miss it)
		mime.AddExtensionType(".js", "application/javascript")
		mime.AddExtensionType(".css", "text/css")
		mime.AddExtensionType(".svg", "image/svg+xml")

		// Resolve webDir to an absolute path for safe prefix checking
		absWebDir, err := filepath.Abs(webDir)
		if err != nil {
			log.WithError(err).Fatal("failed to resolve web directory path")
		}
		// Keep absWebDir without trailing separator; use absWebDirPrefix for checks
		absWebDirPrefix := absWebDir + string(filepath.Separator)

		router.NoRoute(func(c *gin.Context) {
			urlPath := c.Request.URL.Path

			// API routes get JSON 404
			if strings.HasPrefix(urlPath, "/api/") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}

			// Cache immutable Next.js static assets
			if strings.HasPrefix(urlPath, "/_next/static/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}

			// Resolve and validate path stays within webDir (prevent path traversal)
			filePath := filepath.Join(absWebDir, filepath.Clean("/"+urlPath))
			if filePath != absWebDir && !strings.HasPrefix(filePath, absWebDirPrefix) {
				c.JSON(400, gin.H{"error": "invalid path"})
				return
			}

			// Try exact file
			if info, statErr := os.Stat(filePath); statErr == nil && !info.IsDir() {
				c.File(filePath)
				return
			}

			// Try with .html extension (Next.js static export: /login -> login.html)
			htmlPath := filePath + ".html"
			if filePath != absWebDir && !strings.HasPrefix(htmlPath, absWebDirPrefix) {
				c.JSON(400, gin.H{"error": "invalid path"})
				return
			}
			if info, statErr := os.Stat(htmlPath); statErr == nil && !info.IsDir() {
				c.File(htmlPath)
				return
			}

			// Static asset paths (/_next/static/) should 404, not get SPA fallback
			if strings.HasPrefix(urlPath, "/_next/") {
				c.Status(404)
				return
			}

			// SPA fallback: serve index.html
			c.File(filepath.Join(absWebDir, "index.html"))
		})
	}

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.WithField("port", port).Info("starting Team360 API server")
		if err := router.Run(":" + port); err != nil {
			log.WithError(err).Fatal("failed to start server")
		}
	}()

	// Wait for interrupt signal
	<-quit
	log.Info("shutting down server gracefully...")
}
