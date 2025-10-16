package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/internal/services"
	"github.com/gorilla/sessions"
)

// PlayHandler handles direct play requests (not via NFC)
type PlayHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	servers       []ServerInfo
	appleTVs      []config.AppleTV
	haURL         string
	haToken       string
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewPlayHandler creates a new play handler
func NewPlayHandler(store *sessions.CookieStore, database *db.DB, servers []ServerInfo, appleTVs []config.AppleTV, haURL string, haToken string, devMode bool) *PlayHandler {
	return &PlayHandler{
		sessionStore: store,
		db:           database,
		servers:      servers,
		appleTVs:     appleTVs,
		haURL:        haURL,
		haToken:      haToken,
		devMode:      devMode,
		newPlexClient: func(serverURL, serverID, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, serverID, authToken, devMode)
		},
	}
}

// PlayByRatingKey handles POST /api/play - Play media by server ID and rating key
func (h *PlayHandler) PlayByRatingKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	var req struct {
		ServerID  string `json:"server_id"`
		RatingKey string `json:"rating_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Not authenticated",
		})
		return
	}

	// Get user to retrieve auth token
	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("Failed to get user", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get user",
		})
		return
	}

	// Find server
	var serverURL string
	for _, srv := range h.servers {
		if srv.ID == req.ServerID {
			if len(srv.URLs) > 0 {
				serverURL = srv.URLs[0]
			}
			break
		}
	}

	if serverURL == "" {
		log.Error("Server not found", "server_id", req.ServerID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Server not found",
		})
		return
	}

	// Fetch metadata to get title
	plexClient := h.newPlexClient(serverURL, req.ServerID, user.PlexAuthToken, h.devMode)
	apiCtx, cancel := context.WithTimeout(ctx, constants.PlexAPITimeout)
	metadata, err := plexClient.GetMetadata(apiCtx, req.RatingKey)
	cancel()

	if err != nil {
		log.Error("Failed to fetch metadata", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Media not found",
		})
		return
	}

	// Find default Apple TV
	var defaultAppleTV string
	for _, tv := range h.appleTVs {
		if tv.Default {
			defaultAppleTV = tv.Entity
			break
		}
	}

	if defaultAppleTV == "" {
		log.Error("No default Apple TV configured")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No default Apple TV configured",
		})
		return
	}

	// Create playback service
	haClient := ha.NewRestClient(h.haURL, h.haToken, h.devMode)
	playbackService := services.NewPlaybackService(h.db, haClient)

	// Build playback request
	playbackReq := &services.PlaybackRequest{
		TagID:         "", // No tag ID for direct play
		UserID:        userID,
		MediaID:       req.RatingKey,
		MediaTitle:    metadata.Title,
		PlexServerID:  req.ServerID,
		AppleTVEntity: defaultAppleTV,
	}

	// Play media
	result, err := playbackService.Play(ctx, playbackReq)
	if err != nil {
		log.Error("Failed to play media", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to play media: %s", err.Error()),
		})
		return
	}

	if !result.Success {
		log.Error("Playback failed", "error", result.Error)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Playback failed: %s", result.Error.Error()),
		})
		return
	}

	log.Info("Media playback started", "title", metadata.Title, "apple_tv", defaultAppleTV)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Playing %s", metadata.Title),
	})
}
