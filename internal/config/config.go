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
	CSRFKey       []byte // 32-byte key for CSRF protection
	EncryptionKey []byte // 32-byte key for AES-256-GCM encryption
	DevMode       bool
	RequireTLS    bool
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

	// Load CSRF key from persisted file, or generate and persist if missing
	csrfKey, err := loadCSRFKey()
	if err == nil && len(csrfKey) == 32 {
		fmt.Println("Loaded CSRF_KEY from .csrf_key file")
	} else {
		// Generate random 32-byte key for CSRF protection
		csrfKey = make([]byte, 32)
		if _, err := rand.Read(csrfKey); err != nil {
			return nil, fmt.Errorf("failed to generate CSRF_KEY: %w", err)
		}

		// Persist to file so CSRF tokens remain valid after restarts
		if err := saveCSRFKey(csrfKey); err != nil {
			return nil, fmt.Errorf("failed to persist CSRF_KEY: %w", err)
		}

		fmt.Println("Generated and saved CSRF_KEY to .csrf_key file")
	}

	// Load encryption key from persisted file, or generate and persist if missing
	encryptionKey, err := loadEncryptionKey()
	if err == nil && len(encryptionKey) == 32 {
		fmt.Println("Loaded ENCRYPTION_KEY from .encryption_key file")
	} else {
		// Generate random 32-byte key for AES-256
		encryptionKey = make([]byte, 32)
		if _, err := rand.Read(encryptionKey); err != nil {
			return nil, fmt.Errorf("failed to generate ENCRYPTION_KEY: %w", err)
		}

		// Persist to file so encrypted data can be decrypted after restarts
		if err := saveEncryptionKey(encryptionKey); err != nil {
			return nil, fmt.Errorf("failed to persist ENCRYPTION_KEY: %w", err)
		}

		fmt.Println("Generated and saved ENCRYPTION_KEY to .encryption_key file")
	}

	cfg := &Config{
		Port:          getEnvOrDefault("PORT", "3001"),
		DatabasePath:  getEnvOrDefault("DATABASE_PATH", "./data/tapedeck.db"),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		SessionSecret: sessionSecret,
		CSRFKey:       csrfKey,
		EncryptionKey: encryptionKey,
		DevMode:       os.Getenv("DEV_MODE") == "true",
		RequireTLS:    os.Getenv("REQUIRE_TLS") == "true",
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

// loadEncryptionKey reads the encryption key from .encryption_key file
func loadEncryptionKey() ([]byte, error) {
	data, err := os.ReadFile(".encryption_key")
	if err != nil {
		return nil, err
	}
	// Decode from hex
	key := make([]byte, hex.DecodedLen(len(data)))
	n, err := hex.Decode(key, data)
	if err != nil {
		return nil, err
	}
	return key[:n], nil
}

// saveEncryptionKey writes the encryption key to .encryption_key file with restricted permissions
func saveEncryptionKey(key []byte) error {
	// Encode to hex for readability
	hexKey := make([]byte, hex.EncodedLen(len(key)))
	hex.Encode(hexKey, key)
	// Write with 0600 permissions (owner read/write only)
	return os.WriteFile(".encryption_key", hexKey, 0600)
}

// loadCSRFKey reads the CSRF key from .csrf_key file
func loadCSRFKey() ([]byte, error) {
	data, err := os.ReadFile(".csrf_key")
	if err != nil {
		return nil, err
	}
	// Decode from hex
	key := make([]byte, hex.DecodedLen(len(data)))
	n, err := hex.Decode(key, data)
	if err != nil {
		return nil, err
	}
	return key[:n], nil
}

// saveCSRFKey writes the CSRF key to .csrf_key file with restricted permissions
func saveCSRFKey(key []byte) error {
	// Encode to hex for readability
	hexKey := make([]byte, hex.EncodedLen(len(key)))
	hex.Encode(hexKey, key)
	// Write with 0600 permissions (owner read/write only)
	return os.WriteFile(".csrf_key", hexKey, 0600)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
