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

// ValidateRedirectPath validates that a redirect path is safe for same-origin redirects.
// This prevents open redirect attacks where attackers could redirect users to external sites.
// Only allows relative paths starting with "/" but not "//" (which is a protocol-relative URL).
func ValidateRedirectPath(redirect string) (string, error) {
	if redirect == "" {
		return "/", nil
	}

	// Trim whitespace
	redirect = strings.TrimSpace(redirect)

	// Must start with /
	if !strings.HasPrefix(redirect, "/") {
		return "", fmt.Errorf("redirect must be a relative path starting with /")
	}

	// Must not start with // (protocol-relative URL like //evil.com)
	if strings.HasPrefix(redirect, "//") {
		return "", fmt.Errorf("redirect must not be a protocol-relative URL")
	}

	return redirect, nil
}
