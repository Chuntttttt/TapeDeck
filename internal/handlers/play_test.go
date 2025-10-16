package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
)

func TestPlayHandler_PlayByRatingKey(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)

	// Create test database and user
	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i)
	}
	database, err := db.New(":memory:", testKey)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() { _ = database.Close() }()

	if err := database.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	ctx := context.Background()

	// Create user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := database.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Mock HA server
	haServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer haServer.Close()

	// Mock Plex server
	plexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"MediaContainer": map[string]interface{}{
				"Metadata": []map[string]interface{}{
					{
						"ratingKey": "123",
						"title":     "Test Movie",
						"type":      "movie",
					},
				},
			},
		})
	}))
	defer plexServer.Close()

	servers := []ServerInfo{
		{
			ID:   "test-server",
			Name: "Test Server",
			URLs: []string{plexServer.URL},
		},
	}

	appleTVs := []config.AppleTV{
		{
			Entity:  "media_player.test",
			Name:    "Test TV",
			Default: true,
		},
	}

	handler := NewPlayHandler(store, database, servers, appleTVs, haServer.URL, "test-ha-token", false)

	// Create request
	reqBody := map[string]string{
		"server_id":  "test-server",
		"rating_key": "123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/play", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// Set up session with user ID
	session, _ := store.Get(req, middleware.SessionName)
	middleware.SetUserID(session, userID)
	w := httptest.NewRecorder()
	_ = session.Save(req, w)

	// Add session cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.PlayByRatingKey))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got: %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["success"] != true {
		t.Errorf("expected success true, got: %v", response)
	}
}
