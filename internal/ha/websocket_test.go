package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewHAClient(t *testing.T) {
	client := NewHAClient("http://test.local:8123", "test-token")
	if client == nil {
		t.Fatal("NewHAClient() returned nil")
	}

	if client.url != "ws://test.local:8123/api/websocket" {
		t.Errorf("url = %s, want ws://test.local:8123/api/websocket", client.url)
	}

	if client.token != "test-token" {
		t.Errorf("token = %s, want test-token", client.token)
	}
}

func TestNewHAClient_HTTPStoWSS(t *testing.T) {
	client := NewHAClient("https://test.local:8123", "test-token")
	if client == nil {
		t.Fatal("NewHAClient() returned nil")
	}

	if client.url != "wss://test.local:8123/api/websocket" {
		t.Errorf("url = %s, want wss://test.local:8123/api/websocket", client.url)
	}
}

func TestHAClient_Connect_Success(t *testing.T) {
	// Create mock HA server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send auth_required
		authRequired := map[string]string{"type": "auth_required"}
		if err := conn.WriteJSON(authRequired); err != nil {
			return
		}

		// Read auth message
		var authMsg map[string]interface{}
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}

		// Verify auth message
		if authMsg["type"] != "auth" {
			t.Errorf("Expected auth type, got %v", authMsg["type"])
			return
		}

		// Send auth_ok
		authOK := map[string]string{"type": "auth_ok"}
		if err := conn.WriteJSON(authOK); err != nil {
			return
		}

		// Read subscribe message
		var subMsg map[string]interface{}
		if err := conn.ReadJSON(&subMsg); err != nil {
			return
		}

		// Verify subscribe message
		if subMsg["type"] != "subscribe_events" {
			t.Errorf("Expected subscribe_events type, got %v", subMsg["type"])
			return
		}
		if subMsg["event_type"] != "tag_scanned" {
			t.Errorf("Expected tag_scanned event_type, got %v", subMsg["event_type"])
			return
		}

		// Send result message
		result := map[string]interface{}{
			"id":      1,
			"type":    "result",
			"success": true,
		}
		if err := conn.WriteJSON(result); err != nil {
			return
		}

		// Keep connection open
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Create client with test server URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &HAClient{
		url:   wsURL,
		token: "test-token",
		done:  make(chan struct{}),
	}

	// Connect
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Clean up
	client.Close()
}

func TestHAClient_Connect_AuthFailed(t *testing.T) {
	// Create mock HA server that rejects auth
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send auth_required
		authRequired := map[string]string{"type": "auth_required"}
		if err := conn.WriteJSON(authRequired); err != nil {
			return
		}

		// Read auth message
		var authMsg map[string]interface{}
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}

		// Send auth_invalid
		authInvalid := map[string]string{
			"type":    "auth_invalid",
			"message": "Invalid access token",
		}
		if err := conn.WriteJSON(authInvalid); err != nil {
			return
		}
	}))
	defer server.Close()

	// Create client with test server URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &HAClient{
		url:   wsURL,
		token: "bad-token",
		done:  make(chan struct{}),
	}

	// Connect should fail
	err := client.Connect()
	if err == nil {
		t.Fatal("Connect() should have failed with invalid token")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("Error message = %v, want 'authentication failed'", err)
	}
}

func TestHAClient_OnTagScanned(t *testing.T) {
	// Create mock HA server
	upgrader := websocket.Upgrader{}
	tagScannedChan := make(chan bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send auth_required
		authRequired := map[string]string{"type": "auth_required"}
		if err := conn.WriteJSON(authRequired); err != nil {
			return
		}

		// Read auth message
		var authMsg map[string]interface{}
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}

		// Send auth_ok
		authOK := map[string]string{"type": "auth_ok"}
		if err := conn.WriteJSON(authOK); err != nil {
			return
		}

		// Read subscribe message
		var subMsg map[string]interface{}
		if err := conn.ReadJSON(&subMsg); err != nil {
			return
		}

		// Send result message
		result := map[string]interface{}{
			"id":      1,
			"type":    "result",
			"success": true,
		}
		if err := conn.WriteJSON(result); err != nil {
			return
		}

		// Wait for signal to send tag event
		<-tagScannedChan

		// Send tag_scanned event
		event := map[string]interface{}{
			"id":   1,
			"type": "event",
			"event": map[string]interface{}{
				"event_type": "tag_scanned",
				"data": map[string]string{
					"tag_id":    "04-16-5C-D4-2E-61-80",
					"device_id": "esphome_nfc_reader",
				},
			},
		}
		if err := conn.WriteJSON(event); err != nil {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Create client with test server URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &HAClient{
		url:   wsURL,
		token: "test-token",
		done:  make(chan struct{}),
	}

	// Setup callback
	called := false
	var receivedTagID string
	client.OnTagScanned(func(tagID string) {
		called = true
		receivedTagID = tagID
	})

	// Connect
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	// Trigger tag event
	tagScannedChan <- true

	// Wait for callback
	time.Sleep(200 * time.Millisecond)

	if !called {
		t.Error("OnTagScanned callback was not called")
	}

	if receivedTagID != "04-16-5C-D4-2E-61-80" {
		t.Errorf("tagID = %s, want 04-16-5C-D4-2E-61-80", receivedTagID)
	}
}

func TestHAClient_Close(t *testing.T) {
	// Create mock HA server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send auth_required
		authRequired := map[string]string{"type": "auth_required"}
		if err := conn.WriteJSON(authRequired); err != nil {
			return
		}

		// Read auth message
		var authMsg map[string]interface{}
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}

		// Send auth_ok
		authOK := map[string]string{"type": "auth_ok"}
		if err := conn.WriteJSON(authOK); err != nil {
			return
		}

		// Read subscribe message
		var subMsg map[string]interface{}
		if err := conn.ReadJSON(&subMsg); err != nil {
			return
		}

		// Send result message
		result := map[string]interface{}{
			"id":      1,
			"type":    "result",
			"success": true,
		}
		if err := conn.WriteJSON(result); err != nil {
			return
		}

		// Wait for close
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Create client with test server URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &HAClient{
		url:   wsURL,
		token: "test-token",
		done:  make(chan struct{}),
	}

	// Connect
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Close should not error
	client.Close()

	// Calling Close again should be safe
	client.Close()
}

func TestHAMessageTypes(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantType string
	}{
		{
			name:     "auth_required message",
			jsonData: `{"type":"auth_required"}`,
			wantType: "auth_required",
		},
		{
			name:     "auth_ok message",
			jsonData: `{"type":"auth_ok"}`,
			wantType: "auth_ok",
		},
		{
			name:     "auth_invalid message",
			jsonData: `{"type":"auth_invalid","message":"Invalid token"}`,
			wantType: "auth_invalid",
		},
		{
			name:     "event message",
			jsonData: `{"id":1,"type":"event","event":{"event_type":"tag_scanned","data":{"tag_id":"test"}}}`,
			wantType: "event",
		},
		{
			name:     "result message",
			jsonData: `{"id":1,"type":"result","success":true}`,
			wantType: "result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg map[string]interface{}
			if err := json.Unmarshal([]byte(tt.jsonData), &msg); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			msgType, ok := msg["type"].(string)
			if !ok {
				t.Fatal("type field is not a string")
			}

			if msgType != tt.wantType {
				t.Errorf("type = %s, want %s", msgType, tt.wantType)
			}
		})
	}
}
