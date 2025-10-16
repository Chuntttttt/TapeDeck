package router

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Dependencies contains all handler dependencies needed by the router
type Dependencies struct {
	AuthHandler     *handlers.AuthHandler
	SetupHandler    *handlers.SetupHandler
	SettingsHandler *handlers.SettingsHandler

	// Handler getters (return current handler values, may change after initialization)
	GetMappingsHandler    func() *handlers.MappingsHandler
	GetMediaHandler       func() *handlers.MediaHandler
	GetMediaDetailHandler func() *handlers.MediaDetailHandler
	GetPairingHandler     func() *handlers.PairingHandler
	GetPlaybackHandler    func() *handlers.PlaybackHandler
	GetStatusHandler      func() *handlers.StatusHandler

	AuthMiddleware func(http.Handler) http.Handler
	HandlersReady  func() bool
}

// New creates the main application router
func New(deps *Dependencies) *chi.Mux {
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
		r.Use(deps.AuthMiddleware)

		// Media routes
		r.Get("/libraries", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMediaHandler().Libraries(w, req)
		})
		r.Get("/libraries/{libraryKey}", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMediaHandler().LibraryContents(w, req)
		})
		r.Get("/media/{serverID}/{ratingKey}", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMediaDetailHandler().Detail(w, req)
		})

		// Mappings routes
		r.Get("/mappings", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().Dashboard(w, req)
		})
		r.Get("/mappings/new", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().NewMappingForm(w, req)
		})
		r.Post("/mappings", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().CreateMapping(w, req)
		})
		r.Get("/mappings/{id}/edit", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().EditMappingForm(w, req)
		})
		r.Post("/mappings/{id}", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().UpdateMapping(w, req)
		})
		r.Post("/mappings/{id}/delete", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().DeleteMapping(w, req)
		})
		r.Get("/mappings/pair", func(w http.ResponseWriter, req *http.Request) {
			deps.GetPairingHandler().PairForm(w, req)
		})

		// WebSocket routes
		r.Get("/ws/pairing", func(w http.ResponseWriter, req *http.Request) {
			deps.GetPairingHandler().WebSocketPairing(w, req)
		})

		// API routes (no auth middleware - exempt in csrfExemptMiddleware)
		r.Get("/api/search", func(w http.ResponseWriter, req *http.Request) {
			deps.GetMappingsHandler().SearchJSON(w, req)
		})
		r.Post("/api/play", func(w http.ResponseWriter, req *http.Request) {
			deps.GetPlaybackHandler().Play(w, req)
		})
		r.Get("/api/status/ha", func(w http.ResponseWriter, req *http.Request) {
			deps.GetStatusHandler().HAStatus(w, req)
		})
	})

	// Static files
	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// Metrics endpoint (unprotected)
	r.Handle("/metrics", promhttp.Handler())

	// Health check (unprotected, no setup middleware)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			// Health check is already failed at this point (can't write response)
			// Log the error but there's nothing we can do
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
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
