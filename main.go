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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		log.Fatalf("Failed to initialize database: %v", err)
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

	mux := http.NewServeMux()

	// Auth routes (no setup middleware - always available)
	mux.HandleFunc("/auth/login", authHandler.Login)
	mux.HandleFunc("/auth/poll-status", authHandler.PollStatus)
	mux.HandleFunc("/auth/logout", authHandler.Logout)

	// Setup routes (no setup middleware - must be accessible during setup)
	mux.HandleFunc("/setup", setupHandler.Step1Welcome)
	mux.HandleFunc("/setup/plex", setupHandler.Step2Plex)
	mux.HandleFunc("/setup/plex/save", setupHandler.SavePlexServers)
	mux.HandleFunc("/setup/ha", setupHandler.Step3HomeAssistant)
	mux.HandleFunc("/setup/ha/test", setupHandler.TestHomeAssistant)
	mux.HandleFunc("/setup/ha/save", setupHandler.SaveHomeAssistant)
	mux.HandleFunc("/setup/appletv", setupHandler.Step4AppleTVs)
	mux.HandleFunc("/setup/appletv/save", setupHandler.SaveAppleTVs)
	mux.HandleFunc("/setup/complete", setupHandler.Step5Complete)
	mux.HandleFunc("/setup/finish", setupHandler.CompleteSetup)

	// Settings routes (require auth)
	mux.Handle("/settings", middleware.RequireAuth(sessionStore)(http.HandlerFunc(settingsHandler.Settings)))
	mux.Handle("/settings/servers", middleware.RequireAuth(sessionStore)(http.HandlerFunc(settingsHandler.SaveSettings)))

	// Metrics endpoint (unprotected)
	mux.Handle("/metrics", promhttp.Handler())

	// Health check (unprotected, no setup middleware)
	mux.HandleFunc("/health", healthCheckHandler().ServeHTTP)

	// Register all routes - setup middleware will redirect if config incomplete
	// For routes that need initialized handlers, we'll use pointers that get set at startup
	// This allows routes to exist but redirect appropriately if handlers aren't ready

	mux.Handle("/libraries", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mediaHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		mediaHandler.Libraries(w, r)
	})))

	mux.Handle("/libraries/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mediaHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		libraryContentsHandler(mediaHandler)(w, r)
	})))

	mux.Handle("/search", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mediaHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		mediaHandler.Search(w, r)
	})))

	mux.Handle("/mappings", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mappingsHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost {
			mappingsHandler.CreateMapping(w, r)
		} else {
			mappingsHandler.Dashboard(w, r)
		}
	})))

	mux.Handle("/mappings/new", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mappingsHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		mappingsHandler.NewMappingForm(w, r)
	})))

	mux.Handle("/mappings/pair", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pairingHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		pairingHandler.PairForm(w, r)
	})))

	mux.Handle("/mappings/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mappingsHandler == nil {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		mappingsRouteHandler(mappingsHandler)(w, r)
	})))

	mux.Handle("/api/search", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mappingsHandler == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		mappingsHandler.SearchJSON(w, r)
	})))

	mux.Handle("/ws/pairing", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pairingHandler == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		pairingHandler.WebSocketPairing(w, r)
	})))

	mux.HandleFunc("/api/status/ha", func(w http.ResponseWriter, r *http.Request) {
		if statusHandler == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		statusHandler.HAStatus(w, r)
	})

	mux.HandleFunc("/api/status/ha/reconnect", func(w http.ResponseWriter, r *http.Request) {
		if statusHandler == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		statusHandler.HAReconnect(w, r)
	})

	mux.HandleFunc("/api/play", func(w http.ResponseWriter, r *http.Request) {
		if playbackHandler == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		playbackHandler.Play(w, r)
	})

	// Home route - redirect to libraries
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/libraries", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// Wrap mux with middleware chain
	// 1. Metrics middleware (tracks request metrics)
	// 2. Request logging middleware (logs all requests)
	// 3. Setup middleware (checks config for all non-exempted routes)
	handler := middleware.MetricsMiddleware()(
		middleware.RequestLogger()(
			middleware.SetupMiddleware("./config.yml", sessionStore)(mux),
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

// libraryContentsHandler extracts the library key from the URL path and calls LibraryContents
func libraryContentsHandler(h *handlers.MediaHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract library key from /libraries/{key}
		path := r.URL.Path
		if len(path) <= len("/libraries/") {
			http.Error(w, "Library ID required", http.StatusBadRequest)
			return
		}
		libraryKey := path[len("/libraries/"):]
		if libraryKey == "" {
			http.Error(w, "Library ID required", http.StatusBadRequest)
			return
		}
		h.LibraryContents(w, r, libraryKey)
	}
}

// mappingsRouteHandler extracts the mapping ID from the URL path and routes to appropriate handler
func mappingsRouteHandler(h *handlers.MappingsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract path after /mappings/
		path := r.URL.Path
		if len(path) <= len("/mappings/") {
			http.Error(w, "Mapping ID required", http.StatusBadRequest)
			return
		}

		remainder := path[len("/mappings/"):]

		// Parse the ID and action
		var mappingID int64
		var action string

		// Check for /{id}/edit or /{id}/delete patterns
		if n, err := fmt.Sscanf(remainder, "%d/edit", &mappingID); n == 1 && err == nil {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.EditMappingForm(w, r, mappingID)
			return
		}

		if n, err := fmt.Sscanf(remainder, "%d/delete", &mappingID); n == 1 && err == nil {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.DeleteMapping(w, r, mappingID)
			return
		}

		// Check for /{id} pattern (update)
		if n, err := fmt.Sscanf(remainder, "%d%s", &mappingID, &action); n == 1 && err == nil {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h.UpdateMapping(w, r, mappingID)
			return
		}

		http.Error(w, "Invalid mapping route", http.StatusBadRequest)
	}
}

func healthCheckHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})
}
