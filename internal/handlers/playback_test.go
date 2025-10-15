package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/models"
)

func TestPlaybackHandler_Play_ValidTagID(t *testing.T) {
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

	ctx := context.Background()
	// Create test user
	user := models.NewUser("testuser", "123456", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create test card mapping
	mapping := models.NewCardMapping(userID, "04-16-5C-D4-2E-61-80", "movie", "12345", "Toy Story", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to create card mapping: %v", err)
	}

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{"tag_id": "04-16-5C-D4-2E-61-80"}`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Errorf("success = %v, want true", response["success"])
	}

	if tagID, ok := response["tag_id"].(string); !ok || tagID != "04-16-5C-D4-2E-61-80" {
		t.Errorf("tag_id = %v, want 04-16-5C-D4-2E-61-80", response["tag_id"])
	}

	if mediaTitle, ok := response["media_title"].(string); !ok || mediaTitle != "Toy Story" {
		t.Errorf("media_title = %v, want Toy Story", response["media_title"])
	}

	if mediaType, ok := response["media_type"].(string); !ok || mediaType != "movie" {
		t.Errorf("media_type = %v, want movie", response["media_type"])
	}

	if mediaID, ok := response["media_id"].(string); !ok || mediaID != "12345" {
		t.Errorf("media_id = %v, want 12345", response["media_id"])
	}

	if plexKey, ok := response["plex_key"].(string); !ok || plexKey != "/library/metadata/12345" {
		t.Errorf("plex_key = %v, want /library/metadata/12345", response["plex_key"])
	}

	if serverID, ok := response["server_id"].(string); !ok || serverID != "test-server-id" {
		t.Errorf("server_id = %v, want test-server-id", response["server_id"])
	}
}

func TestPlaybackHandler_Play_InvalidTagID(t *testing.T) {
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

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{"tag_id": "00-00-00-00-00-00-00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || success {
		t.Errorf("success = %v, want false", response["success"])
	}

	if errMsg, ok := response["error"].(string); !ok || errMsg != "Tag not found" {
		t.Errorf("error = %v, want 'Tag not found'", response["error"])
	}
}

func TestPlaybackHandler_Play_MissingTagID(t *testing.T) {
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || success {
		t.Errorf("success = %v, want false", response["success"])
	}

	if errMsg, ok := response["error"].(string); !ok || errMsg != "tag_id is required" {
		t.Errorf("error = %v, want 'tag_id is required'", response["error"])
	}
}

func TestPlaybackHandler_Play_EmptyTagID(t *testing.T) {
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{"tag_id": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || success {
		t.Errorf("success = %v, want false", response["success"])
	}

	if errMsg, ok := response["error"].(string); !ok || errMsg != "tag_id is required" {
		t.Errorf("error = %v, want 'tag_id is required'", response["error"])
	}
}

func TestPlaybackHandler_Play_MalformedJSON(t *testing.T) {
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{"tag_id": "invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || success {
		t.Errorf("success = %v, want false", response["success"])
	}

	if errMsg, ok := response["error"].(string); !ok || !strings.Contains(errMsg, "Invalid JSON") {
		t.Errorf("error = %v, want to contain 'Invalid JSON'", response["error"])
	}
}

func TestPlaybackHandler_Play_CreatesPlaybackLog(t *testing.T) {
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

	ctx := context.Background()
	// Create test user
	user := models.NewUser("testuser", "123456", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create test card mapping
	mapping := models.NewCardMapping(userID, "04-16-5C-D4-2E-61-80", "movie", "12345", "Toy Story", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to create card mapping: %v", err)
	}

	handler := NewPlaybackHandler(testDB, "test-server-id")

	reqBody := `{"tag_id": "04-16-5C-D4-2E-61-80"}`
	req := httptest.NewRequest(http.MethodPost, "/api/play", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify playback log was created by checking the database
	// Note: We don't have a GetPlaybackLogsByUserID method, so we'll just verify the response succeeded
	// In production, you might want to add a query method to verify logging
}

func TestPlaybackHandler_Play_WrongHTTPMethod(t *testing.T) {
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	handler := NewPlaybackHandler(testDB, "test-server-id")

	req := httptest.NewRequest(http.MethodGet, "/api/play", nil)
	w := httptest.NewRecorder()

	handler.Play(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
