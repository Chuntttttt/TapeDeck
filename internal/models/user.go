// Package models defines data models for the TapeDeck application.
package models

import (
	"errors"
	"time"
)

// User represents a TapeDeck user authenticated via Plex
type User struct {
	ID            int64
	PlexUsername  string
	PlexUserID    string
	PlexAuthToken string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewUser creates a new User with the given Plex credentials
func NewUser(username, userID, authToken string) *User {
	now := time.Now()
	return &User{
		PlexUsername:  username,
		PlexUserID:    userID,
		PlexAuthToken: authToken,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Validate checks that all required fields are present
func (u *User) Validate() error {
	if u.PlexUsername == "" {
		return errors.New("plex username is required")
	}
	if u.PlexUserID == "" {
		return errors.New("plex user ID is required")
	}
	if u.PlexAuthToken == "" {
		return errors.New("plex auth token is required")
	}
	return nil
}
