package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
)

// HAStatusInterface defines the interface for checking Home Assistant connection
type HAStatusInterface interface {
	IsConnected() bool
	Reconnect(newToken string) error
}

// StatusHandler handles status check requests
type StatusHandler struct {
	haClient   HAStatusInterface
	configPath string
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(haClient HAStatusInterface, configPath string) *StatusHandler {
	return &StatusHandler{
		haClient:   haClient,
		configPath: configPath,
	}
}

// HAStatus handles GET /api/status/ha
func (h *StatusHandler) HAStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	// Reload runtime configuration from config.yml to get updated HA token
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		log.Printf("Failed to reload runtime config: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Failed to reload configuration",
		})
		return
	}

	haToken := runtimeCfg.HomeAssistant.Token

	// Log token info (first 8 chars for verification without exposing full token)
	tokenPreview := "empty"
	if len(haToken) > 8 {
		tokenPreview = haToken[:8] + "..."
	} else if len(haToken) > 0 {
		tokenPreview = haToken + "..."
	}
	log.Printf("Attempting reconnection with token: %s (length: %d)", tokenPreview, len(haToken))

	// Attempt reconnection with new token
	err = h.haClient.Reconnect(haToken)
	if err != nil {
		log.Printf("Failed to reconnect to Home Assistant: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	log.Println("Successfully reconnected to Home Assistant")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Reconnected successfully",
	})
}
