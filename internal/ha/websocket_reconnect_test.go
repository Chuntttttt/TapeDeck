package ha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockWSServer creates a mock WebSocket server for testing
func mockWSServer(t *testing.T) (*httptest.Server, *int32, *sync.Mutex) {
	t.Helper()

	var connectionCount int32
	var mu sync.Mutex

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Failed to upgrade: %v", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("Failed to close connection: %v", err)
			}
		}()

		mu.Lock()
		connectionCount++
		mu.Unlock()

		// Send auth_required
		if err := conn.WriteJSON(map[string]string{"type": "auth_required"}); err != nil {
			return
		}

		// Read auth message
		var authMsg map[string]interface{}
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}

		// Send auth_ok
		if err := conn.WriteJSON(map[string]string{"type": "auth_ok"}); err != nil {
			return
		}

		// Read subscribe message
		var subscribeMsg map[string]interface{}
		if err := conn.ReadJSON(&subscribeMsg); err != nil {
			return
		}

		// Send subscription result
		if err := conn.WriteJSON(map[string]interface{}{
			"type":    "result",
			"success": true,
		}); err != nil {
			return
		}

		// Keep connection open for a bit
		time.Sleep(100 * time.Millisecond)
	}))

	return server, &connectionCount, &mu
}

func TestReconnect_SingleCall(t *testing.T) {
	server, connCount, mu := mockWSServer(t)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

	client := &HAClient{
		url:   wsURL,
		token: "token1",
		done:  make(chan struct{}),
	}

	// Initial connect
	if err := client.Connect(); err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}

	// Wait for connection to establish
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	initialCount := *connCount
	mu.Unlock()

	if initialCount != 1 {
		t.Errorf("Expected 1 connection, got %d", initialCount)
	}

	// Reconnect with new token
	if err := client.Reconnect(context.Background(), "token2"); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	// Wait for new connection
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	finalCount := *connCount
	mu.Unlock()

	// Should have exactly 2 connections total (initial + reconnect)
	if finalCount != 2 {
		t.Errorf("Expected 2 total connections, got %d", finalCount)
	}

	client.Close()
}

func TestReconnect_ConcurrentCalls(t *testing.T) {
	server, connCount, mu := mockWSServer(t)
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

	client := &HAClient{
		url:   wsURL,
		token: "token1",
		done:  make(chan struct{}),
	}

	// Initial connect
	if err := client.Connect(); err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	initialCount := *connCount
	mu.Unlock()

	if initialCount != 1 {
		t.Errorf("Expected 1 initial connection, got %d", initialCount)
	}

	// Launch 5 concurrent reconnect calls
	var wg sync.WaitGroup
	errors := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = client.Reconnect(context.Background(), "token2")
		}(i)
	}

	wg.Wait()

	// At least one should succeed
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("Expected at least one reconnect to succeed")
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	finalCount := *connCount
	mu.Unlock()

	// With proper mutex protection, we should have:
	// - 1 initial connection
	// - 5 reconnect attempts, but they should be serialized
	// - So total connections should be 1 (initial) + 5 (reconnects) = 6
	// Without mutex protection, we might see more connections due to race conditions
	if finalCount != 6 {
		t.Errorf("Expected exactly 6 connections (1 initial + 5 reconnects), got %d", finalCount)
	}

	client.Close()
}

func TestReconnect_UpdatesToken(t *testing.T) {
	server, _, _ := mockWSServer(t)
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

	client := &HAClient{
		url:   wsURL,
		token: "oldtoken",
		done:  make(chan struct{}),
	}

	if err := client.Connect(); err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	newToken := "newtoken"
	if err := client.Reconnect(context.Background(), newToken); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	client.mu.Lock()
	actualToken := client.token
	client.mu.Unlock()

	if actualToken != newToken {
		t.Errorf("Expected token to be updated to %s, got %s", newToken, actualToken)
	}

	client.Close()
}

func TestReconnect_ClosesOldConnection(t *testing.T) {
	server, _, _ := mockWSServer(t)
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)

	client := &HAClient{
		url:   wsURL,
		token: "token1",
		done:  make(chan struct{}),
	}

	if err := client.Connect(); err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Get reference to old connection
	client.mu.Lock()
	oldConn := client.conn
	client.mu.Unlock()

	if oldConn == nil {
		t.Fatal("Expected old connection to exist")
	}

	// Reconnect should close old connection
	if err := client.Reconnect(context.Background(), "token2"); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Try to read from old connection - should fail because it's closed
	if err := oldConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		// Connection already closed, which is what we expect
		t.Logf("SetReadDeadline failed as expected: %v", err)
		client.Close()
		return
	}

	var msg map[string]interface{}
	err := oldConn.ReadJSON(&msg)

	if err == nil {
		t.Error("Expected error when reading from closed connection")
	}

	client.Close()
}
