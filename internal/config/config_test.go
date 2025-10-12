package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	// Set all required environment variables
	setTestEnv(t, map[string]string{
		"PLEX_URL":        "http://localhost:32400",
		"PLEX_SERVER_ID":  "test-server-id",
		"HA_URL":          "http://localhost:8123",
		"HA_TOKEN":        "test-token",
		"APPLE_TV_ENTITY": "media_player.apple_tv",
		"PORT":            "3001",
		"DATABASE_PATH":   "./test.db",
		"LOG_LEVEL":       "info",
		"SESSION_SECRET":  "test-secret-key",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.PlexURL != "http://localhost:32400" {
		t.Errorf("PlexURL = %q, want %q", cfg.PlexURL, "http://localhost:32400")
	}
	if cfg.PlexServerID != "test-server-id" {
		t.Errorf("PlexServerID = %q, want %q", cfg.PlexServerID, "test-server-id")
	}
	if cfg.HAURL != "http://localhost:8123" {
		t.Errorf("HAURL = %q, want %q", cfg.HAURL, "http://localhost:8123")
	}
	if cfg.HAToken != "test-token" {
		t.Errorf("HAToken = %q, want %q", cfg.HAToken, "test-token")
	}
	if cfg.AppleTVEntity != "media_player.apple_tv" {
		t.Errorf("AppleTVEntity = %q, want %q", cfg.AppleTVEntity, "media_player.apple_tv")
	}
	if cfg.Port != "3001" {
		t.Errorf("Port = %q, want %q", cfg.Port, "3001")
	}
	if cfg.DatabasePath != "./test.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "./test.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.SessionSecret != "test-secret-key" {
		t.Errorf("SessionSecret = %q, want %q", cfg.SessionSecret, "test-secret-key")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		envVars map[string]string
	}{
		{
			name:    "missing PLEX_URL",
			missing: "PLEX_URL",
			envVars: map[string]string{
				"PLEX_SERVER_ID":  "test-server-id",
				"HA_URL":          "http://localhost:8123",
				"HA_TOKEN":        "test-token",
				"APPLE_TV_ENTITY": "media_player.apple_tv",
				"SESSION_SECRET":  "test-secret",
			},
		},
		{
			name:    "missing PLEX_SERVER_ID",
			missing: "PLEX_SERVER_ID",
			envVars: map[string]string{
				"PLEX_URL":        "http://localhost:32400",
				"HA_URL":          "http://localhost:8123",
				"HA_TOKEN":        "test-token",
				"APPLE_TV_ENTITY": "media_player.apple_tv",
				"SESSION_SECRET":  "test-secret",
			},
		},
		{
			name:    "missing HA_URL",
			missing: "HA_URL",
			envVars: map[string]string{
				"PLEX_URL":        "http://localhost:32400",
				"PLEX_SERVER_ID":  "test-server-id",
				"HA_TOKEN":        "test-token",
				"APPLE_TV_ENTITY": "media_player.apple_tv",
				"SESSION_SECRET":  "test-secret",
			},
		},
		{
			name:    "missing HA_TOKEN",
			missing: "HA_TOKEN",
			envVars: map[string]string{
				"PLEX_URL":        "http://localhost:32400",
				"PLEX_SERVER_ID":  "test-server-id",
				"HA_URL":          "http://localhost:8123",
				"APPLE_TV_ENTITY": "media_player.apple_tv",
				"SESSION_SECRET":  "test-secret",
			},
		},
		{
			name:    "missing APPLE_TV_ENTITY",
			missing: "APPLE_TV_ENTITY",
			envVars: map[string]string{
				"PLEX_URL":       "http://localhost:32400",
				"PLEX_SERVER_ID": "test-server-id",
				"HA_URL":         "http://localhost:8123",
				"HA_TOKEN":       "test-token",
				"SESSION_SECRET": "test-secret",
			},
		},
		{
			name:    "missing SESSION_SECRET",
			missing: "SESSION_SECRET",
			envVars: map[string]string{
				"PLEX_URL":        "http://localhost:32400",
				"PLEX_SERVER_ID":  "test-server-id",
				"HA_URL":          "http://localhost:8123",
				"HA_TOKEN":        "test-token",
				"APPLE_TV_ENTITY": "media_player.apple_tv",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestEnv(t, tt.envVars)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() succeeded, want error")
			}
		})
	}
}

func TestLoad_OptionalDefaults(t *testing.T) {
	setTestEnv(t, map[string]string{
		"PLEX_URL":        "http://localhost:32400",
		"PLEX_SERVER_ID":  "test-server-id",
		"HA_URL":          "http://localhost:8123",
		"HA_TOKEN":        "test-token",
		"APPLE_TV_ENTITY": "media_player.apple_tv",
		"SESSION_SECRET":  "test-secret",
		// Omit PORT, DATABASE_PATH, LOG_LEVEL to test defaults
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Port != "3001" {
		t.Errorf("Port default = %q, want %q", cfg.Port, "3001")
	}
	if cfg.DatabasePath != "./data/tapedeck.db" {
		t.Errorf("DatabasePath default = %q, want %q", cfg.DatabasePath, "./data/tapedeck.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want %q", cfg.LogLevel, "info")
	}
}

// setTestEnv sets environment variables for testing and cleans them up
func setTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Clear all relevant env vars first
	allKeys := []string{
		"PLEX_URL", "PLEX_SERVER_ID", "HA_URL", "HA_TOKEN",
		"APPLE_TV_ENTITY", "PORT", "DATABASE_PATH", "LOG_LEVEL", "SESSION_SECRET",
	}
	for _, key := range allKeys {
		os.Unsetenv(key)
	}

	// Set provided env vars
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Cleanup after test
	t.Cleanup(func() {
		for _, key := range allKeys {
			os.Unsetenv(key)
		}
	})
}
