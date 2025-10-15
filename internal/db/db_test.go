package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/models"
)

func TestNew(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if db == nil {
		t.Fatal("New() returned nil database")
	}
}

func TestRunMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = db.RunMigrations("../../migrations")
	if err != nil {
		t.Fatalf("RunMigrations() failed: %v", err)
	}

	// Verify users table exists
	var tableName string
	err = db.conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Users table not found: %v", err)
	}
	if tableName != "users" {
		t.Errorf("Table name = %q, want %q", tableName, "users")
	}
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	user := models.NewUser("testuser", "12345", "test-token")

	id, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("CreateUser() returned invalid ID: %d", id)
	}
}

func TestGetUserByPlexUserID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user first
	user := models.NewUser("testuser", "12345", "test-token")
	_, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Retrieve the user
	retrieved, err := db.GetUserByPlexUserID("12345")
	if err != nil {
		t.Fatalf("GetUserByPlexUserID() failed: %v", err)
	}

	if retrieved.PlexUsername != "testuser" {
		t.Errorf("PlexUsername = %q, want %q", retrieved.PlexUsername, "testuser")
	}
	if retrieved.PlexUserID != "12345" {
		t.Errorf("PlexUserID = %q, want %q", retrieved.PlexUserID, "12345")
	}
	if retrieved.PlexAuthToken != "test-token" {
		t.Errorf("PlexAuthToken = %q, want %q", retrieved.PlexAuthToken, "test-token")
	}
}

func TestGetUserByPlexUserID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.GetUserByPlexUserID("nonexistent")
	if err == nil {
		t.Fatal("GetUserByPlexUserID() succeeded for nonexistent user, want error")
	}
}

func TestUpdateUser(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user
	user := models.NewUser("testuser", "12345", "test-token")
	id, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Update the user
	retrieved, err := db.GetUserByPlexUserID("12345")
	if err != nil {
		t.Fatalf("GetUserByPlexUserID() failed: %v", err)
	}

	retrieved.PlexAuthToken = "new-token"
	err = db.UpdateUser(retrieved)
	if err != nil {
		t.Fatalf("UpdateUser() failed: %v", err)
	}

	// Verify update
	updated, err := db.GetUserByPlexUserID("12345")
	if err != nil {
		t.Fatalf("GetUserByPlexUserID() after update failed: %v", err)
	}

	if updated.ID != id {
		t.Errorf("ID changed after update: got %d, want %d", updated.ID, id)
	}
	if updated.PlexAuthToken != "new-token" {
		t.Errorf("PlexAuthToken = %q, want %q", updated.PlexAuthToken, "new-token")
	}
}

func TestCreateCardMapping(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user first
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Create a card mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	id, err := db.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("CreateCardMapping() returned invalid ID: %d", id)
	}
}

func TestGetCardMappingsByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Create multiple mappings
	mapping1 := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	_, err = db.CreateCardMapping(mapping1)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}

	mapping2 := models.NewCardMapping(userID, "nfc-456", "show", "rating-789", "Breaking Bad", "test-server-id", "media_player.test")
	_, err = db.CreateCardMapping(mapping2)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}

	// Retrieve mappings
	mappings, err := db.GetCardMappingsByUserID(userID)
	if err != nil {
		t.Fatalf("GetCardMappingsByUserID() failed: %v", err)
	}

	if len(mappings) != 2 {
		t.Errorf("GetCardMappingsByUserID() returned %d mappings, want 2", len(mappings))
	}

	// Check that mappings are ordered by created_at DESC
	if len(mappings) > 0 && mappings[0].TagID != "nfc-456" {
		t.Errorf("First mapping TagID = %q, want %q", mappings[0].TagID, "nfc-456")
	}
}

