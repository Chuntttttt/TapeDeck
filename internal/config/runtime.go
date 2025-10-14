package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuntimeConfig represents the user-configurable runtime settings
type RuntimeConfig struct {
	Version       int          `yaml:"version"`
	PlexServers   []PlexServer `yaml:"plex_servers"`
	HomeAssistant HAConfig     `yaml:"home_assistant"`
	AppleTVs      []AppleTV    `yaml:"apple_tvs"`
}

// PlexServer represents a Plex Media Server configuration
type PlexServer struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Owner       string       `yaml:"owner"`
	Connections []Connection `yaml:"connections"`
}

// Connection represents a Plex server connection URL
type Connection struct {
	URI   string `yaml:"uri"`
	Local bool   `yaml:"local"`
}

// HAConfig represents Home Assistant configuration
type HAConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// AppleTV represents an Apple TV media player
type AppleTV struct {
	Entity  string `yaml:"entity"`
	Name    string `yaml:"name"`
	Default bool   `yaml:"default"`
}

// LoadRuntimeConfig loads configuration from a YAML file
func LoadRuntimeConfig(path string) (*RuntimeConfig, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create default empty config
		cfg := &RuntimeConfig{
			Version:       1,
			PlexServers:   []PlexServer{},
			HomeAssistant: HAConfig{},
			AppleTVs:      []AppleTV{},
		}
		return cfg, nil
	}

	// Read file
	data, err := os.ReadFile(path) //nolint:gosec // G304: Path is controlled, comes from constant
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg RuntimeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to a YAML file
func (c *RuntimeConfig) Save(path string) error {
	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with restricted permissions (owner read/write only)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// IsEmpty returns true if the configuration has no servers or HA configured
func (c *RuntimeConfig) IsEmpty() bool {
	return len(c.PlexServers) == 0 || c.HomeAssistant.URL == "" || c.HomeAssistant.Token == ""
}

// Validate checks if the configuration is valid
func (c *RuntimeConfig) Validate() error {
	// Check version
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version: %d", c.Version)
	}

	// Check Plex servers
	if len(c.PlexServers) == 0 {
		return fmt.Errorf("no Plex servers configured")
	}

	for i, server := range c.PlexServers {
		if server.ID == "" {
			return fmt.Errorf("plex server %d missing ID", i)
		}
		if server.Name == "" {
			return fmt.Errorf("plex server %d missing name", i)
		}
		if len(server.Connections) == 0 {
			return fmt.Errorf("plex server %s has no connections", server.Name)
		}
	}

	// Check Home Assistant
	if c.HomeAssistant.URL == "" {
		return fmt.Errorf("home Assistant URL not configured")
	}
	if c.HomeAssistant.Token == "" {
		return fmt.Errorf("home Assistant token not configured")
	}

	// Apple TVs are optional, but validate if present
	for i, tv := range c.AppleTVs {
		if tv.Entity == "" {
			return fmt.Errorf("apple TV %d missing entity ID", i)
		}
		if tv.Name == "" {
			return fmt.Errorf("apple TV %d missing name", i)
		}
	}

	return nil
}

// GetDefaultAppleTV returns the default Apple TV, or the first one if none is marked default
func (c *RuntimeConfig) GetDefaultAppleTV() *AppleTV {
	if len(c.AppleTVs) == 0 {
		return nil
	}

	// Look for explicitly marked default
	for i := range c.AppleTVs {
		if c.AppleTVs[i].Default {
			return &c.AppleTVs[i]
		}
	}

	// Return first one
	return &c.AppleTVs[0]
}

// GetPlexServerByID finds a Plex server by its ID
func (c *RuntimeConfig) GetPlexServerByID(id string) *PlexServer {
	for i := range c.PlexServers {
		if c.PlexServers[i].ID == id {
			return &c.PlexServers[i]
		}
	}
	return nil
}

// GetAppleTVByEntity finds an Apple TV by its entity ID
func (c *RuntimeConfig) GetAppleTVByEntity(entity string) *AppleTV {
	for i := range c.AppleTVs {
		if c.AppleTVs[i].Entity == entity {
			return &c.AppleTVs[i]
		}
	}
	return nil
}
