package router

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouterDependencies contains all handler dependencies needed by the router
type RouterDependencies struct {
	AuthHandler     *handlers.AuthHandler
	SetupHandler    *handlers.SetupHandler
	SettingsHandler *handlers.SettingsHandler
	MappingsHandler *handlers.MappingsHandler
	MediaHandler    *handlers.MediaHandler
	PairingHandler  *handlers.PairingHandler
	PlaybackHandler *handlers.PlaybackHandler
	StatusHandler   *handlers.StatusHandler

	AuthMiddleware func(http.Handler) http.Handler
	HandlersReady  func() bool
}

// New creates the main application router
func New(deps *RouterDependencies, configPath string) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)

	// Auth routes (no setup middleware - always available)
	r.Mount("/auth", authRouter(deps.AuthHandler))

	// Setup routes (no setup middleware - must be accessible during setup)
	r.Mount("/setup", setupRouter(deps.SetupHandler))

	// Settings routes (require auth)
	r.Route("/settings", func(r chi.Router) {
		r.Use(deps.AuthMiddleware)
		r.Get("/", deps.SettingsHandler.Settings)
		r.Post("/servers", deps.SettingsHandler.SaveSettings)
	})

	// Application routes that require initialized handlers
	//
	// These routes are protected by both SetupMiddleware (in main.go) and requireInitialized
	// middleware (below). This provides defense-in-depth:
	//
	// 1. SetupMiddleware redirects to /setup if config.yml is missing/invalid
	// 2. requireInitialized checks that handlers have been initialized
	// 3. Handlers are initialized synchronously in main.go's initializeHandlers()
	//
	// This means handlers are guaranteed to be non-nil when these routes are accessed:
	// - On startup: config exists → initializeHandlers() called → handlers ready
	// - After setup: setup wizard completes → initializeHandlers() called → handlers ready
	// - After settings update: settings saved → initializeHandlers() called → handlers ready
	//
	// Therefore, individual route handlers do not need to check for nil handlers.
	r.Group(func(r chi.Router) {
		r.Use(requireInitialized(deps.HandlersReady))

		// Media routes
		r.Mount("/libraries", mediaRouter(deps.MediaHandler, deps.AuthMiddleware))

		// Mappings routes
		r.Mount("/mappings", mappingsRouter(
			deps.MappingsHandler,
			deps.PairingHandler,
			deps.AuthMiddleware,
		))

		// API routes
		r.Mount("/api", apiRouter(
			deps.MappingsHandler,
			deps.PairingHandler,
			deps.PlaybackHandler,
			deps.StatusHandler,
			deps.AuthMiddleware,
		))

		// WebSocket routes
		r.Mount("/ws", wsRouter(deps.PairingHandler, deps.AuthMiddleware))
	})

	// Static files
	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// Metrics endpoint (unprotected)
	r.Handle("/metrics", promhttp.Handler())

	// Health check (unprotected, no setup middleware)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Home route - redirect to libraries
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/libraries", http.StatusFound)
	})

	return r
}

// requireInitialized middleware checks if handlers are ready
func requireInitialized(ready func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ready() {
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
