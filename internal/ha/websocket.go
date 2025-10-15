// Package ha provides Home Assistant integration for TapeDeck.
package ha

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/gorilla/websocket"
)

// HAClient manages a WebSocket connection to Home Assistant
type HAClient struct { //nolint:revive // HAClient name is intentional for clarity
	url            string
	token          string
	conn           *websocket.Conn
	done           chan struct{}
	mu             sync.Mutex
	tagCallback    func(tagID string)
	reconnectDelay time.Duration
}

// NewHAClient creates a new Home Assistant WebSocket client
func NewHAClient(haURL, token string) *HAClient {
	// Convert HTTP(S) URL to WS(S) URL
	wsURL := strings.Replace(haURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/api/websocket"

	return &HAClient{
		url:            wsURL,
		token:          token,
		done:           make(chan struct{}),
		reconnectDelay: constants.HAWebSocketReconnectDelay,
	}
}

// OnTagScanned registers a callback for tag_scanned events
func (c *HAClient) OnTagScanned(callback func(tagID string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tagCallback = callback
}

// Connect establishes a WebSocket connection to Home Assistant
func (c *HAClient) Connect() error {
	// Dial WebSocket
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = constants.HAWebSocketHandshakeTimeout

	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Home Assistant: %w", err)
	}

	// Read auth_required message
	var authRequired map[string]interface{}
	if err := conn.ReadJSON(&authRequired); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("failed to read auth_required: %w", err)
	}

	if msgType, ok := authRequired["type"].(string); !ok || msgType != "auth_required" {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("expected auth_required, got %v", authRequired["type"])
	}

	// Send auth message
	authMsg := map[string]string{
		"type":         "auth",
		"access_token": c.token,
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("failed to send auth: %w", err)
	}

	// Read auth response
	var authResponse map[string]interface{}
	if err := conn.ReadJSON(&authResponse); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("failed to read auth response: %w", err)
	}

	msgType, ok := authResponse["type"].(string)
	if !ok {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("auth response missing type field")
	}

	if msgType == "auth_invalid" {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("authentication failed: invalid token")
	}

	if msgType != "auth_ok" {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("unexpected auth response: %s", msgType)
	}

	// Subscribe to tag_scanned events
	subscribeMsg := map[string]interface{}{
		"id":         1,
		"type":       "subscribe_events",
		"event_type": "tag_scanned",
	}
	if err := conn.WriteJSON(subscribeMsg); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Read subscription result
	var result map[string]interface{}
	if err := conn.ReadJSON(&result); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("failed to read subscription result: %w", err)
	}

	if resultType, ok := result["type"].(string); !ok || resultType != "result" {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("expected result, got %v", result["type"])
	}

	if success, ok := result["success"].(bool); !ok || !success {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection: %v", closeErr)
		}
		return fmt.Errorf("subscription failed")
	}

	// Only set c.conn after successful auth and subscription
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Println("Connected to Home Assistant WebSocket")

	// Start message handler
	go c.handleMessages()

	return nil
}

// handleMessages reads and processes messages from Home Assistant
func (c *HAClient) handleMessages() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			if err := c.conn.Close(); err != nil {
				log.Printf("Failed to close WebSocket: %v", err)
			}
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				return
			}

			var msg map[string]interface{}
			if err := conn.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			// Handle event messages
			if msgType, ok := msg["type"].(string); ok && msgType == "event" {
				c.handleEvent(msg)
			}
		}
	}
}

// handleEvent processes event messages from Home Assistant
func (c *HAClient) handleEvent(msg map[string]interface{}) {
	event, ok := msg["event"].(map[string]interface{})
	if !ok {
		return
	}

	eventType, ok := event["event_type"].(string)
	if !ok || eventType != "tag_scanned" {
		return
	}

	data, ok := event["data"].(map[string]interface{})
	if !ok {
		return
	}

	tagID, ok := data["tag_id"].(string)
	if !ok {
		return
	}

	log.Printf("Tag scanned: %s", tagID)

	// Call callback
	c.mu.Lock()
	callback := c.tagCallback
	c.mu.Unlock()

	if callback != nil {
		callback(tagID)
	}
}

// IsConnected returns true if the WebSocket connection is active
func (c *HAClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if connection exists and done channel is not closed
	if c.conn == nil {
		return false
	}

	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// Reconnect attempts to reconnect to Home Assistant with a new token
func (c *HAClient) Reconnect(_ context.Context, newToken string) error {
	// Close existing connection if any
	c.mu.Lock()
	oldTokenPreview := "empty"
	if len(c.token) > 8 {
		oldTokenPreview = c.token[:8] + "..."
	} else if len(c.token) > 0 {
		oldTokenPreview = c.token + "..."
	}

	newTokenPreview := "empty"
	if len(newToken) > 8 {
		newTokenPreview = newToken[:8] + "..."
	} else if len(newToken) > 0 {
		newTokenPreview = newToken + "..."
	}

	log.Printf("Reconnect: Old token: %s (len=%d), New token: %s (len=%d), Same=%v",
		oldTokenPreview, len(c.token), newTokenPreview, len(newToken), c.token == newToken)

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			log.Printf("Failed to close existing connection: %v", err)
		}
		c.conn = nil
	}
	// Update token
	c.token = newToken
	c.mu.Unlock()

	// Attempt new connection
	return c.Connect()
}

// Close closes the WebSocket connection
func (c *HAClient) Close() {
	select {
	case <-c.done:
		// Already closed
		return
	default:
		close(c.done)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(constants.HAWebSocketCloseTimeout)); err != nil {
			log.Printf("Failed to write close message: %v", err)
		}
		if err := c.conn.Close(); err != nil {
			log.Printf("Failed to close WebSocket: %v", err)
		}
		c.conn = nil
	}

	log.Println("Closed Home Assistant WebSocket connection")
}
