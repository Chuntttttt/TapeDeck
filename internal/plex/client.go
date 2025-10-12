package plex

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client handles Plex Media Server API operations
type Client struct {
	serverURL  string
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
	RatingKey string `json:"ratingKey"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Year      int    `json:"year,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Thumb     string `json:"thumb,omitempty"`
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
func NewClient(serverURL, authToken string, devMode bool) *Client {
	client := &http.Client{Timeout: 30 * time.Second}

	// In dev mode, skip TLS verification (useful for macOS certificate issues)
	if devMode {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return &Client{
		serverURL:  serverURL,
		authToken:  authToken,
		httpClient: client,
	}
}

// GetLibraries retrieves all library sections from the Plex server
func (c *Client) GetLibraries() ([]Library, error) {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/library/sections", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Token", c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var libResp LibrariesResponse
	if err := json.NewDecoder(resp.Body).Decode(&libResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return libResp.MediaContainer.Directory, nil
}

// GetLibraryContents retrieves all items from a specific library
func (c *Client) GetLibraryContents(libraryKey string) ([]MediaItem, error) {
	url := fmt.Sprintf("%s/library/sections/%s/all", c.serverURL, libraryKey)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Token", c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var contentsResp LibraryContentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&contentsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return contentsResp.MediaContainer.Metadata, nil
}

// Search performs a search across all libraries
func (c *Client) Search(query string) ([]MediaItem, error) {
	searchURL := fmt.Sprintf("%s/search", c.serverURL)

	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return searchResp.MediaContainer.Metadata, nil
}
