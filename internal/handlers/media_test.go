package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
)

func TestMediaHandler_Libraries(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client
	mockPlex := &mockPlexClient{
		getLibrariesFunc: func() ([]plex.Library, error) {
			return []plex.Library{
				{Key: "1", Type: "movie", Title: "Movies"},
				{Key: "2", Type: "show", Title: "TV Shows"},
			}, nil
		},
	}

	handler := &MediaHandler{
		sessionStore: store,
		db:           nil,
		servers: []ServerInfo{
			{ID: "test-server-1", Name: "Test Server", URLs: []string{"http://localhost:32400"}},
		},
		devMode: false,
		newPlexClient: func(_, _, _ string, _ bool) PlexClientInterface {
			return mockPlex
		},
	}

	// Create test database and user
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	handler.db = testDB

	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/libraries", nil)
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

	// Make request
	w = httptest.NewRecorder()
	handler.Libraries(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	// Body should contain library titles
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Response body is empty")
	}
}

func TestMediaHandler_LibraryContents(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client
	mockPlex := &mockPlexClient{
		getLibraryContentsFunc: func(libraryKey string) ([]plex.MediaItem, error) {
			if libraryKey != "1" {
				return nil, nil
			}
			return []plex.MediaItem{
				{RatingKey: "100", Title: "The Matrix", Type: "movie", Year: 1999},
				{RatingKey: "101", Title: "Inception", Type: "movie", Year: 2010},
			}, nil
		},
	}

	handler := &MediaHandler{
		sessionStore: store,
		db:           nil,
		servers: []ServerInfo{
			{ID: "test-server-1", Name: "Test Server", URLs: []string{"http://localhost:32400"}},
		},
		devMode: false,
		newPlexClient: func(_, _, _ string, _ bool) PlexClientInterface {
			return mockPlex
		},
	}

	// Create test database and user
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	handler.db = testDB

	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/libraries/1", nil)
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

	// Make request
	w = httptest.NewRecorder()
	handler.LibraryContents(w, req, "1")

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	// Body should contain media titles
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Response body is empty")
	}
}

func TestMediaHandler_Search(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Create mock Plex client
	mockPlex := &mockPlexClient{
		searchFunc: func(query string) ([]plex.MediaItem, error) {
			if query == "matrix" {
				return []plex.MediaItem{
					{RatingKey: "100", Title: "The Matrix", Type: "movie", Year: 1999},
				}, nil
			}
			return []plex.MediaItem{}, nil
		},
	}

	handler := &MediaHandler{
		sessionStore: store,
		db:           nil,
		servers: []ServerInfo{
			{ID: "test-server-1", Name: "Test Server", URLs: []string{"http://localhost:32400"}},
		},
		devMode: false,
		newPlexClient: func(_, _, _ string, _ bool) PlexClientInterface {
			return mockPlex
		},
	}

	// Create test database and user
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	handler.db = testDB

	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/search?q=matrix", nil)
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

	// Make request
	w = httptest.NewRecorder()
	handler.Search(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	// Body should contain search results
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Response body is empty")
	}
}

func TestMediaHandler_Search_EmptyQuery(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	handler := &MediaHandler{
		sessionStore: store,
		db:           nil,
		servers: []ServerInfo{
			{ID: "test-server-1", Name: "Test Server", URLs: []string{"http://localhost:32400"}},
		},
		devMode: false,
	}

	// Create test database and user
	testDB, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	handler.db = testDB

	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session but no query
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
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

	// Make request
	w = httptest.NewRecorder()
	handler.Search(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	// Body should contain search form
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Response body is empty")
	}
}

// Mock Plex client for testing
type mockPlexClient struct {
	getLibrariesFunc       func() ([]plex.Library, error)
	getLibraryContentsFunc func(libraryKey string) ([]plex.MediaItem, error)
	searchFunc             func(query string) ([]plex.MediaItem, error)
}

func (m *mockPlexClient) GetLibraries() ([]plex.Library, error) {
	if m.getLibrariesFunc != nil {
		return m.getLibrariesFunc()
	}
	return nil, nil
}

func (m *mockPlexClient) GetLibraryContents(libraryKey string) ([]plex.MediaItem, error) {
	if m.getLibraryContentsFunc != nil {
		return m.getLibraryContentsFunc(libraryKey)
	}
	return nil, nil
}

func (m *mockPlexClient) Search(query string) ([]plex.MediaItem, error) {
	if m.searchFunc != nil {
		return m.searchFunc(query)
	}
	return nil, nil
}
