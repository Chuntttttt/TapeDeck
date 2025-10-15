package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
)

// HAStatusInterface defines the interface for checking Home Assistant connection
type HAStatusInterface interface {
	IsConnected() bool
	Reconnect(ctx context.Context, newToken string) error
}

// StatusHandler handles status check requests
type StatusHandler struct {
	haClient   HAStatusInterface
	configPath string
	db         *db.DB
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(haClient HAStatusInterface, configPath string, database *db.DB) *StatusHandler {
	return &StatusHandler{
		haClient:   haClient,
		configPath: configPath,
		db:         database,
	}
}

// HAStatus handles GET /api/status/ha
func (h *StatusHandler) HAStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		RespondError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	connected := false
	if h.haClient != nil {
		connected = h.haClient.IsConnected()
	}

	response := map[string]interface{}{
		"connected": connected,
		"service":   "home_assistant",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// HAReconnect handles POST /api/status/ha/reconnect
func (h *StatusHandler) HAReconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	if r.Method != http.MethodPost {
		RespondError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.haClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Home Assistant client not configured",
		})
		return
	}

	// Load HA token from database (encrypted)
	settings, err := h.db.GetSettings(ctx)
	if err != nil {
		log.Error("Failed to load HA token from database", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Failed to load Home Assistant token",
		})
		return
	}

	haToken := settings.HAToken

	// Log token info (first 8 chars for verification without exposing full token)
	tokenPreview := "empty"
	if len(haToken) > 8 {
		tokenPreview = haToken[:8] + "..."
	} else if len(haToken) > 0 {
		tokenPreview = haToken + "..."
	}
	log.Info("Attempting reconnection", "token_preview", tokenPreview, "token_length", len(haToken))

	// Attempt reconnection with new token
	err = h.haClient.Reconnect(ctx, haToken)
	if err != nil {
		log.Error("Failed to reconnect to Home Assistant", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	log.Info("Successfully reconnected to Home Assistant")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Reconnected successfully",
	})
}
