package handlers

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateHTTPURL validates that a URL uses http or https scheme
// and returns a cleaned URL string. This prevents SSRF attacks via
// non-HTTP schemes like file://, gopher://, etc.
func ValidateHTTPURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("URL cannot be empty")
	}

	// Trim whitespace
	rawURL = strings.TrimSpace(rawURL)

	// Parse URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %w", err)
	}

	// Validate scheme
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("URL must use http or https scheme, got: %s", parsedURL.Scheme)
	}

	// Validate host is present
	if parsedURL.Host == "" {
		return "", fmt.Errorf("URL must include a host")
	}

	// Return cleaned URL (removes trailing slash, normalizes)
	return strings.TrimSuffix(parsedURL.String(), "/"), nil
}
