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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

	_, err := db.GetUserByPlexUserID("nonexistent")
	if err == nil {
		t.Fatal("GetUserByPlexUserID() succeeded for nonexistent user, want error")
	}
}

func TestUpdateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

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
