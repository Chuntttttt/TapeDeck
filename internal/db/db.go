// Package db provides database connection and operations for TapeDeck.
package db

import (
	"database/sql"
	"fmt"

	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "modernc.org/sqlite"
)

// DB wraps the database connection and provides data access methods
type DB struct {
	conn *sql.DB
}

// New creates a new database connection to the SQLite database at the given path
func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// RunMigrations runs all pending database migrations from the given directory
func (db *DB) RunMigrations(migrationsPath string) error {
	driver, err := sqlite.WithInstance(db.conn, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"sqlite",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// CreateUser inserts a new user into the database and returns the user ID
func (db *DB) CreateUser(user *models.User) (int64, error) {
	if err := user.Validate(); err != nil {
		return 0, fmt.Errorf("invalid user: %w", err)
	}

	result, err := db.conn.Exec(
		`INSERT INTO users (plex_username, plex_user_id, plex_auth_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		user.PlexUsername,
		user.PlexUserID,
		user.PlexAuthToken,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// GetUserByPlexUserID retrieves a user by their Plex user ID
func (db *DB) GetUserByPlexUserID(plexUserID string) (*models.User, error) {
	user := &models.User{}
	err := db.conn.QueryRow(
		`SELECT id, plex_username, plex_user_id, plex_auth_token, created_at, updated_at
		FROM users WHERE plex_user_id = ?`,
		plexUserID,
	).Scan(
		&user.ID,
		&user.PlexUsername,
		&user.PlexUserID,
		&user.PlexAuthToken,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

// UpdateUser updates an existing user in the database
func (db *DB) UpdateUser(user *models.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	_, err := db.conn.Exec(
		`UPDATE users SET plex_username = ?, plex_auth_token = ?, updated_at = ?
		WHERE id = ?`,
		user.PlexUsername,
		user.PlexAuthToken,
		user.UpdatedAt,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}
