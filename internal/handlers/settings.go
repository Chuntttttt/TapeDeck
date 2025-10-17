package handlers

import (
	"net/http"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	csrf "filippo.io/csrf/gorilla"
	"github.com/gorilla/sessions"
)

// SettingsHandler handles settings/admin requests
type SettingsHandler struct {
	sessionStore   *sessions.CookieStore
	configPath     string
	db             *db.DB
	reloadHandlers func() error
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(store *sessions.CookieStore, configPath string, database *db.DB, reloadHandlers func() error) *SettingsHandler {
	return &SettingsHandler{
		sessionStore:   store,
		configPath:     configPath,
		db:             database,
		reloadHandlers: reloadHandlers,
	}
}

// Settings handles GET /settings
func (h *SettingsHandler) Settings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	_, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Load current config
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		RespondError(w, r, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Load HA token from database
	settings, err := h.db.GetSettings(ctx)
	if err != nil {
		log.Error("Failed to load settings from database", "error", err)
		RespondError(w, r, "Failed to load Home Assistant token", http.StatusInternalServerError)
		return
	}

	// Render using templ template
	if err := pages.Settings(runtimeCfg.PlexServers, runtimeCfg.HomeAssistant.URL, settings.HAToken, runtimeCfg.AppleTVs, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript(), csrf.Token(r)).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// SaveSettings handles POST /settings/servers
func (h *SettingsHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	_, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		RespondError(w, r, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get selected server IDs
	selectedIDs := r.Form["servers"]
	selectedMap := make(map[string]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	// Load current config
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		RespondError(w, r, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Load HA token from database
	settings, err := h.db.GetSettings(ctx)
	if err != nil {
		log.Error("Failed to load settings from database", "error", err)
		RespondError(w, r, "Failed to load Home Assistant token", http.StatusInternalServerError)
		return
	}

	// Update Home Assistant settings
	haURL := r.FormValue("ha_url")
	haToken := r.FormValue("ha_token")
	haChanged := false
	if haURL != "" && haToken != "" {
		// Validate URL scheme (prevent SSRF attacks)
		validatedURL, err := ValidateHTTPURL(haURL)
		if err != nil {
			log.Warn("Invalid Home Assistant URL", "error", err)
			RespondError(w, r, "Invalid Home Assistant URL: "+err.Error(), http.StatusBadRequest)
			return
		}
		haURL = validatedURL

		// Check if URL changed (in config.yml)
		if haURL != runtimeCfg.HomeAssistant.URL {
			runtimeCfg.HomeAssistant.URL = haURL
			haChanged = true
			log.Info("Updated Home Assistant URL", "url", haURL)
		}

		// Check if token changed (in database)
		if haToken != settings.HAToken {
			settings.HAToken = haToken
			settings.UpdatedAt = time.Now()
			if err := h.db.SaveSettings(ctx, settings); err != nil {
				log.Error("Failed to save settings to database", "error", err)
				RespondError(w, r, "Failed to save Home Assistant token", http.StatusInternalServerError)
				return
			}
			haChanged = true
			log.Info("Updated Home Assistant token (encrypted in database)")
		}
	}

	// Filter servers to only include selected ones
	var filteredServers []config.PlexServer
	serversChanged := false
	for _, srv := range runtimeCfg.PlexServers {
		if selectedMap[srv.ID] {
			filteredServers = append(filteredServers, srv)
		} else {
			log.Info("Removing server from configuration", "server", srv.Name)
			serversChanged = true
		}
	}

	runtimeCfg.PlexServers = filteredServers

	// Save updated config
	if err := runtimeCfg.Save(h.configPath); err != nil {
		log.Error("Failed to save configuration", "error", err)
		RespondError(w, r, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Info("Configuration updated", "servers_enabled", len(filteredServers))

	// Reload handlers if HA settings or servers changed
	if (haChanged || serversChanged) && h.reloadHandlers != nil {
		log.Info("Reloading handlers with new configuration")
		if err := h.reloadHandlers(); err != nil {
			log.Warn("Failed to reload handlers", "error", err)
			// Don't fail the request - config was saved successfully
		}
	}

	// Redirect back to settings
	http.Redirect(w, r, "/settings?saved=true", http.StatusSeeOther)
}
