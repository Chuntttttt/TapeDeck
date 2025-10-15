package models

import (
	"errors"
	"time"
)

// Settings holds application settings (singleton table with id=1)
type Settings struct {
	ID        int64
	HAToken   string // Encrypted Home Assistant long-lived access token
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSettings creates a new Settings instance
func NewSettings(haToken string) *Settings {
	now := time.Now()
	return &Settings{
		ID:        1, // Always 1 (singleton)
		HAToken:   haToken,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate checks if the settings data is valid
func (s *Settings) Validate() error {
	if s.HAToken == "" {
		return errors.New("ha_token is required")
	}
	return nil
}
