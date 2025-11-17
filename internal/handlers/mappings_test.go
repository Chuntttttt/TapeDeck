package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/go-chi/chi/v5"
)

func TestMappingsHandler_Dashboard(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create some test mappings
	mapping1 := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(ctx, mapping1)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	mapping2 := models.NewCardMapping(userID, "nfc-789", "show", "rating-101", "Breaking Bad", "test-server-id", "media_player.test")
	_, err = testDB.CreateCardMapping(ctx, mapping2)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/mappings", nil)
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

	// Wrap handler with middleware for tests (WithUserID -> WithUser chain)
	wrappedHandler := middleware.WithUserID(store)(
		middleware.WithUser(store, testDB)(
			http.HandlerFunc(handler.Dashboard),
		),
	)

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" && contentType != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	// Body should contain mapping data in visual grid format
	body := w.Body.String()
	if !strings.Contains(body, "My Card Collection") {
		t.Error("Response body should contain 'My Card Collection' heading")
	}
	if !strings.Contains(body, "The Matrix") {
		t.Error("Response body should contain media title The Matrix")
	}
	if !strings.Contains(body, "Breaking Bad") {
		t.Error("Response body should contain media title Breaking Bad")
	}
	if !strings.Contains(body, "card-grid") {
		t.Error("Response body should contain card-grid class for visual layout")
	}
}

func TestMappingsHandler_Dashboard_Empty(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/mappings", nil)
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

	// Wrap handler with middleware for tests (WithUserID -> WithUser chain)
	wrappedHandler := middleware.WithUserID(store)(
		middleware.WithUser(store, testDB)(
			http.HandlerFunc(handler.Dashboard),
		),
	)

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Body should contain empty state message
	body := w.Body.String()
	if !strings.Contains(body, "No card mappings yet") {
		t.Error("Response body should contain empty state message")
	}
}

func TestMappingsHandler_NewMappingForm(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/mappings/new", nil)
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

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.NewMappingForm))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Body should contain form elements
	body := w.Body.String()
	if !strings.Contains(body, "tag_id") {
		t.Error("Response body should contain tag_id input")
	}
	if !strings.Contains(body, "media_search") {
		t.Error("Response body should contain media_search input")
	}
}

func TestMappingsHandler_CreateMapping(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create form data
	formData := url.Values{}
	formData.Set("tag_id", "nfc-123")
	formData.Set("media_type", "movie")
	formData.Set("media_id", "rating-456")
	formData.Set("media_title", "The Matrix")
	formData.Set("plex_server_id", "test-server-id")
	formData.Set("apple_tv_entity", "media_player.test")

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodPost, "/mappings", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.CreateMapping))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusFound)
	}

	// Check redirect location
	location := w.Header().Get("Location")
	if location != "/mappings" {
		t.Errorf("Location = %s, want /mappings", location)
	}

	// Verify mapping was created
	mappings, err := testDB.GetCardMappingsByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get mappings: %v", err)
	}

	if len(mappings) != 1 {
		t.Fatalf("Expected 1 mapping, got %d", len(mappings))
	}

	if mappings[0].TagID != "nfc-123" {
		t.Errorf("TagID = %s, want nfc-123", mappings[0].TagID)
	}
	if mappings[0].MediaTitle != "The Matrix" {
		t.Errorf("MediaTitle = %s, want The Matrix", mappings[0].MediaTitle)
	}
}

func TestMappingsHandler_EditMappingForm(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	mappingID, err := testDB.CreateCardMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/mappings/1/edit", nil)
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
	rctx.URLParams.Add("id", fmt.Sprintf("%d", mappingID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.EditMappingForm))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Body should contain form with current values
	body := w.Body.String()
	if !strings.Contains(body, "nfc-123") {
		t.Error("Response body should contain tag ID nfc-123")
	}
	if !strings.Contains(body, "The Matrix") {
		t.Error("Response body should contain media title The Matrix")
	}
}

func TestMappingsHandler_UpdateMapping(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	mappingID, err := testDB.CreateCardMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create form data with updated tag ID
	formData := url.Values{}
	formData.Set("tag_id", "nfc-999")
	formData.Set("media_type", "movie")
	formData.Set("media_id", "rating-456")
	formData.Set("media_title", "The Matrix")
	formData.Set("plex_server_id", "test-server-id")
	formData.Set("apple_tv_entity", "media_player.test")

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodPost, "/mappings/1", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	rctx.URLParams.Add("id", fmt.Sprintf("%d", mappingID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.UpdateMapping))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusFound)
	}

	// Verify mapping was updated
	updated, err := testDB.GetCardMappingByID(ctx, mappingID)
	if err != nil {
		t.Fatalf("Failed to get updated mapping: %v", err)
	}

	if updated.TagID != "nfc-999" {
		t.Errorf("TagID = %s, want nfc-999", updated.TagID)
	}
}

