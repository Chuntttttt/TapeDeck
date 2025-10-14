// Package ha provides Home Assistant integration for TapeDeck.
package ha

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RestClient handles REST API calls to Home Assistant
type RestClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewRestClient creates a new Home Assistant REST API client
func NewRestClient(haURL, token string, devMode bool) *RestClient {
	client := &http.Client{Timeout: 30 * time.Second}

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
	}
}

// EntityState represents the state of a Home Assistant entity
type EntityState struct {
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// GetEntityState retrieves the current state of an entity
func (c *RestClient) GetEntityState(entityID string) (string, error) {
	url := fmt.Sprintf("%s/api/states/%s", c.baseURL, entityID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var state EntityState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return state.State, nil
}

// TurnOn calls the media_player.turn_on service
func (c *RestClient) TurnOn(entityID string) error {
	url := fmt.Sprintf("%s/api/services/media_player/turn_on", c.baseURL)

	payload := map[string]interface{}{
		"entity_id": entityID,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// PlayMedia calls the media_player.play_media service in Home Assistant
func (c *RestClient) PlayMedia(entityID, contentType, contentID string) error {
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
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
