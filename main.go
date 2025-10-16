// Package main provides the TapeDeck HTTP server for NFC-based media playback.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/internal/router"
	"github.com/Chuntttttt/tapedeck/internal/services"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/csrf"
)

// csrfExemptMiddleware wraps CSRF protection but exempts certain routes
// that don't use HTML forms (API endpoints use JSON, WebSocket endpoints use WS protocol)
func csrfExemptMiddleware(csrfProtect func(http.Handler) http.Handler, devMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Exempt API routes (use JSON, not forms)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Exempt WebSocket routes (use WS protocol, not forms)
			if strings.HasPrefix(r.URL.Path, "/ws/") {
				next.ServeHTTP(w, r)
				return
			}

			// Exempt metrics endpoint (Prometheus scraping)
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// Exempt health check endpoint
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// In dev mode, exempt localhost cross-origin requests (for Air proxy)
			if devMode && r.Method != "GET" && r.Method != "HEAD" {
				origin := r.Header.Get("Origin")
				if origin != "" && (strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")) {
					// Localhost origin in dev mode - skip CSRF check
					next.ServeHTTP(w, r)
					return
				}
			}

			// Apply CSRF protection to all other routes
			csrfProtect(next).ServeHTTP(w, r)
		})
	}
}

func main() {
	// Try to load runtime configuration from config.yml
	runtimeCfg, err := config.LoadRuntimeConfig("./config.yml")
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

	// Load application configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set up structured logging to both stdout and file
	logFile, err := os.OpenFile("tapedeck.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
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
	database, err := db.New(cfg.DatabasePath, cfg.EncryptionKey)
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

	// Declare handler variables that will be initialized either at startup or after setup
	// These start as nil and are populated by initializeHandlers() below.
	var mediaHandler *handlers.MediaHandler
	var mediaDetailHandler *handlers.MediaDetailHandler
	var mappingsHandler *handlers.MappingsHandler
	var playbackHandler *handlers.PlaybackHandler
	var pairingHandler *handlers.PairingHandler
	var statusHandler *handlers.StatusHandler
	var settingsHandler *handlers.SettingsHandler
	var haClient *ha.HAClient

	// initializeHandlers sets up all runtime handlers from config.yml
	//
	// This function is called in three scenarios:
	// 1. At startup if config.yml exists and is valid
	// 2. After setup wizard completion (via callback)
	// 3. After settings update (via callback)
	//
	// Handler initialization is synchronous and atomic - either all handlers are
	// initialized or the function returns an error. This ensures that routes
	// protected by requireInitialized middleware always have valid handlers.
	//
	// The HandlersReady function (used by requireInitialized middleware) checks
	// that all handlers are non-nil before allowing access to protected routes.
	initializeHandlers := func() error {
		logger.Info("Initializing handlers after setup completion")

		// Close existing HA client if it exists
		if haClient != nil {
			logger.Info("Closing existing Home Assistant connection")
			haClient.Close()
		}

		// Reload config
		runtimeCfg, err := config.LoadRuntimeConfig("./config.yml")
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Load HA token from database
		ctx := context.Background()
		settings, err := database.GetSettings(ctx)
		if err != nil {
			return fmt.Errorf("failed to load Home Assistant token from database: %w", err)
		}
		haToken := settings.HAToken

		// Build list of servers with their best connections
		var servers []handlers.ServerInfo
		var plexURL string      // For legacy handlers that need a single URL
		var plexServerID string // For playback handler

		for _, srv := range runtimeCfg.PlexServers {
			// TODO: Skip shared servers for now - they return 401 Unauthorized
			// See README.md "Known Limitations: Shared Plex Servers"
			if srv.Owner == "Shared" {
				logger.Info("Skipping shared server (not currently supported)", "server", srv.Name)
				continue
			}

			// Collect all connection URLs
			var urls []string
			for _, conn := range srv.Connections {
				// Skip docker internal addresses (172.17.0.x)
				if strings.Contains(conn.URI, "172-17-0-") {
					continue
				}
				urls = append(urls, conn.URI)
			}

			if len(urls) > 0 {
				servers = append(servers, handlers.ServerInfo{
					ID:   srv.ID,
					Name: srv.Name,
					URLs: urls,
				})

				// Use first server's first URL for legacy handlers
				if plexURL == "" {
					plexURL = urls[0]
					plexServerID = srv.ID
				}
			}
		}

		// Initialize handlers
		mediaHandler = handlers.NewMediaHandler(sessionStore, database, servers, cfg.DevMode)
		mediaDetailHandler = handlers.NewMediaDetailHandler(sessionStore, database, servers, cfg.DevMode, runtimeCfg.AppleTVs)
		mappingsHandler = handlers.NewMappingsHandler(sessionStore, database, servers, cfg.DevMode)
		playbackHandler = handlers.NewPlaybackHandler(database, plexServerID)

		// Initialize Home Assistant WebSocket client
		haClient = ha.NewHAClient(runtimeCfg.HomeAssistant.URL, haToken)
		if err := haClient.Connect(); err != nil {
			logger.Warn("Failed to connect to Home Assistant", "error", err)
		}

		// Initialize Home Assistant REST client
		haRest := ha.NewRestClient(runtimeCfg.HomeAssistant.URL, haToken, cfg.DevMode)

		// Initialize playback service
		playbackService := services.NewPlaybackService(database, haRest)

		// Initialize pairing handler
		pairingHandler = handlers.NewPairingHandler(
			sessionStore,
			database,
			haClient,
			playbackService,
			"./config.yml",
			cfg.DevMode,
		)

		// Initialize status handler
		statusHandler = handlers.NewStatusHandler(haClient, "./config.yml", database)

		// Sanity check: ensure all handlers were initialized
		// This should never fail unless there's a programming error above
		if mediaHandler == nil || mediaDetailHandler == nil || mappingsHandler == nil || playbackHandler == nil ||
			pairingHandler == nil || statusHandler == nil {
			return fmt.Errorf("handler initialization incomplete - this is a programming error")
		}

		logger.Info("All handlers initialized successfully")
		return nil
	}

	// Initialize setup handler (always available)
	setupHandler := handlers.NewSetupHandler(sessionStore, "./config.yml", plexAuth, database, cfg.DevMode, initializeHandlers)

	// Initialize settings handler (always available) - uses same reload callback as setup
	settingsHandler = handlers.NewSettingsHandler(sessionStore, "./config.yml", database, initializeHandlers)

	// Initialize handlers if config exists
	if !needsSetup {
		if err := initializeHandlers(); err != nil {
			log.Fatalf("Failed to initialize handlers: %v", err)
		}
		// Set up cleanup for HA client
		if haClient != nil {
			defer haClient.Close()
		}
	} else {
		logger.Info("Config not ready - setup wizard will initialize handlers after completion")
	}

	// Create router dependencies
	deps := &router.Dependencies{
		AuthHandler:     authHandler,
		SetupHandler:    setupHandler,
		SettingsHandler: settingsHandler,
		AuthMiddleware:  middleware.RequireAuth(sessionStore),

		// Handler getters that return current values (updated after setup/settings changes)
		GetMappingsHandler:    func() *handlers.MappingsHandler { return mappingsHandler },
		GetMediaHandler:       func() *handlers.MediaHandler { return mediaHandler },
		GetMediaDetailHandler: func() *handlers.MediaDetailHandler { return mediaDetailHandler },
		GetPairingHandler:     func() *handlers.PairingHandler { return pairingHandler },
		GetPlaybackHandler:    func() *handlers.PlaybackHandler { return playbackHandler },
		GetStatusHandler:      func() *handlers.StatusHandler { return statusHandler },

		// HandlersReady is used by requireInitialized middleware to check if
		// handlers have been initialized. Returns true only if all runtime
		// handlers are non-nil (i.e., initializeHandlers() has been called).
		HandlersReady: func() bool {
			return mediaHandler != nil && mediaDetailHandler != nil && mappingsHandler != nil && pairingHandler != nil &&
				playbackHandler != nil && statusHandler != nil
		},
	}

	// Create router
	r := router.New(deps)

	// Configure CSRF protection
	// Exempt API and WebSocket routes (they use JSON/WebSocket, not forms)
	csrfOptions := []csrf.Option{
		csrf.Secure(cfg.RequireTLS),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Warn("CSRF validation failed",
				"path", r.URL.Path,
				"method", r.Method,
				"remote_addr", r.RemoteAddr,
				"origin", r.Header.Get("Origin"),
				"host", r.Host,
				"reason", csrf.FailureReason(r))

			// Render proper error page
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			if err := pages.Error(http.StatusForbidden, "CSRF token validation failed. Please try again.").Render(r.Context(), w); err != nil {
				logger.Error("Failed to render CSRF error page", "error", err)
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
			}
		})),
	}

	csrfMiddleware := csrf.Protect(cfg.CSRFKey, csrfOptions...)

	if cfg.DevMode {
		logger.Info("CSRF protection: allowing localhost cross-origin requests in dev mode")
	}

	// Wrap with middleware chain
	// 1. CSRF protection (validates tokens on POST/PUT/PATCH/DELETE)
	// 2. Metrics middleware (tracks request metrics)
	// 3. Request ID middleware (generates unique request ID)
	// 4. User ID middleware (extracts user ID from session)
	// 5. Request logger middleware (creates scoped logger with request metadata)
	// 6. Request logging middleware (logs all requests)
	// 7. Setup middleware (checks config for all non-exempted routes)
	handler := csrfExemptMiddleware(csrfMiddleware, cfg.DevMode)(
		middleware.MetricsMiddleware()(
			middleware.WithRequestID()(
				middleware.WithUserID(sessionStore)(
					middleware.WithRequestLogger()(
						middleware.RequestLogger()(
							middleware.SetupMiddleware("./config.yml", sessionStore)(r),
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
