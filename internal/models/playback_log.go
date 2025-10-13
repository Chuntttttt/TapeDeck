// Package models defines data models for the TapeDeck application.
package models

import (
	"errors"
	"time"
)

// PlaybackLog represents a media playback event triggered by an NFC card scan
type PlaybackLog struct {
	ID         int64
	UserID     int64
	TagID      string
	MediaID    string
	MediaTitle string
	PlayedAt   time.Time
}

// NewPlaybackLog creates a new PlaybackLog with the given parameters
func NewPlaybackLog(userID int64, tagID, mediaID, mediaTitle string) *PlaybackLog {
	return &PlaybackLog{
		UserID:     userID,
		TagID:      tagID,
		MediaID:    mediaID,
		MediaTitle: mediaTitle,
		PlayedAt:   time.Now(),
	}
}

// Validate checks that all required fields are present
func (pl *PlaybackLog) Validate() error {
	if pl.UserID <= 0 {
		return errors.New("user ID is required")
	}
	if pl.TagID == "" {
		return errors.New("tag ID is required")
	}
	if pl.MediaID == "" {
		return errors.New("media ID is required")
	}
	if pl.MediaTitle == "" {
		return errors.New("media title is required")
	}
	return nil
}
