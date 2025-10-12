package models

import (
	"testing"
	"time"
)

func TestUser_Validate(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name: "valid user",
			user: User{
				ID:            1,
				PlexUsername:  "testuser",
				PlexUserID:    "12345",
				PlexAuthToken: "test-token",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing plex username",
			user: User{
				PlexUserID:    "12345",
				PlexAuthToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "missing plex user ID",
			user: User{
				PlexUsername:  "testuser",
				PlexAuthToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "missing plex auth token",
			user: User{
				PlexUsername: "testuser",
				PlexUserID:   "12345",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.user.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("User.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUser(t *testing.T) {
	username := "testuser"
	userID := "12345"
	token := "test-token"

	user := NewUser(username, userID, token)

	if user.PlexUsername != username {
		t.Errorf("PlexUsername = %q, want %q", user.PlexUsername, username)
	}
	if user.PlexUserID != userID {
		t.Errorf("PlexUserID = %q, want %q", user.PlexUserID, userID)
	}
	if user.PlexAuthToken != token {
		t.Errorf("PlexAuthToken = %q, want %q", user.PlexAuthToken, token)
	}
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if user.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
	if !user.CreatedAt.Equal(user.UpdatedAt) {
		t.Error("CreatedAt and UpdatedAt should be equal for new user")
	}
}
