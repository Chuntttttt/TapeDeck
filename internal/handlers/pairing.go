package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
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
	GetEntityState(entityID string) (string, error)
	TurnOn(entityID string) error
	PlayMedia(entityID, contentType, contentID string) error
}

// PairingHandler handles NFC pairing requests
type PairingHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	haClient      HAClientInterface
	haRest        HARestInterface
	appleTVEntity string // Legacy: kept for backward compatibility
	plexServerID  string // Legacy: kept for backward compatibility
	configPath    string // Path to runtime config
	upgrader      websocket.Upgrader
	clients       map[*pairingClient]bool
	clientsMu     sync.Mutex
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
	haRest HARestInterface,
	appleTVEntity string,
	plexServerID string,
	configPath string,
) *PairingHandler {
	handler := &PairingHandler{
		sessionStore:  store,
		db:            database,
		haClient:      haClient,
		haRest:        haRest,
		appleTVEntity: appleTVEntity,
		plexServerID:  plexServerID,
		configPath:    configPath,
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
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Load runtime config to get Apple TVs
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
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
	if err := pages.PairingForm(appleTVOptions, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// WebSocketPairing handles WebSocket connection for pairing
func (h *PairingHandler) WebSocketPairing(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
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
	log.Printf("WebSocket client connected (userID=%d)", userID)

	// Start goroutines
	go client.writePump()
	go h.readPump(client)
}

// readPump handles incoming messages from browser client
func (h *PairingHandler) readPump(client *pairingClient) {
	defer func() {
		h.unregisterClient(client)
		if err := client.conn.Close(); err != nil {
			log.Printf("WebSocket close error: %v", err)
		}
	}()

	if err := client.conn.SetReadDeadline(time.Now().Add(constants.WebSocketReadTimeout)); err != nil {
		log.Printf("Failed to set read deadline: %v", err)
	}
	client.conn.SetPongHandler(func(string) error {
		if err := client.conn.SetReadDeadline(time.Now().Add(constants.WebSocketReadTimeout)); err != nil {
			log.Printf("Failed to set read deadline in pong handler: %v", err)
		}
		return nil
	})

	for {
		var msg map[string]interface{}
		if err := client.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
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

			log.Printf("Client ready for pairing (userID=%d, media=%s, server=%s, appleTV=%s)",
				client.userID, mediaTitle, serverID, appleTVEntity)
		}
	}
}

// writePump handles outgoing messages to browser client
func (client *pairingClient) writePump() {
	ticker := time.NewTicker(constants.WebSocketPingInterval)
	defer func() {
		ticker.Stop()
		if err := client.conn.Close(); err != nil {
			log.Printf("WebSocket close error: %v", err)
		}
	}()

	for {
		select {
		case message, ok := <-client.send:
			if err := client.conn.SetWriteDeadline(time.Now().Add(constants.WebSocketWriteTimeout)); err != nil {
				log.Printf("Failed to set write deadline: %v", err)
			}
			if !ok {
				if err := client.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("Failed to write close message: %v", err)
				}
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := client.conn.SetWriteDeadline(time.Now().Add(constants.WebSocketWriteTimeout)); err != nil {
				log.Printf("Failed to set write deadline: %v", err)
			}
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleTagScanned is called when a tag is scanned in Home Assistant
func (h *PairingHandler) handleTagScanned(tagID string) {
	log.Printf("Tag scanned from HA: %s", tagID)

	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	// Check if any pairing clients are active
	if len(h.clients) == 0 {
		// NO PAIRING CLIENTS: Look up mapping and play media
		h.playMedia(tagID)
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
		existing, err := h.db.GetCardMappingByTagID(tagID)
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
		mappingID, err := h.db.CreateCardMapping(mapping)
		if err != nil {
			log.Printf("Failed to create mapping: %v", err)
			h.sendError(client, "Failed to create mapping")
			continue
		}

		log.Printf("Created mapping ID=%d for tag=%s (server=%s, appleTV=%s)",
			mappingID, tagID, client.plexServerID, client.appleTVEntity)

		// Send success message
		successMsg := map[string]string{
			"type":        "mapping_created",
			"tag_id":      tagID,
			"media_title": client.mediaTitle,
		}
		h.sendJSON(client, successMsg)
	}
}

// playMedia looks up a mapping and triggers playback via Home Assistant
func (h *PairingHandler) playMedia(tagID string) {
	// Look up mapping
	mapping, err := h.db.GetCardMappingByTagID(tagID)
	if err != nil {
		log.Printf("No mapping found for tag %s: %v", tagID, err)
		return
	}

	// Build Plex deep link
	plexURL := fmt.Sprintf(
		"plex://play/?metadataKey=/library/metadata/%s&server=%s",
		mapping.MediaID,
		h.plexServerID,
	)

	// Call Home Assistant to play media
	if h.haRest == nil {
		log.Printf("HA REST client not configured, cannot play media")
		return
	}

	// Check Apple TV state
	state, err := h.haRest.GetEntityState(h.appleTVEntity)
	if err != nil {
		log.Printf("Failed to get Apple TV state: %v (continuing anyway)", err)
		// Continue with playback attempt even if state check fails
	} else {
		log.Printf("Apple TV state: %s", state)

		// Turn on Apple TV if it's off or in standby
		if state == "off" || state == "standby" {
			log.Printf("Apple TV is %s, turning on...", state)
			err = h.haRest.TurnOn(h.appleTVEntity)
			if err != nil {
				log.Printf("Failed to turn on Apple TV: %v", err)
				return
			}

			// Wait for Apple TV to wake up
			log.Printf("Waiting for Apple TV to wake up...")
			time.Sleep(constants.AppleTVWakeTime)
		}
	}

	// Play media
	err = h.haRest.PlayMedia(h.appleTVEntity, "url", plexURL)
	if err != nil {
		log.Printf("Failed to play media for tag %s: %v", tagID, err)
		return
	}

	log.Printf("Playing %s on %s", mapping.MediaTitle, h.appleTVEntity)

	// Create playback log
	playbackLog := models.NewPlaybackLog(mapping.UserID, mapping.TagID, mapping.MediaID, mapping.MediaTitle)
	_, err = h.db.CreatePlaybackLog(playbackLog)
	if err != nil {
		log.Printf("Failed to create playback log: %v", err)
	}
}

// sendJSON sends a JSON message to a client
func (h *PairingHandler) sendJSON(client *pairingClient, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal JSON: %v", err)
		return
	}

	select {
	case client.send <- jsonData:
	default:
		h.unregisterClient(client)
		if err := client.conn.Close(); err != nil {
			log.Printf("WebSocket close error: %v", err)
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
		log.Printf("WebSocket client disconnected (userID=%d)", client.userID)
	}
}
