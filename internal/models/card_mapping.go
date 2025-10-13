// Package models defines data models for the TapeDeck application.
package models

import (
	"errors"
	"time"
)

// CardMapping represents a mapping between an NFC card and Plex media
type CardMapping struct {
	ID         int64
	UserID     int64
	TagID      string
	MediaType  string
	MediaID    string
	MediaTitle string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewCardMapping creates a new CardMapping with the given parameters
func NewCardMapping(userID int64, tagID, mediaType, mediaID, mediaTitle string) *CardMapping {
	now := time.Now()
	return &CardMapping{
		UserID:     userID,
		TagID:      tagID,
		MediaType:  mediaType,
		MediaID:    mediaID,
		MediaTitle: mediaTitle,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Validate checks that all required fields are present
func (cm *CardMapping) Validate() error {
	if cm.UserID <= 0 {
		return errors.New("user ID is required")
	}
	if cm.TagID == "" {
		return errors.New("tag ID is required")
	}
	if cm.MediaType == "" {
		return errors.New("media type is required")
	}
	if cm.MediaID == "" {
		return errors.New("media ID is required")
	}
	if cm.MediaTitle == "" {
		return errors.New("media title is required")
	}
	return nil
}
