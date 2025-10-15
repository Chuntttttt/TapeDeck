package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sync"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/services"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
)

// HAClientInterface defines the interface for Home Assistant WebSocket client
type HAClientInterface interface {
	OnTagScanned(callback func(tagID string))
	IsConnected() bool
}

// HARestInterface defines the interface for Home Assistant REST client
type HARestInterface interface {
	GetEntityState(ctx context.Context, entityID string) (string, error)
	TurnOn(ctx context.Context, entityID string) error
	PlayMedia(ctx context.Context, entityID, contentType, contentID string) error
}

// PairingHandler handles NFC pairing requests
type PairingHandler struct {
	sessionStore    *sessions.CookieStore
	db              *db.DB
	haClient        HAClientInterface
	playbackService *services.PlaybackService
	configPath      string // Path to runtime config
	upgrader        websocket.Upgrader
	clients         map[*pairingClient]bool
	clientsMu       sync.Mutex
}

// pairingClient represents a browser WebSocket client in pairing mode
type pairingClient struct {
	conn          *websocket.Conn
	send          chan []byte
	userID        int64
	mediaID       string
	mediaType     string
	mediaTitle    string
	plexServerID  string // Selected Plex server ID
	appleTVEntity string // Selected Apple TV entity
}

// NewPairingHandler creates a new pairing handler
func NewPairingHandler(
	store *sessions.CookieStore,
	database *db.DB,
	haClient HAClientInterface,
	playbackService *services.PlaybackService,
	configPath string,
) *PairingHandler {
	handler := &PairingHandler{
		sessionStore:    store,
		db:              database,
		haClient:        haClient,
		playbackService: playbackService,
		configPath:      configPath,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true // Allow all origins for now (same-origin in production)
			},
		},
		clients: make(map[*pairingClient]bool),
	}

	// Register HA tag callback
	if haClient != nil {
		haClient.OnTagScanned(handler.handleTagScanned)
	}

	return handler
}

// PairForm handles GET /mappings/pair
func (h *PairingHandler) PairForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)

	// Get user from context
	_, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Load runtime config to get Apple TVs
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		RespondError(w, r, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Convert Apple TVs to pages.AppleTVOption
	var appleTVOptions []pages.AppleTVOption
	for _, tv := range runtimeCfg.AppleTVs {
		appleTVOptions = append(appleTVOptions, pages.AppleTVOption{
			Entity:  tv.Entity,
			Name:    tv.Name,
			Default: tv.Default,
		})
	}

	// Render using templ template
	if err := pages.PairingForm(appleTVOptions, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		logger.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// WebSocketPairing handles WebSocket connection for pairing
func (h *PairingHandler) WebSocketPairing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqLogger := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		reqLogger.Error("Failed to upgrade WebSocket", "error", err)
		return
	}

	// Create client
	client := &pairingClient{
		conn:   conn,
		send:   make(chan []byte, constants.WebSocketSendBufferSize),
		userID: userID,
	}

	// Register client
	h.clientsMu.Lock()
	h.clients[client] = true
	h.clientsMu.Unlock()

	middleware.IncrementWebSocketConnections()
	logger.Info("WebSocket client connected", "user_id", userID)

	// Start goroutines
	go client.writePump()
	go h.readPump(client)
}

// readPump handles incoming messages from browser client
func (h *PairingHandler) readPump(client *pairingClient) {
	defer func() {
		h.unregisterClient(client)
		if err := client.conn.Close(); err != nil {
			logger.Warn("WebSocket close error", "error", err)
		}
	}()

	if err := client.conn.SetReadDeadline(time.Now().Add(constants.WebSocketReadTimeout)); err != nil {
		logger.Warn("Failed to set read deadline", "error", err)
	}
	client.conn.SetPongHandler(func(string) error {
		if err := client.conn.SetReadDeadline(time.Now().Add(constants.WebSocketReadTimeout)); err != nil {
			logger.Warn("Failed to set read deadline in pong handler", "error", err)
		}
		return nil
	})

	for {
		var msg map[string]interface{}
		if err := client.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn("WebSocket error", "error", err)
			}
			break
		}

		// Handle start_pairing message
		if msgType, ok := msg["type"].(string); ok && msgType == "start_pairing" {
			mediaID, okID := msg["media_id"].(string)
			mediaTitle, okTitle := msg["media_title"].(string)
			mediaType, okType := msg["media_type"].(string)
			serverID, okServerID := msg["server_id"].(string)
			appleTVEntity, okAppleTV := msg["apple_tv_entity"].(string)

			if !okID || !okTitle || !okType || !okServerID || !okAppleTV {
				h.sendError(client, "Missing required fields")
				continue
			}

			// Store pairing info
			client.mediaID = mediaID
			client.mediaTitle = mediaTitle
			client.mediaType = mediaType
			client.plexServerID = serverID
			client.appleTVEntity = appleTVEntity

			logger.Info("Client ready for pairing", "user_id", client.userID, "media", mediaTitle, "server", serverID, "apple_tv", appleTVEntity)
		}
	}
}