func TestMappingsHandler_DeleteMapping(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	mappingID, err := testDB.CreateCardMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("Failed to create mapping: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodPost, "/mappings/1/delete", nil)
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
	rctx.URLParams.Add("id", fmt.Sprintf("%d", mappingID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Wrap handler with middleware for tests
	wrappedHandler := middleware.WithUserID(store)(http.HandlerFunc(handler.DeleteMapping))

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusFound)
	}

	// Verify mapping was deleted
	_, err = testDB.GetCardMappingByID(ctx, mappingID)
	if err == nil {
		t.Error("Expected error when getting deleted mapping, got nil")
	}
}

func TestMappingsHandler_SearchJSON(t *testing.T) {
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)

	// Create mock Plex client
	mockPlex := &mockPlexClient{
		searchFunc: func(_ context.Context, query string) ([]plex.MediaItem, error) {
			if query == "matrix" {
				return []plex.MediaItem{
					{RatingKey: "100", Title: "The Matrix", Type: "movie", Year: 1999},
					{RatingKey: "101", Title: "The Matrix Reloaded", Type: "movie", Year: 2003},
				}, nil
			}
			return []plex.MediaItem{}, nil
		},
	}

	handler := &MappingsHandler{
		sessionStore: store,
		db:           nil,
		devMode:      false,
		servers: []ServerInfo{
			{
				ID:   "test-server",
				Name: "Test Server",
				URLs: []string{"http://test-server:32400"},
			},
		},
		newPlexClient: func(_ string, _ string, _ string, _ bool) PlexClientInterface {
			return mockPlex
		},
	}

	// Create test database and user
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

	handler.db = testDB

	ctx := context.Background()
	// Create a test user
	user := models.NewUser("testuser", "plex-user-123", "test-auth-token")
	userID, err := testDB.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create request with authenticated session
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=matrix", nil)
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

	// Wrap handler with middleware for tests (WithUserID -> WithUser chain)
	wrappedHandler := middleware.WithUserID(store)(
		middleware.WithUser(store, testDB)(
			http.HandlerFunc(handler.SearchJSON),
		),
	)

	// Make request
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Body should contain JSON results
	body := w.Body.String()
	if !strings.Contains(body, "The Matrix") {
		t.Error("Response body should contain The Matrix in JSON")
	}
	if !strings.Contains(body, "ratingKey") {
		t.Error("Response body should contain ratingKey field")
	}
}

func TestGenerateStickers(t *testing.T) {
	// Setup test database
	// Create test encryption key (32 bytes for AES-256)
	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i)
	}
	database, err := db.New(":memory:", testKey)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = database.Close() }()

	if err := database.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create test user
	user := models.NewUser("test@example.com", "Test User", "test-token")
	userID, err := database.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user.ID = userID

	// Create test mappings
	mapping1 := models.NewCardMapping(userID, "tag1", "movie", "123", "Test Movie 1", "server1", "appletv1")
	mapping1ID, err := database.CreateCardMapping(context.Background(), mapping1)
	if err != nil {
		t.Fatalf("Failed to create mapping1: %v", err)
	}

	mapping2 := models.NewCardMapping(userID, "tag2", "show", "456", "Test Show 2", "server1", "appletv1")
	mapping2ID, err := database.CreateCardMapping(context.Background(), mapping2)
	if err != nil {
		t.Fatalf("Failed to create mapping2: %v", err)
	}

	// Create handler
	store := middleware.NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	handler := NewMappingsHandler(store, database, []ServerInfo{}, true)

	// Create request with form data
	formData := fmt.Sprintf("mapping_ids=%d&mapping_ids=%d", mapping1ID, mapping2ID)
	req := httptest.NewRequest("POST", "/mappings/generate-stickers", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

	// Wrap handler with middleware for tests (WithUserID -> WithUser chain)
	wrappedHandler := middleware.WithUserID(store)(
		middleware.WithUser(store, database)(
			http.HandlerFunc(handler.GenerateStickers),
		),
	)

	// Make request
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/pdf" {
		t.Errorf("Expected Content-Type application/pdf, got %s", contentType)
	}

	// Verify PDF signature
	body := rr.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("Response body is empty")
	}
	if !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Error("Response is not a valid PDF")
	}
}
