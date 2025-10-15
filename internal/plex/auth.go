// Package plex provides Plex Media Server API client functionality.
package plex

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// AuthClientInterface defines the methods required for Plex authentication
type AuthClientInterface interface {
	RequestPIN() (*PINResponse, error)
	CheckPIN(pinID int) (*PINCheckResponse, error)
}

// AuthClient handles Plex authentication operations
type AuthClient struct {
	baseURL     string
	clientID    string
	productName string
	version     string
	platform    string
	device      string
	httpClient  *http.Client
}

// PINResponse represents the response from requesting a new PIN
type PINResponse struct {
	ID        int       `json:"id"`
	Code      string    `json:"code"`
	ExpiresIn int       `json:"expiresIn"`
	CreatedAt time.Time `json:"createdAt"`
}

// PINCheckResponse represents the response when checking a PIN's status
type PINCheckResponse struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

// NewAuthClient creates a new Plex authentication client
func NewAuthClient(baseURL, clientID, productName string, devMode bool) *AuthClient {
	client := &http.Client{Timeout: 10 * time.Second}

	// In dev mode, skip TLS verification (useful for macOS certificate issues)
	if devMode {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return &AuthClient{
		baseURL:     baseURL,
		clientID:    clientID,
		productName: productName,
		version:     "1.0.0",
		platform:    "Web",
		device:      "TapeDeck",
		httpClient:  client,
	}
}

// RequestPIN requests a new authentication PIN from Plex
func (c *AuthClient) RequestPIN() (*PINResponse, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("client ID is required")
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v2/pins", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	req.Header.Set("X-Plex-Product", c.productName)
	req.Header.Set("X-Plex-Version", c.version)
	req.Header.Set("X-Plex-Platform", c.platform)
	req.Header.Set("X-Plex-Device", c.device)
	req.Header.Set("X-Plex-Device-Name", c.device)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var pinResp PINResponse
	if err := json.NewDecoder(resp.Body).Decode(&pinResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &pinResp, nil
}

// CheckPIN checks the status of a PIN to see if it has been authorized
func (c *AuthClient) CheckPIN(pinID int) (*PINCheckResponse, error) {
	url := fmt.Sprintf("%s/api/v2/pins/%d", c.baseURL, pinID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	req.Header.Set("X-Plex-Product", c.productName)
	req.Header.Set("X-Plex-Version", c.version)
	req.Header.Set("X-Plex-Platform", c.platform)
	req.Header.Set("X-Plex-Device", c.device)
	req.Header.Set("X-Plex-Device-Name", c.device)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var checkResp PINCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &checkResp, nil
}

// GetAuthURL constructs the Plex authentication URL for user authorization
//
// NOTE: This method is currently unused due to Plex OAuth being broken for 3rd party apps
// in v4.152.0 (Sept 2025). We use PIN polling instead until Plex fixes their OAuth.
// See: https://forums.plex.tv/t/plex-oauth-authenticate-with-plex-broken-after-plex-web-update-v4-152-0/931098
//
// TODO: Re-enable forwardUrl redirect flow when Plex fixes OAuth
func (c *AuthClient) GetAuthURL(pinCode, forwardURL string) string {
	params := url.Values{}
	params.Add("clientID", c.clientID)
	params.Add("code", pinCode)

	// Only add forwardUrl for redirect flow
	if forwardURL != "" {
		params.Add("forwardUrl", forwardURL)
	}

	return fmt.Sprintf("https://app.plex.tv/auth#!?%s", params.Encode())
}