// writePump handles outgoing messages to browser client
func (client *pairingClient) writePump() {
	ticker := time.NewTicker(constants.WebSocketPingInterval)
	defer func() {
		ticker.Stop()
		if err := client.conn.Close(); err != nil {
			logger.Warn("WebSocket close error", "error", err)
		}
	}()

	for {
		select {
		case message, ok := <-client.send:
			if err := client.conn.SetWriteDeadline(time.Now().Add(constants.WebSocketWriteTimeout)); err != nil {
				logger.Warn("Failed to set write deadline", "error", err)
			}
			if !ok {
				if err := client.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					logger.Warn("Failed to write close message", "error", err)
				}
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := client.conn.SetWriteDeadline(time.Now().Add(constants.WebSocketWriteTimeout)); err != nil {
				logger.Warn("Failed to set write deadline", "error", err)
			}
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleTagScanned is called when a tag is scanned in Home Assistant
func (h *PairingHandler) handleTagScanned(tagID string) {
	ctx := context.Background()
	logger.Info("Tag scanned from HA", "tag_id", tagID)

	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	// Check if any pairing clients are active
	if len(h.clients) == 0 {
		// NO PAIRING CLIENTS: Look up mapping and play media
		h.playMedia(ctx, tagID)
		return
	}

	// PAIRING MODE: Broadcast to all pairing clients
	for client := range h.clients {
		// Send tag_detected message
		tagDetectedMsg := map[string]string{
			"type":   "tag_detected",
			"tag_id": tagID,
		}
		h.sendJSON(client, tagDetectedMsg)

		// Check if client has pairing info
		if client.mediaID == "" {
			continue
		}

		// Check if tag already exists
		existing, err := h.db.GetCardMappingByTagID(ctx, tagID)
		if err == nil && existing != nil {
			// Tag already mapped
			errorMsg := fmt.Sprintf("Tag already mapped to '%s'", html.EscapeString(existing.MediaTitle))
			h.sendError(client, errorMsg)
			continue
		}

		// Create mapping using selected server and Apple TV
		mapping := models.NewCardMapping(
			client.userID,
			tagID,
			client.mediaType,
			client.mediaID,
			client.mediaTitle,
			client.plexServerID,
			client.appleTVEntity,
		)
		mappingID, err := h.db.CreateCardMapping(ctx, mapping)
		if err != nil {
			logger.Error("Failed to create mapping", "error", err)
			h.sendError(client, "Failed to create mapping")
			continue
		}

		logger.Info("Created mapping", "mapping_id", mappingID, "tag_id", tagID, "server", client.plexServerID, "apple_tv", client.appleTVEntity)

		// Send success message
		successMsg := map[string]string{
			"type":        "mapping_created",
			"tag_id":      tagID,
			"media_title": client.mediaTitle,
		}
		h.sendJSON(client, successMsg)
	}
}

// playMedia looks up a mapping and triggers playback via PlaybackService
func (h *PairingHandler) playMedia(ctx context.Context, tagID string) {
	result, err := h.playbackService.PlayByTagID(ctx, tagID)
	if err != nil {
		logger.Error("Playback failed", "tag_id", tagID, "error", err)
		return
	}

	if !result.Success {
		logger.Error("Playback unsuccessful", "tag_id", tagID, "error", result.Error)
		return
	}

	logger.Info("Playback completed",
		"tag_id", tagID,
		"media", result.MediaTitle,
		"woke_device", result.WokeDevice)
}

// sendJSON sends a JSON message to a client
func (h *PairingHandler) sendJSON(client *pairingClient, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal JSON", "error", err)
		return
	}

	select {
	case client.send <- jsonData:
	default:
		h.unregisterClient(client)
		if err := client.conn.Close(); err != nil {
			logger.Warn("WebSocket close error", "error", err)
		}
	}
}

// sendError sends an error message to a client
func (h *PairingHandler) sendError(client *pairingClient, message string) {
	errorMsg := map[string]string{
		"type":    "error",
		"message": message,
	}
	h.sendJSON(client, errorMsg)
}

// unregisterClient removes a client from the registry
func (h *PairingHandler) unregisterClient(client *pairingClient) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		middleware.DecrementWebSocketConnections()
		logger.Info("WebSocket client disconnected", "user_id", client.userID)
	}
}
