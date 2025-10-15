// Package ha provides Home Assistant integration for TapeDeck.
package ha

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/logger"
)

// RestClient handles REST API calls to Home Assistant
type RestClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	devMode    bool
}

// NewRestClient creates a new Home Assistant REST API client
func NewRestClient(haURL, token string, devMode bool) *RestClient {
	client := &http.Client{Timeout: constants.HARestTimeout}

	// In dev mode, skip TLS verification (useful for self-signed certificates)
	if devMode {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return &RestClient{
		baseURL:    haURL,
		token:      token,
		httpClient: client,
		devMode:    devMode,
	}
}

// EntityState represents the state of a Home Assistant entity
type EntityState struct {
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// GetEntityState retrieves the current state of an entity
func (c *RestClient) GetEntityState(ctx context.Context, entityID string) (string, error) {
	url := fmt.Sprintf("%s/api/states/%s", c.baseURL, entityID)

	if c.devMode {
		logger.Debug("Getting entity state", "url", url, "entity_id", entityID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var state EntityState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if c.devMode {
		logger.Debug("Entity state retrieved", "state", state.State)
	}

	return state.State, nil
}

// TurnOn calls the media_player.turn_on service
func (c *RestClient) TurnOn(ctx context.Context, entityID string) error {
	url := fmt.Sprintf("%s/api/services/media_player/turn_on", c.baseURL)

	payload := map[string]interface{}{
		"entity_id": entityID,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.devMode {
		logger.Debug("Calling turn_on service", "url", url, "entity_id", entityID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if c.devMode {
		logger.Debug("Turn_on response received", "status_code", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// PlayMedia calls the media_player.play_media service in Home Assistant
func (c *RestClient) PlayMedia(ctx context.Context, entityID, contentType, contentID string) error {
	url := fmt.Sprintf("%s/api/services/media_player/play_media", c.baseURL)

	// Build request body
	payload := map[string]interface{}{
		"entity_id":          entityID,
		"media_content_type": contentType,
		"media_content_id":   contentID,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	if c.devMode {
		logger.Debug("Calling play_media service", "url", url, "request_body", string(jsonData))
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if c.devMode {
		body, _ := io.ReadAll(resp.Body)
		logger.Debug("Play_media response received", "status_code", resp.StatusCode, "response_body", string(body))

		// Need to check status after reading body
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}
	} else if resp.StatusCode != http.StatusOK {
		// Check response status (non-dev mode)
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Entity represents a Home Assistant entity with its state and attributes
type Entity struct {
	EntityID   string                 `json:"entity_id"`
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// GetStates retrieves all entity states from Home Assistant
func (c *RestClient) GetStates(ctx context.Context) ([]Entity, error) {
	url := fmt.Sprintf("%s/api/states", c.baseURL)

	if c.devMode {
		logger.Debug("Getting all entity states", "url", url)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var entities []Entity
	if err := json.NewDecoder(resp.Body).Decode(&entities); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if c.devMode {
		logger.Debug("Entity states retrieved", "count", len(entities))
	}

	return entities, nil
}

// GetMediaPlayers retrieves all media_player entities from Home Assistant
func (c *RestClient) GetMediaPlayers(ctx context.Context) ([]Entity, error) {
	// Get all states
	allEntities, err := c.GetStates(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to media_player entities
	var mediaPlayers []Entity
	for _, entity := range allEntities {
		if len(entity.EntityID) > 13 && entity.EntityID[:13] == "media_player." {
			mediaPlayers = append(mediaPlayers, entity)
		}
	}

	if c.devMode {
		logger.Debug("Media players filtered", "count", len(mediaPlayers))
	}

	return mediaPlayers, nil
}

// GetFriendlyName extracts the friendly_name from entity attributes, or returns entity_id if not found
func (e *Entity) GetFriendlyName() string {
	if friendlyName, ok := e.Attributes["friendly_name"].(string); ok {
		return friendlyName
	}
	return e.EntityID
}
