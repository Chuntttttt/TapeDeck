package plex

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/Chuntttttt/tapedeck/internal/logger"
)

// APIError represents an error response from the Plex API
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("plex API error (status %d): %s", e.StatusCode, e.Message)
}

// IsUnauthorized checks if an error is a 401 Unauthorized from Plex
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized
	}
	return false
}

// Client handles Plex Media Server API operations
type Client struct {
	serverURL  string
	serverID   string
	authToken  string
	httpClient *http.Client
}

// Library represents a Plex library section
type Library struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// MediaContainer wraps library sections
type MediaContainer struct {
	Size      int       `json:"size"`
	Directory []Library `json:"Directory"`
}

// LibrariesResponse represents the response from /library/sections
type LibrariesResponse struct {
	MediaContainer MediaContainer `json:"MediaContainer"`
}

// MediaItem represents a media item (movie, show, episode, etc.)
type MediaItem struct {
	RatingKey  string `json:"ratingKey"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Year       int    `json:"year,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Thumb      string `json:"thumb,omitempty"`
	ServerName string `json:"-"` // Not from JSON, populated by handler
	ServerID   string `json:"-"` // Not from JSON, populated by handler
}

// MediaItemContainer wraps media items
type MediaItemContainer struct {
	Size     int         `json:"size"`
	Metadata []MediaItem `json:"Metadata"`
}

// LibraryContentsResponse represents the response from /library/sections/{id}/all
type LibraryContentsResponse struct {
	MediaContainer MediaItemContainer `json:"MediaContainer"`
}

// SearchResponse represents the response from /search
type SearchResponse struct {
	MediaContainer MediaItemContainer `json:"MediaContainer"`
}

// NewClient creates a new Plex API client
func NewClient(serverURL, serverID, authToken string, devMode bool) *Client {
	client := &http.Client{Timeout: constants.PlexClientTimeout}

	// In dev mode, skip TLS verification (useful for macOS certificate issues)
	if devMode {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return &Client{
		serverURL:  serverURL,
		serverID:   serverID,
		authToken:  authToken,
		httpClient: client,
	}
}

// GetLibraries retrieves all library sections from the Plex server
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/library/sections", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Token", c.authToken)
	req.Header.Set("Accept", "application/json")

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
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    "failed to get libraries",
		}
	}

	var libResp LibrariesResponse
	if err := json.NewDecoder(resp.Body).Decode(&libResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return libResp.MediaContainer.Directory, nil
}

// GetLibraryContents retrieves all items from a specific library
func (c *Client) GetLibraryContents(ctx context.Context, libraryKey string) ([]MediaItem, error) {
	url := fmt.Sprintf("%s/library/sections/%s/all", c.serverURL, libraryKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Token", c.authToken)
	req.Header.Set("Accept", "application/json")

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
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    "failed to get library contents",
		}
	}

	var contentsResp LibraryContentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&contentsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return contentsResp.MediaContainer.Metadata, nil
}

// Search performs a search across all libraries
// TODO: Shared servers may return 401 Unauthorized when accessed via direct .plex.direct URLs
// even with a valid auth token. This appears to be a Plex permission limitation for shared content.
// Consider investigating alternative access methods or documenting this as a known limitation.
func (c *Client) Search(ctx context.Context, query string) ([]MediaItem, error) {
	searchURL := fmt.Sprintf("%s/search", c.serverURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameter
	q := req.URL.Query()
	q.Add("query", query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-Plex-Token", c.authToken)
	req.Header.Set("Accept", "application/json")

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
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    "failed to search",
		}
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return searchResp.MediaContainer.Metadata, nil
}
