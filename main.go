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
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/internal/router"
)

func main() {
	// Try to load runtime configuration from config.yml
	runtimeCfg, err := config.LoadRuntimeConfig("./config.yml")
	needsSetup := false
	if err != nil || runtimeCfg.IsEmpty() {
		log.Printf("Runtime configuration not found or empty: %v", err)
		log.Println("Setup wizard will be required before using the application")
		needsSetup = true
	} else if err := runtimeCfg.Validate(); err != nil {
		log.Printf("Runtime configuration validation failed: %v", err)
		log.Println("Setup wizard will be required before using the application")
		needsSetup = true
	}

	// Load legacy env-based config for backward compatibility
	// (will be deprecated once all config is in config.yml)
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: Failed to load env config: %v", err)
		// Use defaults for basic settings
		cfg = &config.Config{
			Port:          "8080",
			DatabasePath:  "./tapedeck.db",
			SessionSecret: "change-me-in-production",
			LogLevel:      "info",
			DevMode:       false,
		}
	}

	// Set up structured logging to both stdout and file
	logFile, err := os.OpenFile("tapedeck.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: Failed to open log file: %v", err)
		logger.Init(cfg.LogLevel, os.Stdout)
	} else {
		defer func() { _ = logFile.Close() }()
		logger.Init(cfg.LogLevel, os.Stdout, logFile)
	}

	// Also set standard log output for backward compatibility
	if logFile != nil {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
	}

	logger.Info("Starting TapeDeck", "log_level", cfg.LogLevel, "dev_mode", cfg.DevMode, "needs_setup", needsSetup)

	// Initialize database
	database, err := db.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err) //nolint:gocritic // exitAfterDefer: acceptable for fatal errors
	}
	defer func() {
		_ = database.Close()
	}()

	// Run migrations
	if err := database.RunMigrations("./migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Database initialized successfully")

	// Initialize session store
	sessionStore := middleware.NewSessionStore([]byte(cfg.SessionSecret))

	// Initialize Plex auth client (needed for both normal operation and setup)
	plexAuth := plex.NewAuthClient("https://plex.tv", "tapedeck-client-id", "TapeDeck", cfg.DevMode)
	if cfg.DevMode {
		log.Println("⚠️  DEV_MODE enabled: TLS verification disabled")
	}

	// Initialize auth handler (needed for both normal operation and setup)
	authHandler := handlers.NewAuthHandler(sessionStore, plexAuth, database)

	// Declare handler variables that will be initialized either at startup or after setup
	var mediaHandler *handlers.MediaHandler
	var mappingsHandler *handlers.MappingsHandler
	var playbackHandler *handlers.PlaybackHandler
	var pairingHandler *handlers.PairingHandler
	var statusHandler *handlers.StatusHandler
	var settingsHandler *handlers.SettingsHandler
	var haClient *ha.HAClient

	// Define handler initialization callback for setup wizard and settings
	initializeHandlers := func() error {
		log.Println("Initializing handlers after setup completion...")

		// Close existing HA client if it exists
		if haClient != nil {
			log.Println("Closing existing Home Assistant connection...")
			haClient.Close()
		}

		// Reload config
		runtimeCfg, err := config.LoadRuntimeConfig("./config.yml")
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Build list of servers with their best connections
		var servers []handlers.ServerInfo
		var plexURL string      // For legacy handlers that need a single URL
		var plexServerID string // For playback handler

		for _, srv := range runtimeCfg.PlexServers {
			// TODO: Skip shared servers for now - they return 401 Unauthorized
			// See README.md "Known Limitations: Shared Plex Servers"
			if srv.Owner == "Shared" {
				log.Printf("Skipping shared server '%s' (not currently supported)", srv.Name)
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
		mappingsHandler = handlers.NewMappingsHandler(sessionStore, database, servers, cfg.DevMode)
		playbackHandler = handlers.NewPlaybackHandler(database, plexServerID)

		// Initialize Home Assistant WebSocket client
		haClient = ha.NewHAClient(runtimeCfg.HomeAssistant.URL, runtimeCfg.HomeAssistant.Token)
		if err := haClient.Connect(); err != nil {
			log.Printf("Warning: Failed to connect to Home Assistant: %v", err)
		}

		// Initialize Home Assistant REST client
		haRest := ha.NewRestClient(runtimeCfg.HomeAssistant.URL, runtimeCfg.HomeAssistant.Token, cfg.DevMode)

		// Get default Apple TV entity
		var defaultAppleTV string
		for _, tv := range runtimeCfg.AppleTVs {
			if tv.Default {
				defaultAppleTV = tv.Entity
				break
			}
		}
		if defaultAppleTV == "" && len(runtimeCfg.AppleTVs) > 0 {
			defaultAppleTV = runtimeCfg.AppleTVs[0].Entity
		}

		// Initialize pairing handler
		pairingHandler = handlers.NewPairingHandler(
			sessionStore,
			database,
			haClient,
			haRest,
			defaultAppleTV,
			plexServerID,
			"./config.yml",
		)

		// Initialize status handler
		statusHandler = handlers.NewStatusHandler(haClient)

		log.Println("All handlers initialized successfully")
		return nil
	}

	// Initialize setup handler (always available)
	setupHandler := handlers.NewSetupHandler(sessionStore, "./config.yml", plexAuth, database, cfg.DevMode, initializeHandlers)

	// Initialize settings handler (always available) - uses same reload callback as setup
	settingsHandler = handlers.NewSettingsHandler(sessionStore, "./config.yml", initializeHandlers)

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
		log.Println("Config not ready - setup wizard will initialize handlers after completion")
	}

	// Create router dependencies
	deps := &router.RouterDependencies{
		AuthHandler:     authHandler,
		SetupHandler:    setupHandler,
		SettingsHandler: settingsHandler,
		MappingsHandler: mappingsHandler,
		MediaHandler:    mediaHandler,
		PairingHandler:  pairingHandler,
		PlaybackHandler: playbackHandler,
		StatusHandler:   statusHandler,
		AuthMiddleware:  middleware.RequireAuth(sessionStore),
		HandlersReady: func() bool {
			return mediaHandler != nil && mappingsHandler != nil && pairingHandler != nil &&
				playbackHandler != nil && statusHandler != nil
		},
	}

	// Create router
	r := router.New(deps, "./config.yml")

	// Wrap with middleware chain
	// 1. Metrics middleware (tracks request metrics)
	// 2. Request logging middleware (logs all requests)
	// 3. Setup middleware (checks config for all non-exempted routes)
	handler := middleware.MetricsMiddleware()(
		middleware.RequestLogger()(
			middleware.SetupMiddleware("./config.yml", sessionStore)(r),
		),
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting TapeDeck on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
