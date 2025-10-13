package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
)

// HAClientInterface defines the interface for Home Assistant WebSocket client
type HAClientInterface interface {
	OnTagScanned(callback func(tagID string))
}

// PairingHandler handles NFC pairing requests
type PairingHandler struct {
	sessionStore *sessions.CookieStore
	db           *db.DB
	haClient     HAClientInterface
	upgrader     websocket.Upgrader
	clients      map[*pairingClient]bool
	clientsMu    sync.Mutex
}

// pairingClient represents a browser WebSocket client in pairing mode
type pairingClient struct {
	conn       *websocket.Conn
	send       chan []byte
	userID     int64
	mediaID    string
	mediaType  string
	mediaTitle string
}

// NewPairingHandler creates a new pairing handler
func NewPairingHandler(store *sessions.CookieStore, database *db.DB, haClient HAClientInterface) *PairingHandler {
	handler := &PairingHandler{
		sessionStore: store,
		db:           database,
		haClient:     haClient,
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

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>NFC Pairing Mode - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; background: #f5f5f5; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .step { margin-bottom: 30px; }
        .step-number { display: inline-block; width: 30px; height: 30px; background: #e5a00d; color: white; border-radius: 50%; text-align: center; line-height: 30px; font-weight: bold; margin-right: 10px; }
        .search-container { position: relative; margin-top: 10px; }
        .search-results { position: absolute; top: 100%; left: 0; right: 0; background: white; border: 1px solid #ddd; border-top: none; max-height: 300px; overflow-y: auto; display: none; z-index: 10; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .search-result-item { padding: 12px; border-bottom: 1px solid #f0f0f0; cursor: pointer; }
        .search-result-item:hover { background: #f5f5f5; }
        .result-title { font-weight: bold; margin-bottom: 3px; }
        .result-meta { font-size: 14px; color: #666; }
        .selected-media { padding: 15px; background: #f0f9ff; border: 2px solid #0284c7; border-radius: 4px; margin-top: 10px; display: none; }
        .selected-media-title { font-weight: bold; font-size: 18px; margin-bottom: 5px; color: #0c4a6e; }
        .selected-media-meta { font-size: 14px; color: #075985; }
        input[type="text"] { padding: 12px; width: 100%; font-size: 16px; border: 2px solid #ddd; border-radius: 4px; box-sizing: border-box; }
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
        .loading { display: inline-block; width: 20px; height: 20px; border: 3px solid #f3f3f3; border-top: 3px solid #e5a00d; border-radius: 50%; animation: spin 1s linear infinite; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
        .ws-status { font-size: 12px; color: #666; margin-top: 10px; }
        .ws-status.connected { color: #22c55e; }
        .ws-status.disconnected { color: #ef4444; }
    </style>
</head>
<body>
    <a href="/mappings" class="back-link">← Back to Mappings</a>
    <div class="container">
        <h1>NFC Pairing Mode</h1>

        <div class="step">
            <span class="step-number">1</span>
            <strong>Search for media to pair</strong>
            <div class="search-container">
                <input type="text" id="media_search" placeholder="Start typing to search..." autocomplete="off">
                <div class="search-results" id="searchResults"></div>
            </div>
            <div class="selected-media" id="selectedMedia">
                <div class="selected-media-title" id="selectedTitle"></div>
                <div class="selected-media-meta" id="selectedMeta"></div>
            </div>
        </div>

        <div class="step">
            <span class="step-number">2</span>
            <strong>Start pairing mode</strong>
            <div style="margin-top: 10px;">
                <button id="startPairingBtn" class="btn" disabled>Start Pairing Mode</button>
            </div>
            <div class="ws-status" id="wsStatus">Connecting...</div>
        </div>

        <div class="step">
            <span class="step-number">3</span>
            <strong>Tap your NFC card</strong>
        </div>

        <div class="status" id="status">
            <div class="status-icon" id="statusIcon"></div>
            <div class="status-text" id="statusText"></div>
            <div class="status-detail" id="statusDetail"></div>
        </div>
    </div>

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
                const response = await fetch('/api/search?q=' + encodeURIComponent(query));

                if (!response.ok) {
                    throw new Error('Search failed');
                }

                const data = await response.json();

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
                    item.innerHTML = '<div class="result-title">' + result.title + '</div><div class="result-meta">' + result.type + yearStr + '</div>';
                    item.dataset.title = result.title;
                    item.dataset.year = result.year || '';
                    item.dataset.type = result.type;
                    item.dataset.ratingKey = result.ratingKey;

                    item.addEventListener('click', function() {
                        selectMedia(this.dataset.title, this.dataset.type, this.dataset.ratingKey, this.dataset.year);
                    });

                    searchResults.appendChild(item);
                });

                searchResults.style.display = 'block';
            } catch (error) {
                console.error('Search error:', error);
            }
        }

        function selectMedia(title, type, ratingKey, year) {
            selectedItem = { title, type, ratingKey, year };

            selectedTitle.textContent = title;
            selectedMeta.textContent = type + (year ? ' (' + year + ')' : '');
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

            startPairingBtn.disabled = true;

            const msg = {
                type: 'start_pairing',
                media_id: selectedItem.ratingKey,
                media_title: selectedItem.title,
                media_type: selectedItem.type
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
</body>
</html>`)
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

			if !okID || !okTitle || !okType {
				h.sendError(client, "Missing required fields")
				continue
			}

			// Store pairing info
			client.mediaID = mediaID
			client.mediaTitle = mediaTitle
			client.mediaType = mediaType

			log.Printf("Client ready for pairing (userID=%d, media=%s)", client.userID, mediaTitle)
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

	// Broadcast to all pairing clients
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

		// Create mapping
		mapping := models.NewCardMapping(client.userID, tagID, client.mediaType, client.mediaID, client.mediaTitle)
		mappingID, err := h.db.CreateCardMapping(mapping)
		if err != nil {
			log.Printf("Failed to create mapping: %v", err)
			h.sendError(client, "Failed to create mapping")
			continue
		}

		log.Printf("Created mapping ID=%d for tag=%s", mappingID, tagID)

		// Send success message
		successMsg := map[string]string{
			"type":        "mapping_created",
			"tag_id":      tagID,
			"media_title": client.mediaTitle,
		}
		h.sendJSON(client, successMsg)
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
		log.Printf("WebSocket client disconnected (userID=%d)", client.userID)
	}
}
