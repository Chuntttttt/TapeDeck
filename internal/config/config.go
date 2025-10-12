// Package config provides application configuration loading and validation.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	PlexURL       string
	PlexServerID  string
	HAURL         string
	HAToken       string
	AppleTVEntity string
	Port          string
	DatabasePath  string
	LogLevel      string
	SessionSecret string
}

// Load reads configuration from environment variables and validates required fields.
// It attempts to load from a .env file if present, then reads from the environment.
// Returns an error if any required field is missing.
func Load() (*Config, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		PlexURL:       os.Getenv("PLEX_URL"),
		PlexServerID:  os.Getenv("PLEX_SERVER_ID"),
		HAURL:         os.Getenv("HA_URL"),
		HAToken:       os.Getenv("HA_TOKEN"),
		AppleTVEntity: os.Getenv("APPLE_TV_ENTITY"),
		Port:          getEnvOrDefault("PORT", "3001"),
		DatabasePath:  getEnvOrDefault("DATABASE_PATH", "./data/tapedeck.db"),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
	}

	// Validate required fields
	required := map[string]string{
		"PLEX_URL":        cfg.PlexURL,
		"PLEX_SERVER_ID":  cfg.PlexServerID,
		"HA_URL":          cfg.HAURL,
		"HA_TOKEN":        cfg.HAToken,
		"APPLE_TV_ENTITY": cfg.AppleTVEntity,
		"SESSION_SECRET":  cfg.SessionSecret,
	}

	for name, value := range required {
		if value == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", name)
		}
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
