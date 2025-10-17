package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	// Set environment variables
	setTestEnv(t, map[string]string{
		"PORT":      "3001",
		"DATA_DIR":  "./testdata",
		"LOG_LEVEL": "info",
		"DEV_MODE":  "true",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Port != "3001" {
		t.Errorf("Port = %q, want %q", cfg.Port, "3001")
	}
	if cfg.DataDir != "./testdata" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "./testdata")
	}
	// filepath.Join simplifies "./testdata" + "tapedeck.db" to "testdata/tapedeck.db"
	if cfg.DatabasePath() != "testdata/tapedeck.db" {
		t.Errorf("DatabasePath() = %q, want %q", cfg.DatabasePath(), "testdata/tapedeck.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	// SessionSecret should be auto-generated (64 character hex string)
	if len(cfg.SessionSecret) != 64 {
		t.Errorf("SessionSecret length = %d, want 64 (32 bytes hex encoded)", len(cfg.SessionSecret))
	}
	if !cfg.DevMode {
		t.Errorf("DevMode = %v, want %v", cfg.DevMode, true)
	}
}

func TestLoad_MissingSessionSecret(t *testing.T) {
	// When SESSION_SECRET is missing, it should generate a random one
	setTestEnv(t, map[string]string{
		// Omit SESSION_SECRET
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should have generated a random session secret
	if cfg.SessionSecret == "" {
		t.Error("SessionSecret is empty, expected random value")
	}

	// Should be a hex string of 64 characters (32 bytes)
	if len(cfg.SessionSecret) != 64 {
		t.Errorf("SessionSecret length = %d, want 64 (32 bytes hex encoded)", len(cfg.SessionSecret))
	}
}

func TestLoad_OptionalDefaults(t *testing.T) {
	// Omit PORT, DATA_DIR, LOG_LEVEL, DEV_MODE to test defaults
	setTestEnv(t, map[string]string{})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Port != "3001" {
		t.Errorf("Port default = %q, want %q", cfg.Port, "3001")
	}
	if cfg.DataDir != "." {
		t.Errorf("DataDir default = %q, want %q", cfg.DataDir, ".")
	}
	// filepath.Join simplifies "." + "tapedeck.db" to "tapedeck.db"
	if cfg.DatabasePath() != "tapedeck.db" {
		t.Errorf("DatabasePath() default = %q, want %q", cfg.DatabasePath(), "tapedeck.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.DevMode != false {
		t.Errorf("DevMode default = %v, want %v", cfg.DevMode, false)
	}
	// SessionSecret should still be auto-generated
	if len(cfg.SessionSecret) != 64 {
		t.Errorf("SessionSecret length = %d, want 64 (32 bytes hex encoded)", len(cfg.SessionSecret))
	}
}

// setTestEnv sets environment variables for testing and cleans them up
func setTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Clear all relevant env vars first
	allKeys := []string{
		"PORT", "DATA_DIR", "LOG_LEVEL", "DEV_MODE",
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
