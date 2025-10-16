package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
)

// MediaDetailHandler handles media detail page requests
type MediaDetailHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	servers       []ServerInfo
	appleTVs      []config.AppleTV
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewMediaDetailHandler creates a new media detail handler
func NewMediaDetailHandler(store *sessions.CookieStore, database *db.DB, servers []ServerInfo, devMode bool, appleTVs []config.AppleTV) *MediaDetailHandler {
	return &MediaDetailHandler{
		sessionStore: store,
		db:           database,
		servers:      servers,
		appleTVs:     appleTVs,
		devMode:      devMode,
		newPlexClient: func(serverURL, serverID, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, serverID, authToken, devMode)
		},
	}
}

// Detail handles GET /media/{serverID}/{ratingKey}
func (h *MediaDetailHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	serverID := chi.URLParam(r, "serverID")
	ratingKey := chi.URLParam(r, "ratingKey")

	if serverID == "" || ratingKey == "" {
		RespondError(w, r, "Invalid request", http.StatusBadRequest)
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user to retrieve auth token
	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("Failed to get user", "error", err)
		RespondError(w, r, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Find server
	var serverURL string
	var serverName string
	for _, srv := range h.servers {
		if srv.ID == serverID {
			if len(srv.URLs) > 0 {
				serverURL = srv.URLs[0]
				serverName = srv.Name
			}
			break
		}
	}

	if serverURL == "" {
		log.Error("Server not found", "server_id", serverID)
		RespondError(w, r, "Server not found", http.StatusNotFound)
		return
	}

	// Fetch metadata
	plexClient := h.newPlexClient(serverURL, serverID, user.PlexAuthToken, h.devMode)
	apiCtx, cancel := context.WithTimeout(ctx, constants.PlexAPITimeout)
	metadata, err := plexClient.GetMetadata(apiCtx, ratingKey)
	cancel()

	if err != nil {
		log.Error("Failed to fetch metadata", "error", err, "server_id", serverID, "rating_key", ratingKey)
		RespondError(w, r, "Media not found", http.StatusNotFound)
		return
	}

	// Build full thumbnail URL
	var thumbnailURL string
	if metadata.Thumb != "" {
		thumbnailURL = fmt.Sprintf("%s%s?X-Plex-Token=%s", serverURL, metadata.Thumb, user.PlexAuthToken)
	}

	// Build Plex web URL
	plexWebURL := fmt.Sprintf("https://app.plex.tv/desktop/#!/server/%s/details?key=/library/metadata/%s", serverID, ratingKey)

	// Check if already mapped
	mappings, err := h.db.GetCardMappingsByUserID(ctx, userID)
	if err != nil {
		log.Warn("Failed to get mappings", "error", err)
		mappings = nil
	}

	var existingMapping *models.CardMapping
	for _, m := range mappings {
		if m.PlexServerID == serverID && m.MediaID == ratingKey {
			existingMapping = m
			break
		}
	}

	// Find default Apple TV
	var defaultAppleTV string
	for _, tv := range h.appleTVs {
		if tv.Default {
			defaultAppleTV = tv.Name
			break
		}
	}

	// Render using templ template
	if err := pages.MediaDetail(metadata.Title, metadata.Summary, metadata.Year, thumbnailURL, plexWebURL, serverID, ratingKey, serverName, defaultAppleTV, existingMapping != nil, h.appleTVs, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript(), csrf.Token(r)).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}
