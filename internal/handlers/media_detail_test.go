package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/go-chi/chi/v5"
)

func TestMediaDetailHandler_Detail(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)

	// Create test encryption key (32 bytes for AES-256)
	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i)
	}
	testDB, err := db.New(":memory:", testKey)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	ctx := context.Background()

	// Create user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, _ := testDB.CreateUser(ctx, user)

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
						"thumb":     "/library/metadata/123/thumb/1234567890",
						"year":      2023,
						"summary":   "A test movie summary",
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

	handler := NewMediaDetailHandler(store, testDB, servers, false, nil)

	// Create request
	req := httptest.NewRequest("GET", "/media/test-server/123", nil)
	w := httptest.NewRecorder()

	// Set up session with user ID
	session, _ := store.Get(req, middleware.SessionName)
	middleware.SetUserID(session, userID)
	_ = session.Save(req, w)

	// Add session cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Add chi URL context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("serverID", "test-server")
	rctx.URLParams.Add("ratingKey", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.Detail))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got: %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Test Movie") {
		t.Error("expected body to contain 'Test Movie'")
	}
	if !strings.Contains(body, "A test movie summary") {
		t.Error("expected body to contain summary")
	}
}
