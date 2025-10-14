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
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
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
			CheckOrigin: func(r *http.Request) bool {
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

	// Build Apple TV options HTML
	appleTVOptionsHTML := ""
	if len(runtimeCfg.AppleTVs) == 0 {
		appleTVOptionsHTML = `<option value="">No Apple TVs configured</option>`
	} else {
		for _, tv := range runtimeCfg.AppleTVs {
			selected := ""
			if tv.Default {
				selected = " selected"
			}
			appleTVOptionsHTML += fmt.Sprintf(`<option value="%s"%s>%s</option>`,
				html.EscapeString(tv.Entity),
				selected,
				html.EscapeString(tv.Name))
		}
	}

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>NFC Pairing Mode - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; padding-top: 60px; background: #f5f5f5; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .step { margin-bottom: 30px; }
        .step-number { display: inline-block; width: 30px; height: 30px; background: #e5a00d; color: white; border-radius: 50%%; text-align: center; line-height: 30px; font-weight: bold; margin-right: 10px; }
        .search-container { position: relative; margin-top: 10px; }
        .search-results { position: absolute; top: 100%%; left: 0; right: 0; background: white; border: 1px solid #ddd; border-top: none; max-height: 300px; overflow-y: auto; display: none; z-index: 10; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .search-result-item { padding: 12px; border-bottom: 1px solid #f0f0f0; cursor: pointer; }
        .search-result-item:hover { background: #f5f5f5; }
        .result-title { font-weight: bold; margin-bottom: 3px; }
        .result-meta { font-size: 14px; color: #666; }
        .result-server { font-size: 12px; color: #1976d2; margin-top: 3px; font-weight: 500; }
        .selected-media { padding: 15px; background: #f0f9ff; border: 2px solid #0284c7; border-radius: 4px; margin-top: 10px; display: none; }
        .selected-media-title { font-weight: bold; font-size: 18px; margin-bottom: 5px; color: #0c4a6e; }
        .selected-media-meta { font-size: 14px; color: #075985; }
        input[type="text"] { padding: 12px; width: 100%%; font-size: 16px; border: 2px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        input[type="text"]:focus { border-color: #e5a00d; outline: none; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; font-weight: bold; }
        .btn:hover { background: #cc8f0a; }
        .btn:disabled { background: #ccc; cursor: not-allowed; }
        .status { padding: 20px; border-radius: 8px; text-align: center; display: none; margin-top: 20px; }
        .status.waiting { background: #fef3c7; border: 2px solid #f59e0b; }
        .status.ready { background: #dcfce7; border: 2px solid #22c55e; }
        .status.error { background: #fee2e2; border: 2px solid #ef4444; }
        .status-icon { font-size: 48px; margin-bottom: 10px; }
        .status-text { font-size: 18px; font-weight: bold; margin-bottom: 5px; }
        .status-detail { font-size: 14px; color: #666; }
        .tag-id { font-family: monospace; font-size: 16px; background: #f5f5f5; padding: 4px 8px; border-radius: 4px; margin: 5px 0; }
        .loading { display: inline-block; width: 20px; height: 20px; border: 3px solid #f3f3f3; border-top: 3px solid #e5a00d; border-radius: 50%%; animation: spin 1s linear infinite; }
        @keyframes spin { 0%% { transform: rotate(0deg); } 100%% { transform: rotate(360deg); } }
        .ws-status { font-size: 12px; color: #666; margin-top: 10px; }
        .ws-status.connected { color: #22c55e; }
        .ws-status.disconnected { color: #ef4444; }
    </style>
</head>
<body>
%s
%s
    <div class="container">
        <h1>NFC Pairing Mode</h1>

        <div class="step">
            <span class="step-number">1</span>
            <strong>Search for media to pair</strong>
            <div class="search-container">
                <input type="text" id="media_search" placeholder="Start typing to search..." autocomplete="off">
                <div style="position: absolute; right: 12px; top: 12px; display: none;" id="searchSpinner">
                    <div class="loading"></div>
                </div>
                <div class="search-results" id="searchResults"></div>
            </div>
            <div class="selected-media" id="selectedMedia">
                <div class="selected-media-title" id="selectedTitle"></div>
                <div class="selected-media-meta" id="selectedMeta"></div>
            </div>
        </div>

        <div class="step">
            <span class="step-number">2</span>
            <strong>Select Apple TV</strong>
            <div style="margin-top: 10px;">
                <select id="appleTVSelect" style="padding: 12px; width: 100%%; font-size: 16px; border: 2px solid #ddd; border-radius: 4px; background: white;">
                    %s
                </select>
            </div>
        </div>

        <div class="step">
            <span class="step-number">3</span>
            <strong>Start pairing mode</strong>
            <div style="margin-top: 10px;">
                <button id="startPairingBtn" class="btn" disabled>Start Pairing Mode</button>
            </div>
            <div class="ws-status" id="wsStatus">Connecting...</div>
        </div>

        <div class="step">
            <span class="step-number">4</span>
            <strong>Tap your NFC card</strong>
        </div>

        <div class="status" id="status">
            <div class="status-icon" id="statusIcon"></div>
            <div class="status-text" id="statusText"></div>
            <div class="status-detail" id="statusDetail"></div>
        </div>
    </div>
`, NavigationHTML(), ConnectionBannerHTML(), appleTVOptionsHTML)

	_, _ = fmt.Fprintf(w, `
    <script>
        const searchInput = document.getElementById('media_search');
        const searchResults = document.getElementById('searchResults');
        const selectedMedia = document.getElementById('selectedMedia');
        const selectedTitle = document.getElementById('selectedTitle');
        const selectedMeta = document.getElementById('selectedMeta');
        const startPairingBtn = document.getElementById('startPairingBtn');
        const status = document.getElementById('status');
        const statusIcon = document.getElementById('statusIcon');
        const statusText = document.getElementById('statusText');
        const statusDetail = document.getElementById('statusDetail');
        const wsStatus = document.getElementById('wsStatus');
        const searchSpinner = document.getElementById('searchSpinner');

        let searchTimeout;
        let selectedItem = null;
        let ws = null;

        // Connect WebSocket
        function connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';

            // If we're on the Air proxy (port 3002), connect directly to app server (port 3001)
            // Air proxy doesn't handle WebSocket upgrades properly
            let host = window.location.host;
            if (host.includes(':3002')) {
                host = host.replace(':3002', ':3001');
            }

            const wsURL = protocol + '//' + host + '/ws/pairing';

            ws = new WebSocket(wsURL);

            ws.onopen = function() {
                console.log('WebSocket connected');
                wsStatus.textContent = 'Connected';
                wsStatus.classList.add('connected');
                wsStatus.classList.remove('disconnected');
            };

            ws.onclose = function() {
                console.log('WebSocket disconnected');
                wsStatus.textContent = 'Disconnected';
                wsStatus.classList.add('disconnected');
                wsStatus.classList.remove('connected');

                // Reconnect after 2 seconds
                setTimeout(connectWebSocket, 2000);
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
            };

            ws.onmessage = function(event) {
                const msg = JSON.parse(event.data);
                console.log('Received message:', msg);

                if (msg.type === 'tag_detected') {
                    showStatus('waiting', '⏳', 'Tag Detected!', 'Tag ID: ' + msg.tag_id);
                } else if (msg.type === 'mapping_created') {
                    showStatus('ready', '✓', 'Success!', 'Mapping created for "' + msg.media_title + '"');

                    // Reset after 3 seconds
                    setTimeout(() => {
                        window.location.href = '/mappings';
                    }, 3000);
                } else if (msg.type === 'error') {
                    showStatus('error', '✗', 'Error', msg.message);
                    startPairingBtn.disabled = false;
                }
            };
        }

        connectWebSocket();

        // Search functionality
        searchInput.addEventListener('input', function() {
            const query = this.value.trim();

            clearTimeout(searchTimeout);

            if (query.length < 2) {
                searchResults.style.display = 'none';
                searchResults.innerHTML = '';
                return;
            }

            searchTimeout = setTimeout(() => {
                performSearch(query);
            }, 300);
        });

        async function performSearch(query) {
            try {
                // Show loading spinner
                searchSpinner.style.display = 'block';
                searchResults.style.display = 'none';

                const response = await fetch('/api/search?q=' + encodeURIComponent(query));

                if (!response.ok) {
                    throw new Error('Search failed');
                }

                const data = await response.json();

                // Hide loading spinner
                searchSpinner.style.display = 'none';

                if (data.results.length === 0) {
                    searchResults.innerHTML = '<div class="search-result-item">No results found</div>';
                    searchResults.style.display = 'block';
                    return;
                }

                searchResults.innerHTML = '';

                data.results.forEach(result => {
                    const item = document.createElement('div');
                    item.className = 'search-result-item';
                    const yearStr = result.year ? ' (' + result.year + ')' : '';
                    const serverStr = result.serverName ? '<div class="result-server">📡 ' + result.serverName + '</div>' : '';
                    item.innerHTML = '<div class="result-title">' + result.title + '</div><div class="result-meta">' + result.type + yearStr + '</div>' + serverStr;
                    item.dataset.title = result.title;
                    item.dataset.year = result.year || '';
                    item.dataset.type = result.type;
                    item.dataset.ratingKey = result.ratingKey;
                    item.dataset.serverID = result.serverID || '';
                    item.dataset.serverName = result.serverName || '';

                    item.addEventListener('click', function() {
                        selectMedia(this.dataset.title, this.dataset.type, this.dataset.ratingKey, this.dataset.year, this.dataset.serverID, this.dataset.serverName);
                    });

                    searchResults.appendChild(item);
                });

                searchResults.style.display = 'block';
            } catch (error) {
                console.error('Search error:', error);
                // Hide loading spinner on error
                searchSpinner.style.display = 'none';
            }
        }

        function selectMedia(title, type, ratingKey, year, serverID, serverName) {
            selectedItem = { title, type, ratingKey, year, serverID, serverName };

            selectedTitle.textContent = title;
            const serverInfo = serverName ? ' • ' + serverName : '';
            selectedMeta.textContent = type + (year ? ' (' + year + ')' : '') + serverInfo;
            selectedMedia.style.display = 'block';

            searchResults.style.display = 'none';
            searchInput.value = title;

            startPairingBtn.disabled = false;
        }

        // Start pairing
        startPairingBtn.addEventListener('click', function() {
            if (!selectedItem || !ws || ws.readyState !== WebSocket.OPEN) {
                return;
            }

            const appleTVSelect = document.getElementById('appleTVSelect');
            const selectedAppleTV = appleTVSelect.value;

            if (!selectedAppleTV) {
                alert('Please select an Apple TV');
                return;
            }

            startPairingBtn.disabled = true;

            const msg = {
                type: 'start_pairing',
                media_id: selectedItem.ratingKey,
                media_title: selectedItem.title,
                media_type: selectedItem.type,
                server_id: selectedItem.serverID,
                apple_tv_entity: selectedAppleTV
            };

            ws.send(JSON.stringify(msg));

            showStatus('ready', '📱', 'Ready to Scan', 'Tap your NFC card on the reader');
        });

        function showStatus(className, icon, text, detail) {
            status.className = 'status ' + className;
            status.style.display = 'block';
            statusIcon.textContent = icon;
            statusText.textContent = text;
            statusDetail.textContent = detail;
        }

        // Hide search results when clicking outside
        document.addEventListener('click', function(e) {
            if (!searchInput.contains(e.target) && !searchResults.contains(e.target)) {
                searchResults.style.display = 'none';
            }
        });
    </script>
%s
</body>
</html>`, ConnectionBannerScript())
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
		send:   make(chan []byte, 256),
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
		_ = client.conn.Close()
	}()

	client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		_ = client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
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

			// Wait for Apple TV to wake up (5 seconds should be enough)
			log.Printf("Waiting 5 seconds for Apple TV to wake up...")
			time.Sleep(5 * time.Second)
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
	// Marshal to JSON (ignore error - will be caught in write)
	jsonData, _ := json.Marshal(data)

	select {
	case client.send <- jsonData:
	default:
		h.unregisterClient(client)
		_ = client.conn.Close()
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