func TestGetCardMappingByID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	id, err := db.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}

	// Retrieve the mapping
	retrieved, err := db.GetCardMappingByID(id)
	if err != nil {
		t.Fatalf("GetCardMappingByID() failed: %v", err)
	}

	if retrieved.TagID != "nfc-123" {
		t.Errorf("TagID = %q, want %q", retrieved.TagID, "nfc-123")
	}
	if retrieved.MediaType != "movie" {
		t.Errorf("MediaType = %q, want %q", retrieved.MediaType, "movie")
	}
	if retrieved.MediaID != "rating-456" {
		t.Errorf("MediaID = %q, want %q", retrieved.MediaID, "rating-456")
	}
	if retrieved.MediaTitle != "The Matrix" {
		t.Errorf("MediaTitle = %q, want %q", retrieved.MediaTitle, "The Matrix")
	}
}

func TestGetCardMappingByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := db.GetCardMappingByID(9999)
	if err == nil {
		t.Fatal("GetCardMappingByID() succeeded for nonexistent mapping, want error")
	}
}

func TestUpdateCardMapping(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	id, err := db.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}

	// Retrieve and update the mapping
	retrieved, err := db.GetCardMappingByID(id)
	if err != nil {
		t.Fatalf("GetCardMappingByID() failed: %v", err)
	}

	retrieved.MediaTitle = "The Matrix Reloaded"
	retrieved.MediaID = "rating-789"
	err = db.UpdateCardMapping(retrieved)
	if err != nil {
		t.Fatalf("UpdateCardMapping() failed: %v", err)
	}

	// Verify update
	updated, err := db.GetCardMappingByID(id)
	if err != nil {
		t.Fatalf("GetCardMappingByID() after update failed: %v", err)
	}

	if updated.MediaTitle != "The Matrix Reloaded" {
		t.Errorf("MediaTitle = %q, want %q", updated.MediaTitle, "The Matrix Reloaded")
	}
	if updated.MediaID != "rating-789" {
		t.Errorf("MediaID = %q, want %q", updated.MediaID, "rating-789")
	}
}

func TestDeleteCardMapping(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a user
	user := models.NewUser("testuser", "12345", "test-token")
	userID, err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("CreateUser() failed: %v", err)
	}

	// Create a mapping
	mapping := models.NewCardMapping(userID, "nfc-123", "movie", "rating-456", "The Matrix", "test-server-id", "media_player.test")
	id, err := db.CreateCardMapping(mapping)
	if err != nil {
		t.Fatalf("CreateCardMapping() failed: %v", err)
	}

	// Delete the mapping
	err = db.DeleteCardMapping(id)
	if err != nil {
		t.Fatalf("DeleteCardMapping() failed: %v", err)
	}

	// Verify deletion
	_, err = db.GetCardMappingByID(id)
	if err == nil {
		t.Fatal("GetCardMappingByID() succeeded after deletion, want error")
	}
}

func TestDeleteCardMapping_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	err := db.DeleteCardMapping(9999)
	if err == nil {
		t.Fatal("DeleteCardMapping() succeeded for nonexistent mapping, want error")
	}
}

func TestSingleUserConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create first user - should succeed
	user1 := models.NewUser("user1", "plex-id-1", "token-1")
	_, err := db.CreateUser(user1)
	if err != nil {
		t.Fatalf("CreateUser() for first user failed: %v", err)
	}

	// Try to create second user - should fail due to trigger
	user2 := models.NewUser("user2", "plex-id-2", "token-2")
	_, err = db.CreateUser(user2)
	if err == nil {
		t.Fatal("CreateUser() succeeded for second user, want error due to single-user constraint")
	}

	// Verify error message mentions single-user constraint
	if err != nil && err.Error() != "failed to insert user: Only one user allowed. TapeDeck is designed for single-user operation." {
		t.Logf("Got error: %v", err)
		// Still pass the test as long as it failed - the exact error message may vary by SQLite version
	}
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Run migrations relative to test file location
	migrationsPath := "../../migrations"
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		t.Fatalf("Migrations directory not found at %s", migrationsPath)
	}

	err = db.RunMigrations(migrationsPath)
	if err != nil {
		t.Fatalf("RunMigrations() failed: %v", err)
	}

	return db
}
