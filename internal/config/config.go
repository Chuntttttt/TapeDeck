// Package config provides application configuration loading and validation.
package config

import (
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
// Returns an error if SESSION_SECRET is missing.
func Load() (*Config, error) {
	// Try to load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		Port:          getEnvOrDefault("PORT", "3001"),
		DatabasePath:  getEnvOrDefault("DATABASE_PATH", "./data/tapedeck.db"),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
		DevMode:       os.Getenv("DEV_MODE") == "true",
	}

	// Validate SESSION_SECRET is set (critical for session security)
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("required environment variable SESSION_SECRET is not set")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
