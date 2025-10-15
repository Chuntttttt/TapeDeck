package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
)

func TestAuthHandler_Login_GET(t *testing.T) {
	// Create test dependencies
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))
	authClient := plex.NewAuthClient("https://plex.tv", "test-client", "TapeDeck", false)

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
	authClient := plex.NewAuthClient("https://plex.tv", "test-client", "TapeDeck", false)

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

func TestAuthHandler_PollStatus_NoPINInSession(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))
	handler := NewAuthHandler(store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/poll-status", nil)
	w := httptest.NewRecorder()

	handler.PollStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", w.Header().Get("Content-Type"))
	}

	// Should return {"authorized": false}
	body := strings.TrimSpace(w.Body.String())
	if body != `{"authorized":false}` {
		t.Errorf("Body = %s, want {\"authorized\":false}", body)
	}
}

func TestAuthHandler_PollStatus_InvalidPINIDType(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))
	handler := NewAuthHandler(store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/poll-status", nil)
	w := httptest.NewRecorder()

	// Create session with invalid PIN ID type (string instead of int)
	session, _ := store.Get(req, middleware.SessionName)
	session.Values["plex_pin_id"] = "not-an-int"
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	w = httptest.NewRecorder()
	handler.PollStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != `{"authorized":false}` {
		t.Errorf("Body = %s, want {\"authorized\":false}", body)
	}
}

func TestAuthHandler_PollStatus_RateLimited(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client that returns rate limit error
	mockPlex := &mockPlexAuthClient{
		checkPINFunc: func(_ context.Context, _ int) (*plex.PINCheckResponse, error) {
			return nil, &httpError{code: 429, message: "unexpected status code: 429"}
		},
	}

	handler := &AuthHandler{
		sessionStore: store,
		plexAuth:     mockPlex,
		db:           nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/poll-status", nil)
	w := httptest.NewRecorder()

	// Create session with PIN ID
	session, _ := store.Get(req, middleware.SessionName)
	session.Values["plex_pin_id"] = 12345
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	w = httptest.NewRecorder()
	handler.PollStatus(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != `{"authorized":false}` {
		t.Errorf("Body = %s, want {\"authorized\":false}", body)
	}
}

func TestAuthHandler_PollStatus_NotYetAuthorized(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client that returns empty auth token
	mockPlex := &mockPlexAuthClient{
		checkPINFunc: func(_ context.Context, _ int) (*plex.PINCheckResponse, error) {
			return &plex.PINCheckResponse{
				ID:        12345,
				Code:      "ABC123",
				AuthToken: "", // Not yet authorized
			}, nil
		},
	}

	handler := &AuthHandler{
		sessionStore: store,
		plexAuth:     mockPlex,
		db:           nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/poll-status", nil)
	w := httptest.NewRecorder()

	// Create session with PIN ID
	session, _ := store.Get(req, middleware.SessionName)
	session.Values["plex_pin_id"] = 12345
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	w = httptest.NewRecorder()
	handler.PollStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != `{"authorized":false}` {
		t.Errorf("Body = %s, want {\"authorized\":false}", body)
	}
}

func TestAuthHandler_PollStatus_Authorized_CreateNewUser(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client that returns auth token
	mockPlex := &mockPlexAuthClient{
		checkPINFunc: func(_ context.Context, _ int) (*plex.PINCheckResponse, error) {
			return &plex.PINCheckResponse{
				ID:        12345,
				Code:      "ABC123",
				AuthToken: "test-auth-token-xyz",
			}, nil
		},
	}

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

	handler := &AuthHandler{
		sessionStore: store,
		plexAuth:     mockPlex,
		db:           testDB,
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/poll-status", nil)
	w := httptest.NewRecorder()

	// Create session with PIN ID
	session, _ := store.Get(req, middleware.SessionName)
	session.Values["plex_pin_id"] = 12345
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	w = httptest.NewRecorder()
	handler.PollStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != `{"authorized":true}` {
		t.Errorf("Body = %s, want {\"authorized\":true}", body)
	}

	// Verify session was updated
	cookies = w.Result().Cookies()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range cookies {
		req2.AddCookie(cookie)
	}
	session2, _ := store.Get(req2, middleware.SessionName)
	userID, ok := middleware.GetUserID(session2)
	if !ok {
		t.Error("User ID not set in session")
	}
	if userID == 0 {
		t.Error("User ID is zero")
	}

	// Verify PIN values were cleared from session
	if _, ok := session2.Values["plex_pin_id"]; ok {
		t.Error("plex_pin_id should be removed from session")
	}
	if _, ok := session2.Values["plex_pin_code"]; ok {
		t.Error("plex_pin_code should be removed from session")
	}
}

// Mock Plex auth client for testing
type mockPlexAuthClient struct {
	requestPINFunc func(context.Context) (*plex.PINResponse, error)
	checkPINFunc   func(context.Context, int) (*plex.PINCheckResponse, error)
}

func (m *mockPlexAuthClient) RequestPIN(ctx context.Context) (*plex.PINResponse, error) {
	if m.requestPINFunc != nil {
		return m.requestPINFunc(ctx)
	}
	return nil, nil
}

func (m *mockPlexAuthClient) CheckPIN(ctx context.Context, pinID int) (*plex.PINCheckResponse, error) {
	if m.checkPINFunc != nil {
		return m.checkPINFunc(ctx, pinID)
	}
	return nil, nil
}

// Mock HTTP error for testing
type httpError struct {
	code    int
	message string
}

func (e *httpError) Error() string {
	return e.message
}
