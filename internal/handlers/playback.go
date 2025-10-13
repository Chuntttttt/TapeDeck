// Package handlers provides HTTP request handlers for the TapeDeck application.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/models"
)

// PlaybackHandler handles playback requests from Home Assistant
type PlaybackHandler struct {
	db       *db.DB
	serverID string
}

// NewPlaybackHandler creates a new PlaybackHandler
func NewPlaybackHandler(database *db.DB, serverID string) *PlaybackHandler {
	return &PlaybackHandler{
		db:       database,
		serverID: serverID,
	}
}

// PlayRequest represents the JSON request body for the play endpoint
type PlayRequest struct {
	TagID string `json:"tag_id"`
}

// PlayResponse represents the JSON response for successful playback requests
type PlayResponse struct {
	Success    bool   `json:"success"`
	TagID      string `json:"tag_id"`
	MediaTitle string `json:"media_title"`
	MediaType  string `json:"media_type"`
	MediaID    string `json:"media_id"`
	PlexKey    string `json:"plex_key"`
	ServerID   string `json:"server_id"`
}

// PlayErrorResponse represents the JSON response for failed playback requests
type PlayErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// Play handles POST /api/play requests from Home Assistant
func (h *PlaybackHandler) Play(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(PlayErrorResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// Parse request body
	var req PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(PlayErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid JSON: %v", err),
		})
		return
	}

	// Validate tag_id is present
	if req.TagID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(PlayErrorResponse{
			Success: false,
			Error:   "tag_id is required",
		})
		return
	}

	// Look up card mapping by tag_id
	mapping, err := h.db.GetCardMappingByTagID(req.TagID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(PlayErrorResponse{
			Success: false,
			Error:   "Tag not found",
		})
		return
	}

	// Create playback log
	log := models.NewPlaybackLog(mapping.UserID, mapping.TagID, mapping.MediaID, mapping.MediaTitle)
	_, err = h.db.CreatePlaybackLog(log)
	if err != nil {
		// Log error but don't fail the request
		// In production, you might want to use a proper logger here
		fmt.Printf("Warning: Failed to create playback log: %v\n", err)
	}

	// Build Plex key from media_id (rating key)
	plexKey := fmt.Sprintf("/library/metadata/%s", mapping.MediaID)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(PlayResponse{
		Success:    true,
		TagID:      mapping.TagID,
		MediaTitle: mapping.MediaTitle,
		MediaType:  mapping.MediaType,
		MediaID:    mapping.MediaID,
		PlexKey:    plexKey,
		ServerID:   h.serverID,
	})
}
