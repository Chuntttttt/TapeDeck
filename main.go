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
	"syscall"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
)

func main() {
	// Load configuration first (need LOG_LEVEL for logger init)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set up structured logging to both stdout and file
	logFile, err := os.OpenFile("tapedeck.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: Failed to open log file: %v", err)
		logger.Init(cfg.LogLevel, os.Stdout)
	} else {
		defer logFile.Close()
		logger.Init(cfg.LogLevel, os.Stdout, logFile)
	}

	// Also set standard log output for backward compatibility
	if logFile != nil {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
	}

	logger.Info("Starting TapeDeck", "log_level", cfg.LogLevel, "dev_mode", cfg.DevMode)

	// Initialize database
	database, err := db.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Run migrations (before setting up defer, so Fatalf can exit cleanly)
	if err := database.RunMigrations("./migrations"); err != nil {
		_ = database.Close()
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Now set up defer for normal shutdown path
	defer func() { _ = database.Close() }()

	log.Println("Database initialized successfully")

	// Initialize session store
	sessionStore := middleware.NewSessionStore([]byte(cfg.SessionSecret))

	// Initialize Plex auth client
	plexAuth := plex.NewAuthClient("https://plex.tv", "tapedeck-client-id", "TapeDeck", cfg.DevMode)
	if cfg.DevMode {
		log.Println("⚠️  DEV_MODE enabled: TLS verification disabled")
	}

	// Initialize auth handler
	authHandler := handlers.NewAuthHandler(sessionStore, plexAuth, database)

	// Initialize media handler
	mediaHandler := handlers.NewMediaHandler(sessionStore, database, cfg.PlexURL, cfg.DevMode)

	// Initialize mappings handler
	mappingsHandler := handlers.NewMappingsHandler(sessionStore, database, cfg.PlexURL, cfg.DevMode)

	// Initialize playback handler (for Home Assistant integration)
	playbackHandler := handlers.NewPlaybackHandler(database, cfg.PlexServerID)

	// Initialize Home Assistant WebSocket client
	haClient := ha.NewHAClient(cfg.HAURL, cfg.HAToken)
	if err := haClient.Connect(); err != nil {
		log.Printf("Warning: Failed to connect to Home Assistant: %v", err)
		log.Println("Pairing mode will not work until connection is established")
	}
	defer haClient.Close()

	// Initialize Home Assistant REST client
	haRest := ha.NewRestClient(cfg.HAURL, cfg.HAToken, cfg.DevMode)

	// Initialize pairing handler
	pairingHandler := handlers.NewPairingHandler(
		sessionStore,
		database,
		haClient,
		haRest,
		cfg.AppleTVEntity,
		cfg.PlexServerID,
	)

	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("/auth/login", authHandler.Login)
	mux.HandleFunc("/auth/poll-status", authHandler.PollStatus)
	mux.HandleFunc("/auth/logout", authHandler.Logout)

	// Protected media routes
	mux.Handle("/libraries", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mediaHandler.Libraries)))
	mux.Handle("/libraries/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(libraryContentsHandler(mediaHandler))))
	mux.Handle("/search", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mediaHandler.Search)))

	// Protected mappings routes
	mux.Handle("/mappings", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mappingsHandler.CreateMapping(w, r)
		} else {
			mappingsHandler.Dashboard(w, r)
		}
	})))
	mux.Handle("/mappings/new", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mappingsHandler.NewMappingForm)))
	mux.Handle("/mappings/pair", middleware.RequireAuth(sessionStore)(http.HandlerFunc(pairingHandler.PairForm)))
	mux.Handle("/mappings/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mappingsRouteHandler(mappingsHandler))))
	mux.Handle("/api/search", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mappingsHandler.SearchJSON)))

	// Protected pairing WebSocket route
	mux.Handle("/ws/pairing", middleware.RequireAuth(sessionStore)(http.HandlerFunc(pairingHandler.WebSocketPairing)))

	// Home route - redirect to libraries
	mux.Handle("/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/libraries", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})))

	// Health check (unprotected)
	mux.HandleFunc("/health", healthCheckHandler().ServeHTTP)

	// Playback API for Home Assistant (unprotected - HA server is trusted)
	mux.HandleFunc("/api/play", playbackHandler.Play)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
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
