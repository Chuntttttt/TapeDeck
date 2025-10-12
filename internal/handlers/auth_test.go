package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

func TestAuthHandler_Login_GET(t *testing.T) {
	// Create test dependencies
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))
	authClient := plex.NewAuthClient("https://plex.tv", "test-client", "TapeDeck")

	handler := NewAuthHandler(store, authClient, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	w := httptest.NewRecorder()

	handler.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Should contain "Login with Plex" or similar
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Response body is empty")
	}
}

func TestAuthHandler_Callback_Success(t *testing.T) {
	// This is a more complex integration test that would require
	// mocking the Plex PIN check and database operations
	// For now, we'll test the basic structure
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))
	authClient := plex.NewAuthClient("https://plex.tv", "test-client", "TapeDeck")

	// Create temporary test database
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	handler := NewAuthHandler(store, authClient, testDB)

	// Test that handler exists and can be called
	if handler == nil {
		t.Fatal("NewAuthHandler() returned nil")
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	handler := NewAuthHandler(store, nil, nil)

	// Setup authenticated session
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	session, _ := store.Get(req, middleware.SessionName)
	middleware.SetUserID(session, 12345)
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Call logout
	w = httptest.NewRecorder()
	handler.Logout(w, req)

	// Should redirect
	if w.Code != http.StatusFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusFound)
	}

	location := w.Header().Get("Location")
	if location != "/auth/login" {
		t.Errorf("Redirect location = %s, want /auth/login", location)
	}
}

func TestGetOrCreateSession(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	session := getOrCreateSession(store, req)
	if session == nil {
		t.Fatal("getOrCreateSession() returned nil")
	}
}

func TestGetOrCreateSession_ExistingSession(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Create a session first
	session, _ := store.Get(req, middleware.SessionName)
	session.Values["test"] = "value"
	_ = session.Save(req, w)

	// Add cookie to new request
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req2.AddCookie(cookie)
	}

	// Get session
	session2 := getOrCreateSession(store, req2)
	if session2 == nil {
		t.Fatal("getOrCreateSession() returned nil")
	}

	// Should have the test value
	if val, ok := session2.Values["test"]; !ok || val != "value" {
		t.Error("Session did not preserve values")
	}
}

func setupTestSession(store *sessions.CookieStore, req *http.Request, userID int64) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	session, _ := store.Get(req, middleware.SessionName)
	middleware.SetUserID(session, userID)
	_ = session.Save(req, w)
	return w
}
