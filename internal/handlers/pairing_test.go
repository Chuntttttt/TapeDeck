package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/gorilla/websocket"
)

func TestPairingHandler_PairForm_NotAuthenticated(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	handler := NewPairingHandler(store, nil, nil, nil, "", "")

	req := httptest.NewRequest(http.MethodGet, "/mappings/pair", nil)
	w := httptest.NewRecorder()

	handler.PairForm(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestPairingHandler_PairForm_Authenticated(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	handler := NewPairingHandler(store, nil, nil, nil, "", "")

	req := httptest.NewRequest(http.MethodGet, "/mappings/pair", nil)
	w := httptest.NewRecorder()

	// Setup authenticated session
	session, _ := store.Get(req, middleware.SessionName)
	middleware.SetUserID(session, 1)
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	w = httptest.NewRecorder()
	handler.PairForm(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "NFC Pairing Mode") {
		t.Error("Response should contain 'NFC Pairing Mode'")
	}

	if !strings.Contains(strings.ToLower(body), "websocket") {
		t.Error("Response should contain websocket connection code")
	}
}

func TestPairingHandler_WebSocketUpgrade_NotAuthenticated(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	handler := NewPairingHandler(store, nil, nil, nil, "", "")

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	w := httptest.NewRecorder()

	handler.WebSocketPairing(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestPairingHandler_WebSocketPairing_Success(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create mock HA client
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}

	handler := NewPairingHandler(store, testDB, mockHA, nil, "media_player.test", "test-server")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Setup authenticated session
		session, _ := store.Get(r, middleware.SessionName)
		middleware.SetUserID(session, userID)
		_ = session.Save(r, w)

		handler.WebSocketPairing(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send start_pairing message
	startMsg := map[string]interface{}{
		"type":        "start_pairing",
		"media_id":    "12345",
		"media_title": "Toy Story",
		"media_type":  "movie",
	}
	if err := conn.WriteJSON(startMsg); err != nil {
		t.Fatalf("Failed to send start_pairing: %v", err)
	}

	// Simulate NFC tag scan
	time.Sleep(50 * time.Millisecond)
	mockHA.simulateTagScan("04-16-5C-D4-2E-61-80")

	// Read tag_detected message
	var tagDetectedMsg map[string]interface{}
	if err := conn.ReadJSON(&tagDetectedMsg); err != nil {
		t.Fatalf("Failed to read tag_detected: %v", err)
	}

	if tagDetectedMsg["type"] != "tag_detected" {
		t.Errorf("Message type = %v, want tag_detected", tagDetectedMsg["type"])
	}

	if tagDetectedMsg["tag_id"] != "04-16-5C-D4-2E-61-80" {
		t.Errorf("tag_id = %v, want 04-16-5C-D4-2E-61-80", tagDetectedMsg["tag_id"])
	}

	// Read mapping_created message
	var mappingCreatedMsg map[string]interface{}
	if err := conn.ReadJSON(&mappingCreatedMsg); err != nil {
		t.Fatalf("Failed to read mapping_created: %v", err)
	}

	if mappingCreatedMsg["type"] != "mapping_created" {
		t.Errorf("Message type = %v, want mapping_created", mappingCreatedMsg["type"])
	}

	if mappingCreatedMsg["tag_id"] != "04-16-5C-D4-2E-61-80" {
		t.Errorf("tag_id = %v, want 04-16-5C-D4-2E-61-80", mappingCreatedMsg["tag_id"])
	}

	if mappingCreatedMsg["media_title"] != "Toy Story" {
		t.Errorf("media_title = %v, want Toy Story", mappingCreatedMsg["media_title"])
	}

	// Verify mapping was created in database
	mapping, err := testDB.GetCardMappingByTagID("04-16-5C-D4-2E-61-80")
	if err != nil {
		t.Fatalf("Failed to get mapping: %v", err)
	}

	if mapping.MediaTitle != "Toy Story" {
		t.Errorf("mapping.MediaTitle = %s, want Toy Story", mapping.MediaTitle)
	}

	if mapping.MediaID != "12345" {
		t.Errorf("mapping.MediaID = %s, want 12345", mapping.MediaID)
	}

	if mapping.MediaType != "movie" {
		t.Errorf("mapping.MediaType = %s, want movie", mapping.MediaType)
	}
}

func TestPairingHandler_WebSocketPairing_DuplicateTag(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create existing mapping with same tag
	existingMapping := models.NewCardMapping(userID, "04-16-5C-D4-2E-61-80", "movie", "99999", "Existing Movie", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(existingMapping)
	if err != nil {
		t.Fatalf("Failed to create existing mapping: %v", err)
	}

	// Create mock HA client
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}

	handler := NewPairingHandler(store, testDB, mockHA, nil, "media_player.test", "test-server")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Setup authenticated session
		session, _ := store.Get(r, middleware.SessionName)
		middleware.SetUserID(session, userID)
		_ = session.Save(r, w)

		handler.WebSocketPairing(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send start_pairing message
	startMsg := map[string]interface{}{
		"type":        "start_pairing",
		"media_id":    "12345",
		"media_title": "Toy Story",
		"media_type":  "movie",
	}
	if err := conn.WriteJSON(startMsg); err != nil {
		t.Fatalf("Failed to send start_pairing: %v", err)
	}

	// Simulate NFC tag scan with duplicate tag
	time.Sleep(50 * time.Millisecond)
	mockHA.simulateTagScan("04-16-5C-D4-2E-61-80")

	// Read tag_detected message
	var tagDetectedMsg map[string]interface{}
	if err := conn.ReadJSON(&tagDetectedMsg); err != nil {
		t.Fatalf("Failed to read tag_detected: %v", err)
	}

	if tagDetectedMsg["type"] != "tag_detected" {
		t.Errorf("Message type = %v, want tag_detected", tagDetectedMsg["type"])
	}

	// Read error message about duplicate
	var errorMsg map[string]interface{}
	if err := conn.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error: %v", err)
	}

	if errorMsg["type"] != "error" {
		t.Errorf("Message type = %v, want error", errorMsg["type"])
	}

	if !strings.Contains(errorMsg["message"].(string), "already mapped") {
		t.Errorf("Error message = %v, want 'already mapped'", errorMsg["message"])
	}
}

func TestPairingHandler_WebSocketPairing_InvalidMessage(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}

	handler := NewPairingHandler(store, testDB, mockHA, nil, "media_player.test", "test-server")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Setup authenticated session
		session, _ := store.Get(r, middleware.SessionName)
		middleware.SetUserID(session, userID)
		_ = session.Save(r, w)

		handler.WebSocketPairing(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send invalid JSON
	if err := conn.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
		t.Fatalf("Failed to send invalid message: %v", err)
	}

	// Should close connection
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Expected connection to close after invalid message")
	}
}

func TestPairingHandler_WebSocketPairing_MissingFields(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}

	handler := NewPairingHandler(store, testDB, mockHA, nil, "media_player.test", "test-server")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Setup authenticated session
		session, _ := store.Get(r, middleware.SessionName)
		middleware.SetUserID(session, userID)
		_ = session.Save(r, w)

		handler.WebSocketPairing(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send message with missing fields
	startMsg := map[string]interface{}{
		"type": "start_pairing",
		// Missing media_id, media_title, media_type
	}
	if err := conn.WriteJSON(startMsg); err != nil {
		t.Fatalf("Failed to send start_pairing: %v", err)
	}

	// Read error message
	var errorMsg map[string]interface{}
	if err := conn.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error: %v", err)
	}

	if errorMsg["type"] != "error" {
		t.Errorf("Message type = %v, want error", errorMsg["type"])
	}

	if !strings.Contains(errorMsg["message"].(string), "Missing required fields") {
		t.Errorf("Error message = %v, want 'Missing required fields'", errorMsg["message"])
	}
}

func TestPairingMessageTypes(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantType string
	}{
		{
			name:     "start_pairing message",
			jsonData: `{"type":"start_pairing","media_id":"123","media_title":"Test","media_type":"movie"}`,
			wantType: "start_pairing",
		},
		{
			name:     "tag_detected message",
			jsonData: `{"type":"tag_detected","tag_id":"test-tag"}`,
			wantType: "tag_detected",
		},
		{
			name:     "mapping_created message",
			jsonData: `{"type":"mapping_created","tag_id":"test-tag","media_title":"Test"}`,
			wantType: "mapping_created",
		},
		{
			name:     "error message",
			jsonData: `{"type":"error","message":"Test error"}`,
			wantType: "error",
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

// Mock HA client for testing
type mockHAClient struct {
	tagCallbacks []func(string)
}

func (m *mockHAClient) OnTagScanned(callback func(tagID string)) {
	m.tagCallbacks = append(m.tagCallbacks, callback)
}

func (m *mockHAClient) IsConnected() bool {
	return true
}

func (m *mockHAClient) Reconnect(newToken string) error {
	return nil
}

func (m *mockHAClient) simulateTagScan(tagID string) {
	for _, callback := range m.tagCallbacks {
		callback(tagID)
	}
}

// Mock HA REST client for testing
type mockHARestClient struct {
	entityState       string
	getStateError     error
	turnOnCalls       []string
	turnOnError       error
	playMediaCalls    []playMediaCall
	playMediaError    error
}

type playMediaCall struct {
	entityID    string
	contentType string
	contentID   string
}

func (m *mockHARestClient) GetEntityState(entityID string) (string, error) {
	if m.getStateError != nil {
		return "", m.getStateError
	}
	if m.entityState == "" {
		return "idle", nil // Default state
	}
	return m.entityState, nil
}

func (m *mockHARestClient) TurnOn(entityID string) error {
	m.turnOnCalls = append(m.turnOnCalls, entityID)
	return m.turnOnError
}

func (m *mockHARestClient) PlayMedia(entityID, contentType, contentID string) error {
	m.playMediaCalls = append(m.playMediaCalls, playMediaCall{
		entityID:    entityID,
		contentType: contentType,
		contentID:   contentID,
	})
	return m.playMediaError
}

func TestPairingHandler_Playback_Success(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create mapping
	mapping := models.NewCardMapping(userID, "test-tag-123", "movie", "12345", "Toy Story", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create mock HA clients
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}
	mockRest := &mockHARestClient{}

	_ = NewPairingHandler(store, testDB, mockHA, mockRest, "media_player.apple_tv", "server-abc123")

	// Simulate tag scan (no pairing clients active)
	mockHA.simulateTagScan("test-tag-123")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Verify PlayMedia was called
	if len(mockRest.playMediaCalls) != 1 {
		t.Fatalf("PlayMedia calls = %d, want 1", len(mockRest.playMediaCalls))
	}

	call := mockRest.playMediaCalls[0]
	if call.entityID != "media_player.apple_tv" {
		t.Errorf("entityID = %s, want media_player.apple_tv", call.entityID)
	}

	if call.contentType != "url" {
		t.Errorf("contentType = %s, want url", call.contentType)
	}

	expectedURL := "plex://play/?metadataKey=/library/metadata/12345&server=server-abc123"
	if call.contentID != expectedURL {
		t.Errorf("contentID = %s, want %s", call.contentID, expectedURL)
	}
}

func TestPairingHandler_Playback_NoMapping(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create mock HA clients
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}
	mockRest := &mockHARestClient{}

	_ = NewPairingHandler(store, testDB, mockHA, mockRest, "media_player.apple_tv", "server-abc123")

	// Simulate tag scan with unmapped tag
	mockHA.simulateTagScan("unmapped-tag")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Verify PlayMedia was NOT called
	if len(mockRest.playMediaCalls) != 0 {
		t.Errorf("PlayMedia calls = %d, want 0", len(mockRest.playMediaCalls))
	}
}

func TestPairingHandler_Playback_RestClientError(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create mapping
	mapping := models.NewCardMapping(userID, "test-tag-456", "movie", "67890", "Finding Nemo", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create mock HA clients with error
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}
	mockRest := &mockHARestClient{
		playMediaError: fmt.Errorf("HA service unavailable"),
	}

	_ = NewPairingHandler(store, testDB, mockHA, mockRest, "media_player.apple_tv", "server-abc123")

	// Simulate tag scan
	mockHA.simulateTagScan("test-tag-456")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Verify PlayMedia was called (even though it errored)
	if len(mockRest.playMediaCalls) != 1 {
		t.Fatalf("PlayMedia calls = %d, want 1", len(mockRest.playMediaCalls))
	}

	// The handler should log the error but not crash
}

func TestPairingHandler_Playback_NilRestClient(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create mapping
	mapping := models.NewCardMapping(userID, "test-tag-789", "movie", "11111", "The Incredibles", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create mock HA client with nil REST client
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}

	_ = NewPairingHandler(store, testDB, mockHA, nil, "media_player.apple_tv", "server-abc123")

	// Simulate tag scan
	mockHA.simulateTagScan("test-tag-789")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Should handle gracefully without crashing
}

func TestPairingHandler_PairingMode_StillWorks(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	// Run migrations
	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("testuser", "plex-user-123", "test-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create mock HA clients
	mockHA := &mockHAClient{
		tagCallbacks: []func(string){},
	}
	mockRest := &mockHARestClient{}

	handler := NewPairingHandler(store, testDB, mockHA, mockRest, "media_player.apple_tv", "server-abc123")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Setup authenticated session
		session, _ := store.Get(r, middleware.SessionName)
		middleware.SetUserID(session, userID)
		_ = session.Save(r, w)

		handler.WebSocketPairing(w, r)
	}))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect WebSocket client (entering pairing mode)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send start_pairing message
	startMsg := map[string]interface{}{
		"type":        "start_pairing",
		"media_id":    "99999",
		"media_title": "Test Movie",
		"media_type":  "movie",
	}
	if err := conn.WriteJSON(startMsg); err != nil {
		t.Fatalf("Failed to send start_pairing: %v", err)
	}

	// Simulate NFC tag scan
	time.Sleep(50 * time.Millisecond)
	mockHA.simulateTagScan("pairing-tag-123")

	// Read tag_detected message
	var tagDetectedMsg map[string]interface{}
	if err := conn.ReadJSON(&tagDetectedMsg); err != nil {
		t.Fatalf("Failed to read tag_detected: %v", err)
	}

	if tagDetectedMsg["type"] != "tag_detected" {
		t.Errorf("Message type = %v, want tag_detected", tagDetectedMsg["type"])
	}

	// Read mapping_created message
	var mappingCreatedMsg map[string]interface{}
	if err := conn.ReadJSON(&mappingCreatedMsg); err != nil {
		t.Fatalf("Failed to read mapping_created: %v", err)
	}

	if mappingCreatedMsg["type"] != "mapping_created" {
		t.Errorf("Message type = %v, want mapping_created", mappingCreatedMsg["type"])
	}

	// Verify PlayMedia was NOT called (in pairing mode)
	if len(mockRest.playMediaCalls) != 0 {
		t.Errorf("PlayMedia calls = %d, want 0 (should not play in pairing mode)", len(mockRest.playMediaCalls))
	}

	// Verify mapping was created
	mapping, err := testDB.GetCardMappingByTagID("pairing-tag-123")
	if err != nil {
		t.Fatalf("Failed to get mapping: %v", err)
	}

	if mapping.MediaTitle != "Test Movie" {
		t.Errorf("mapping.MediaTitle = %s, want Test Movie", mapping.MediaTitle)
	}
}
