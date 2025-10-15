// Package config provides application configuration loading and validation.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds application-level configuration from environment variables.
// User data (Plex servers, Home Assistant, Apple TVs) is stored in config.yml
// and loaded via LoadRuntimeConfig().
type Config struct {
	Port          string
	DatabasePath  string
	LogLevel      string
	SessionSecret string
	DevMode       bool
}

// Load reads configuration from environment variables and validates required fields.
// It attempts to load from a .env file if present, then reads from the environment.
// Generates and persists a random SESSION_SECRET if not set.
func Load() (*Config, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	// Load session secret from persisted file, or generate and persist if missing
	sessionSecret, err := loadSessionSecret()
	if err == nil && sessionSecret != "" {
		fmt.Println("Loaded SESSION_SECRET from .session_secret file")
	} else {
		// Generate random SESSION_SECRET and persist it
		randomSecret, err := generateRandomSecret(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate SESSION_SECRET: %w", err)
		}

		// Persist to file so sessions survive restarts
		if err := saveSessionSecret(randomSecret); err != nil {
			return nil, fmt.Errorf("failed to persist SESSION_SECRET: %w", err)
		}

		sessionSecret = randomSecret
		fmt.Println("Generated and saved SESSION_SECRET to .session_secret file")
	}

	cfg := &Config{
		Port:          getEnvOrDefault("PORT", "3001"),
		DatabasePath:  getEnvOrDefault("DATABASE_PATH", "./data/tapedeck.db"),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		SessionSecret: sessionSecret,
		DevMode:       os.Getenv("DEV_MODE") == "true",
	}

	return cfg, nil
}

// generateRandomSecret generates a cryptographically secure random hex string
func generateRandomSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// loadSessionSecret reads the session secret from .session_secret file
func loadSessionSecret() (string, error) {
	data, err := os.ReadFile(".session_secret")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// saveSessionSecret writes the session secret to .session_secret file with restricted permissions
func saveSessionSecret(secret string) error {
	// Write with 0600 permissions (owner read/write only)
	return os.WriteFile(".session_secret", []byte(secret), 0600)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
