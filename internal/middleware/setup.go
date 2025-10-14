package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/gorilla/sessions"
)

// SetupMiddleware checks if the application has been configured
// and redirects to the setup wizard if not
func SetupMiddleware(configPath string, sessionStore *sessions.CookieStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow setup, auth, and static routes without check
			path := r.URL.Path
			if strings.HasPrefix(path, "/setup") ||
				strings.HasPrefix(path, "/auth") ||
				strings.HasPrefix(path, "/static") ||
				path == "/favicon.ico" {
				next.ServeHTTP(w, r)
				return
			}

			// Load and validate runtime config
			cfg, err := config.LoadRuntimeConfig(configPath)
			if err != nil {
				log.Printf("Failed to load config: %v, redirecting to setup", err)
				redirectToSetup(w, r, sessionStore)
				return
			}

			// Check if config is empty
			if cfg.IsEmpty() {
				log.Printf("Config is empty, redirecting to setup")
				redirectToSetup(w, r, sessionStore)
				return
			}

			// Validate config
			if err := cfg.Validate(); err != nil {
				log.Printf("Config validation failed: %v, redirecting to setup", err)
				redirectToSetup(w, r, sessionStore)
				return
			}

			// Config is valid, proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// redirectToSetup redirects to the appropriate setup wizard page
// If user is authenticated, skip to Plex server selection
// Otherwise, start at welcome page
func redirectToSetup(w http.ResponseWriter, r *http.Request, sessionStore *sessions.CookieStore) {
	session, _ := sessionStore.Get(r, SessionName)

	// Check if user is authenticated
	if _, ok := GetUserID(session); ok {
		// User is authenticated, go directly to Plex server selection
		log.Printf("User authenticated but config incomplete, redirecting to /setup/plex")
		http.Redirect(w, r, "/setup/plex", http.StatusSeeOther)
		return
	}

	// User not authenticated, start at welcome page
	log.Printf("User not authenticated and config incomplete, redirecting to /setup")
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}
