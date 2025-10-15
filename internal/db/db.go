// Package db provides database connection and operations for TapeDeck.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file" // Register file source driver
	_ "modernc.org/sqlite"                               // Register SQLite driver
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
		if closeErr := conn.Close(); closeErr != nil {
			fmt.Printf("Failed to close connection during error: %v\n", closeErr)
		}
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
func (db *DB) CreateUser(ctx context.Context, user *models.User) (int64, error) {
	if err := user.Validate(); err != nil {
		return 0, fmt.Errorf("invalid user: %w", err)
	}

	result, err := db.conn.ExecContext(ctx,
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

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	user := &models.User{}
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, plex_username, plex_user_id, plex_auth_token, created_at, updated_at
		FROM users WHERE id = ?`,
		id,
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

// GetUserByPlexUserID retrieves a user by their Plex user ID
func (db *DB) GetUserByPlexUserID(ctx context.Context, plexUserID string) (*models.User, error) {
	user := &models.User{}
	err := db.conn.QueryRowContext(ctx,
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
func (db *DB) UpdateUser(ctx context.Context, user *models.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	_, err := db.conn.ExecContext(ctx,
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

// CreateCardMapping inserts a new card mapping into the database and returns the mapping ID
func (db *DB) CreateCardMapping(ctx context.Context, mapping *models.CardMapping) (int64, error) {
	if err := mapping.Validate(); err != nil {
		return 0, fmt.Errorf("invalid card mapping: %w", err)
	}

	result, err := db.conn.ExecContext(ctx,
		`INSERT INTO card_mappings (user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mapping.UserID,
		mapping.TagID,
		mapping.MediaType,
		mapping.MediaID,
		mapping.MediaTitle,
		mapping.PlexServerID,
		mapping.AppleTVEntity,
		mapping.CreatedAt,
		mapping.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert card mapping: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// GetCardMappingsByUserID retrieves all card mappings for a user
func (db *DB) GetCardMappingsByUserID(ctx context.Context, userID int64) ([]*models.CardMapping, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at
		FROM card_mappings WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query card mappings: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logger.Warn("Failed to close rows", "error", err)
		}
	}()

	var mappings []*models.CardMapping
	for rows.Next() {
		mapping := &models.CardMapping{}
		err := rows.Scan(
			&mapping.ID,
			&mapping.UserID,
			&mapping.TagID,
			&mapping.MediaType,
			&mapping.MediaID,
			&mapping.MediaTitle,
			&mapping.PlexServerID,
			&mapping.AppleTVEntity,
			&mapping.CreatedAt,
			&mapping.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan card mapping: %w", err)
		}
		mappings = append(mappings, mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return mappings, nil
}

// GetCardMappingByID retrieves a card mapping by its ID
func (db *DB) GetCardMappingByID(ctx context.Context, id int64) (*models.CardMapping, error) {
	mapping := &models.CardMapping{}
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at
		FROM card_mappings WHERE id = ?`,
		id,
	).Scan(
		&mapping.ID,
		&mapping.UserID,
		&mapping.TagID,
		&mapping.MediaType,
		&mapping.MediaID,
		&mapping.MediaTitle,
		&mapping.PlexServerID,
		&mapping.AppleTVEntity,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("card mapping not found")
		}
		return nil, fmt.Errorf("failed to query card mapping: %w", err)
	}

	return mapping, nil
}

// UpdateCardMapping updates an existing card mapping in the database
func (db *DB) UpdateCardMapping(ctx context.Context, mapping *models.CardMapping) error {
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid card mapping: %w", err)
	}

	_, err := db.conn.ExecContext(ctx,
		`UPDATE card_mappings SET tag_id = ?, media_type = ?, media_id = ?, media_title = ?, plex_server_id = ?, apple_tv_entity = ?, updated_at = ?
		WHERE id = ?`,
		mapping.TagID,
		mapping.MediaType,
		mapping.MediaID,
		mapping.MediaTitle,
		mapping.PlexServerID,
		mapping.AppleTVEntity,
		mapping.UpdatedAt,
		mapping.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update card mapping: %w", err)
	}

	return nil
}

// DeleteCardMapping deletes a card mapping by its ID
func (db *DB) DeleteCardMapping(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(ctx, `DELETE FROM card_mappings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete card mapping: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("card mapping not found")
	}

	return nil
}

// GetCardMappingByTagID retrieves a card mapping by its tag ID.
// Since TapeDeck enforces single-user operation and tag_id is unique per user,
// each tag_id is guaranteed to be globally unique.
func (db *DB) GetCardMappingByTagID(ctx context.Context, tagID string) (*models.CardMapping, error) {
	mapping := &models.CardMapping{}
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at
		FROM card_mappings WHERE tag_id = ?`,
		tagID,
	).Scan(
		&mapping.ID,
		&mapping.UserID,
		&mapping.TagID,
		&mapping.MediaType,
		&mapping.MediaID,
		&mapping.MediaTitle,
		&mapping.PlexServerID,
		&mapping.AppleTVEntity,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("card mapping not found")
		}
		return nil, fmt.Errorf("failed to query card mapping: %w", err)
	}

	return mapping, nil
}

// CreatePlaybackLog inserts a new playback log into the database and returns the log ID
func (db *DB) CreatePlaybackLog(ctx context.Context, log *models.PlaybackLog) (int64, error) {
	if err := log.Validate(); err != nil {
		return 0, fmt.Errorf("invalid playback log: %w", err)
	}

	result, err := db.conn.ExecContext(ctx,
		`INSERT INTO playback_logs (user_id, tag_id, media_id, media_title, played_at)
		VALUES (?, ?, ?, ?, ?)`,
		log.UserID,
		log.TagID,
		log.MediaID,
		log.MediaTitle,
		log.PlayedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert playback log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}
