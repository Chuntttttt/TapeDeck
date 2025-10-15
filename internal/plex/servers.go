package plex

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/Chuntttttt/tapedeck/internal/config"
)

// ResourcesResponse represents the XML response from Plex API
type ResourcesResponse struct {
	XMLName   xml.Name   `xml:"resources"`
	Resources []Resource `xml:"resource"`
}

// Resource represents a Plex device (server or client) in the resources response
type Resource struct {
	Name             string             `xml:"name,attr"`
	ClientIdentifier string             `xml:"clientIdentifier,attr"`
	Owned            string             `xml:"owned,attr"` // "1" or "0"
	OwnerID          string             `xml:"ownerId,attr"`
	Provides         string             `xml:"provides,attr"` // e.g., "server", "player", etc.
	Connections      ConnectionsWrapper `xml:"connections"`
}

// ConnectionsWrapper wraps the list of connections
type ConnectionsWrapper struct {
	Connections []Connection `xml:"connection"`
}

// Connection represents a connection URL for a Plex server
type Connection struct {
	URI   string `xml:"uri,attr"`
	Local string `xml:"local,attr"` // "1" or "0"
}

// GetServers fetches all Plex servers the user has access to
func (c *AuthClient) GetServers(ctx context.Context, authToken string) ([]config.PlexServer, error) {
	// Build request
	url := fmt.Sprintf("%s/api/v2/resources?includeHttps=1&includeRelay=1", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add required Plex headers
	req.Header.Set("X-Plex-Token", authToken)
	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	req.Header.Set("X-Plex-Product", c.productName)
	req.Header.Set("X-Plex-Version", c.version)
	req.Header.Set("X-Plex-Platform", c.platform)
	req.Header.Set("X-Plex-Device", c.device)
	req.Header.Set("X-Plex-Device-Name", c.device)
	req.Header.Set("Accept", "application/xml")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plex API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Read response body for parsing
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse XML response
	var resources ResourcesResponse
	if err := xml.Unmarshal(body, &resources); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to config.PlexServer format
	var servers []config.PlexServer
	for _, resource := range resources.Resources {
		// Skip devices that aren't servers
		// Check if "provides" attribute contains "server"
		// (it might be "server" or "server,controller" etc.)
		if !containsProvides(resource.Provides, "server") {
			continue
		}

		// Skip if no connections
		if len(resource.Connections.Connections) == 0 {
			continue
		}

		// Convert connections
		var connections []config.Connection
		for _, conn := range resource.Connections.Connections {
			connections = append(connections, config.Connection{
				URI:   conn.URI,
				Local: conn.Local == "1",
			})
		}

		// Get owner name (for display purposes)
		// If owned by user, show as "You", otherwise show owner ID
		ownerName := "Shared"
		if resource.Owned == "1" {
			ownerName = "You"
		}

		servers = append(servers, config.PlexServer{
			ID:          resource.ClientIdentifier,
			Name:        resource.Name,
			Owner:       ownerName,
			Connections: connections,
		})
	}

	return servers, nil
}

// containsProvides checks if a provides attribute contains a specific value
// The provides attribute can be comma-separated like "server,controller"
func containsProvides(provides, value string) bool {
	if provides == "" {
		return false
	}
	parts := strings.Split(provides, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == value {
			return true
		}
	}
	return false
}
