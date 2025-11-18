// Package main provides the TapeDeck HTTP server for NFC-based media playback.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Chuntttttt/tapedeck/internal/app"
	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/internal/router"
	"github.com/Chuntttttt/tapedeck/templates/pages"
)

func main() {
	// Load application configuration from environment variables first (needed for DataDir)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Try to load runtime configuration from config.yml (using DataDir path)
	runtimeCfg, err := config.LoadRuntimeConfig(cfg.ConfigPath())
	needsSetup := false
	if err != nil || runtimeCfg.IsEmpty() {
		logger.Info("Runtime configuration not found or empty", "error", err)
		logger.Info("Setup wizard will be required before using the application")
		needsSetup = true
	} else if err := runtimeCfg.Validate(); err != nil {
		logger.Info("Runtime configuration validation failed", "error", err)
		logger.Info("Setup wizard will be required before using the application")
		needsSetup = true
	}

	// Set up structured logging to both stdout and file
	logFile, err := os.OpenFile(cfg.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		logger.Warn("Failed to open log file", "error", err)
		logger.Init(cfg.LogLevel, os.Stdout)
	} else {
		defer func() {
			if err := logFile.Close(); err != nil {
				logger.Warn("Failed to close log file", "error", err)
			}
		}()
		logger.Init(cfg.LogLevel, os.Stdout, logFile)
	}

	// Also set standard log output for backward compatibility
	if logFile != nil {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
	}

	logger.Info("Starting TapeDeck", "log_level", cfg.LogLevel, "dev_mode", cfg.DevMode, "needs_setup", needsSetup)

	// Initialize database with encryption key
	database, err := db.New(cfg.DatabasePath(), cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err) //nolint:gocritic // exitAfterDefer: acceptable for fatal errors
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Warn("Failed to close database", "error", err)
		}
	}()

	// Run migrations
	if err := database.RunMigrations("./migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	logger.Info("Database initialized successfully")

	// Check if settings exist in database (required for HA token)
	// If config.yml exists but settings don't, require setup
	if !needsSetup {
		ctx := context.Background()
		_, err := database.GetSettings(ctx)
		if err != nil {
			logger.Info("Settings not found in database - setup wizard required")
			needsSetup = true
		}
	}

	// Initialize session store
	sessionStore := middleware.NewSessionStore([]byte(cfg.SessionSecret), cfg.RequireTLS)

	// Security warnings
	if cfg.DevMode {
		logger.Warn("DEV_MODE enabled: TLS verification disabled")
	}
	if !cfg.RequireTLS {
		logger.Warn("TLS NOT REQUIRED - Session cookies will be sent over HTTP")
		logger.Warn("This is INSECURE for internet-exposed deployments")
		logger.Warn("Set REQUIRE_TLS=true and use HTTPS in production")
	}

	// Initialize Plex auth client (needed for both normal operation and setup)
	plexAuth := plex.NewAuthClient("https://plex.tv", "tapedeck-client-id", "TapeDeck", cfg.DevMode)

	// Initialize auth handler (needed for both normal operation and setup)
	authHandler := handlers.NewAuthHandler(sessionStore, plexAuth, database)

	// Create app initializer
	initializer := &app.Initializer{
		ConfigPath:   cfg.ConfigPath(),
		SessionStore: sessionStore,
		Database:     database,
		DevMode:      cfg.DevMode,
	}

	// Declare handler variables that will be initialized either at startup or after setup
	var appHandlers *app.Handlers
	var settingsHandler *handlers.SettingsHandler

	// initializeHandlers sets up all runtime handlers from config.yml
	//
	// This function is called in three scenarios:
	// 1. At startup if config.yml exists and is valid
	// 2. After setup wizard completion (via callback)
	// 3. After settings update (via callback)
	initializeHandlers := func() error {
		var err error
		var existingHAClient *ha.HAClient
		if appHandlers != nil {
			existingHAClient = appHandlers.HAClient
		}
		appHandlers, err = initializer.Initialize(existingHAClient)
		return err
	}
	// Initialize setup handler (always available)
	setupHandler := handlers.NewSetupHandler(sessionStore, cfg.ConfigPath(), plexAuth, database, cfg.DevMode, initializeHandlers)

	// Initialize settings handler (always available) - uses same reload callback as setup
	settingsHandler = handlers.NewSettingsHandler(sessionStore, cfg.ConfigPath(), database, initializeHandlers)

	// Initialize handlers if config exists
	if !needsSetup {
		if err := initializeHandlers(); err != nil {
			log.Fatalf("Failed to initialize handlers: %v", err)
		}
		// Set up cleanup for HA client
		if appHandlers != nil && appHandlers.HAClient != nil {
			defer appHandlers.HAClient.Close()
		}
	} else {
		logger.Info("Config not ready - setup wizard will initialize handlers after completion")
	}

	// Create auth middleware chain: WithUserID -> WithUser -> RequireAuth
	authMiddlewareChain := func(next http.Handler) http.Handler {
		return middleware.WithUserID(sessionStore)(
			middleware.WithUser(sessionStore, database)(
				middleware.RequireAuth(sessionStore)(next),
			),
		)
	}

	// Create router dependencies
	deps := &router.Dependencies{
		AuthHandler:     authHandler,
		SetupHandler:    setupHandler,
		SettingsHandler: settingsHandler,
		AuthMiddleware:  authMiddlewareChain,

		// Handler getters that return current values (updated after setup/settings changes)
		GetMappingsHandler: func() *handlers.MappingsHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Mappings
		},
		GetMediaHandler: func() *handlers.MediaHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Media
		},
		GetMediaDetailHandler: func() *handlers.MediaDetailHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.MediaDetail
		},
		GetPairingHandler: func() *handlers.PairingHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Pairing
		},
		GetPlaybackHandler: func() *handlers.PlaybackHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Playback
		},
		GetPlayHandler: func() *handlers.PlayHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Play
		},
		GetStatusHandler: func() *handlers.StatusHandler {
			if appHandlers == nil {
				return nil
			}
			return appHandlers.Status
		},

		// HandlersReady is used by requireInitialized middleware to check if
		// handlers have been initialized. Returns true only if all runtime
		// handlers are non-nil (i.e., initializeHandlers() has been called).
		HandlersReady: func() bool {
			return appHandlers != nil
		},
	}

	// Create router
	r := router.New(deps)

	// Configure CSRF protection using Go 1.25 standard library
	// CrossOriginProtection uses Fetch metadata headers (no tokens/cookies)
	csrfProtection := http.NewCrossOriginProtection()

	// Set custom error handler
	csrfProtection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Warn("CSRF validation failed",
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr,
			"origin", r.Header.Get("Origin"),
			"host", r.Host)

		// Render proper error page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		if err := pages.Error(http.StatusForbidden, "CSRF validation failed. Please try again.").Render(r.Context(), w); err != nil {
			logger.Error("Failed to render CSRF error page", "error", err)
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
		}
	}))

	// Exempt routes that don't use HTML forms
	csrfProtection.AddInsecureBypassPattern("/api/")    // API routes use JSON
	csrfProtection.AddInsecureBypassPattern("/ws/")     // WebSocket routes
	csrfProtection.AddInsecureBypassPattern("/metrics") // Prometheus scraping
	csrfProtection.AddInsecureBypassPattern("/health")  // Health check

	// In dev mode, allow localhost cross-origin requests (for Air proxy)
	if cfg.DevMode {
		csrfProtection.AddInsecureBypassPattern("http://localhost:")
		csrfProtection.AddInsecureBypassPattern("http://127.0.0.1:")
		logger.Info("CSRF protection: allowing localhost cross-origin requests in dev mode")
	}

	// Wrap with middleware chain
	// 1. CSRF protection (validates cross-origin requests using Fetch metadata)
	// 2. Metrics middleware (tracks request metrics)
	// 3. Request ID middleware (generates unique request ID)
	// 4. User ID middleware (extracts user ID from session)
	// 5. Request logger middleware (creates scoped logger with request metadata)
	// 6. Request logging middleware (logs all requests)
	// 7. Setup middleware (checks config for all non-exempted routes)
	handler := csrfProtection.Handler(
		middleware.MetricsMiddleware()(
			middleware.WithRequestID()(
				middleware.WithUserID(sessionStore)(
					middleware.WithRequestLogger()(
						middleware.RequestLogger()(
							middleware.SetupMiddleware(cfg.ConfigPath(), sessionStore)(r),
						),
					),
				),
			),
		),
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: constants.ServerReadHeaderTimeout,
	}

	// Graceful shutdown
	go func() {
		logger.Info("Starting TapeDeck", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), constants.ServerShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server stopped")
}
