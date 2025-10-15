package handlers

import (
	"log"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/sessions"
)

// SettingsHandler handles settings/admin requests
type SettingsHandler struct {
	sessionStore   *sessions.CookieStore
	configPath     string
	reloadHandlers func() error
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(store *sessions.CookieStore, configPath string, reloadHandlers func() error) *SettingsHandler {
	return &SettingsHandler{
		sessionStore:   store,
		configPath:     configPath,
		reloadHandlers: reloadHandlers,
	}
}

// Settings handles GET /settings
func (h *SettingsHandler) Settings(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Load current config
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		RespondError(w, r, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Render using templ template
	if err := pages.Settings(runtimeCfg.PlexServers, runtimeCfg.HomeAssistant.URL, runtimeCfg.HomeAssistant.Token, runtimeCfg.AppleTVs, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// SaveSettings handles POST /settings/servers
func (h *SettingsHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
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
		RespondError(w, r, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Update Home Assistant settings
	haURL := r.FormValue("ha_url")
	haToken := r.FormValue("ha_token")
	haChanged := false
	if haURL != "" && haToken != "" {
		if haURL != runtimeCfg.HomeAssistant.URL || haToken != runtimeCfg.HomeAssistant.Token {
			runtimeCfg.HomeAssistant.URL = haURL
			runtimeCfg.HomeAssistant.Token = haToken
			haChanged = true
			log.Printf("Updated Home Assistant settings: %s", haURL)
		}
	}

	// Filter servers to only include selected ones
	var filteredServers []config.PlexServer
	serversChanged := false
	for _, srv := range runtimeCfg.PlexServers {
		if selectedMap[srv.ID] {
			filteredServers = append(filteredServers, srv)
		} else {
			log.Printf("Removing server '%s' from configuration", srv.Name)
			serversChanged = true
		}
	}

	runtimeCfg.PlexServers = filteredServers

	// Save updated config
	if err := runtimeCfg.Save(h.configPath); err != nil {
		log.Printf("Failed to save configuration: %v", err)
		RespondError(w, r, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Printf("Configuration updated: %d server(s) enabled", len(filteredServers))

	// Reload handlers if HA settings or servers changed
	if (haChanged || serversChanged) && h.reloadHandlers != nil {
		log.Println("Reloading handlers with new configuration...")
		if err := h.reloadHandlers(); err != nil {
			log.Printf("Warning: Failed to reload handlers: %v", err)
			// Don't fail the request - config was saved successfully
		}
	}

	// Redirect back to settings
	http.Redirect(w, r, "/settings?saved=true", http.StatusSeeOther)
}
