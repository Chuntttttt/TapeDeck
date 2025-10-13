package models

import (
	"testing"
)

func TestNewCardMapping(t *testing.T) {
	mapping := NewCardMapping(1, "nfc-123", "movie", "rating-456", "The Matrix")

	if mapping.UserID != 1 {
		t.Errorf("UserID = %d, want 1", mapping.UserID)
	}
	if mapping.TagID != "nfc-123" {
		t.Errorf("TagID = %q, want %q", mapping.TagID, "nfc-123")
	}
	if mapping.MediaType != "movie" {
		t.Errorf("MediaType = %q, want %q", mapping.MediaType, "movie")
	}
	if mapping.MediaID != "rating-456" {
		t.Errorf("MediaID = %q, want %q", mapping.MediaID, "rating-456")
	}
	if mapping.MediaTitle != "The Matrix" {
		t.Errorf("MediaTitle = %q, want %q", mapping.MediaTitle, "The Matrix")
	}
	if mapping.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if mapping.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestCardMappingValidate(t *testing.T) {
	tests := []struct {
		name    string
		mapping *CardMapping
		wantErr bool
	}{
		{
			name:    "valid mapping",
			mapping: NewCardMapping(1, "nfc-123", "movie", "rating-456", "The Matrix"),
			wantErr: false,
		},
		{
			name:    "missing user ID",
			mapping: NewCardMapping(0, "nfc-123", "movie", "rating-456", "The Matrix"),
			wantErr: true,
		},
		{
			name:    "missing tag ID",
			mapping: NewCardMapping(1, "", "movie", "rating-456", "The Matrix"),
			wantErr: true,
		},
		{
			name:    "missing media type",
			mapping: NewCardMapping(1, "nfc-123", "", "rating-456", "The Matrix"),
			wantErr: true,
		},
		{
			name:    "missing media ID",
			mapping: NewCardMapping(1, "nfc-123", "movie", "", "The Matrix"),
			wantErr: true,
		},
		{
			name:    "missing media title",
			mapping: NewCardMapping(1, "nfc-123", "movie", "rating-456", ""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
