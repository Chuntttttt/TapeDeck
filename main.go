// Package main provides the TapeDeck HTTP server for NFC-based media playback.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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
	plexAuth := plex.NewAuthClient("https://plex.tv", "tapedeck-client-id", "TapeDeck")

	// Initialize auth handler
	authHandler := handlers.NewAuthHandler(sessionStore, plexAuth, database)

	mux := http.NewServeMux()

	// Auth routes
	mux.HandleFunc("/auth/login", authHandler.Login)
	mux.HandleFunc("/auth/callback", authHandler.Callback)
	mux.HandleFunc("/auth/logout", authHandler.Logout)

	// Protected routes
	mux.Handle("/", middleware.RequireAuth(sessionStore)(homeHandler()))

	// Health check (unprotected)
	mux.HandleFunc("/health", healthCheckHandler().ServeHTTP)

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

func healthCheckHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})
}

func homeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>TapeDeck</title>
</head>
<body>
    <h1>🎬 TapeDeck</h1>
    <p>Welcome to TapeDeck!</p>
    <form method="post" action="/auth/logout">
        <button type="submit">Logout</button>
    </form>
</body>
</html>`)
	})
}
