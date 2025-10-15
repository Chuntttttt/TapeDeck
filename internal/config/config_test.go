package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	// Set all required environment variables
	setTestEnv(t, map[string]string{
		"PORT":           "3001",
		"DATABASE_PATH":  "./test.db",
		"LOG_LEVEL":      "info",
		"SESSION_SECRET": "test-secret-key",
		"DEV_MODE":       "true",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
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
	if !cfg.DevMode {
		t.Errorf("DevMode = %v, want %v", cfg.DevMode, true)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		envVars map[string]string
	}{
		{
			name:    "missing SESSION_SECRET",
			missing: "SESSION_SECRET",
			envVars: map[string]string{
				// SESSION_SECRET is the only required field
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
		"SESSION_SECRET": "test-secret",
		// Omit PORT, DATABASE_PATH, LOG_LEVEL, DEV_MODE to test defaults
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
	if cfg.DevMode != false {
		t.Errorf("DevMode default = %v, want %v", cfg.DevMode, false)
	}
}

// setTestEnv sets environment variables for testing and cleans them up
func setTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Clear all relevant env vars first
	allKeys := []string{
		"PORT", "DATABASE_PATH", "LOG_LEVEL", "SESSION_SECRET", "DEV_MODE",
	}
	for _, key := range allKeys {
		_ = os.Unsetenv(key)
	}

	// Set provided env vars
	for key, value := range envVars {
		_ = os.Setenv(key, value)
	}

	// Cleanup after test
	t.Cleanup(func() {
		for _, key := range allKeys {
			_ = os.Unsetenv(key)
		}
	})
}
