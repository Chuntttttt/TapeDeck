package handlers

import (
	"testing"
)

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantURL   string
		wantError bool
	}{
		{
			name:      "valid http URL",
			input:     "http://homeassistant.local:8123",
			wantURL:   "http://homeassistant.local:8123",
			wantError: false,
		},
		{
			name:      "valid https URL",
			input:     "https://homeassistant.example.com",
			wantURL:   "https://homeassistant.example.com",
			wantError: false,
		},
		{
			name:      "http with trailing slash",
			input:     "http://homeassistant.local:8123/",
			wantURL:   "http://homeassistant.local:8123",
			wantError: false,
		},
		{
			name:      "https with path",
			input:     "https://example.com/homeassistant",
			wantURL:   "https://example.com/homeassistant",
			wantError: false,
		},
		{
			name:      "URL with whitespace",
			input:     "  https://homeassistant.local  ",
			wantURL:   "https://homeassistant.local",
			wantError: false,
		},
		{
			name:      "empty URL",
			input:     "",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "file:// scheme (SSRF prevention)",
			input:     "file:///etc/passwd",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "gopher:// scheme (SSRF prevention)",
			input:     "gopher://internal-service:70",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "ftp:// scheme",
			input:     "ftp://ftp.example.com",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "no scheme",
			input:     "homeassistant.local:8123",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "no host",
			input:     "http://",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "invalid URL characters",
			input:     "http://home assistant.local",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "javascript: scheme (XSS prevention)",
			input:     "javascript:alert(1)",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "data: scheme",
			input:     "data:text/html,<script>alert(1)</script>",
			wantURL:   "",
			wantError: true,
		},
		{
			name:      "HTTP uppercase scheme (normalized to lowercase)",
			input:     "HTTP://homeassistant.local",
			wantURL:   "http://homeassistant.local",
			wantError: false,
		},
		{
			name:      "localhost http",
			input:     "http://localhost:8123",
			wantURL:   "http://localhost:8123",
			wantError: false,
		},
		{
			name:      "IPv4 address",
			input:     "http://192.168.1.100:8123",
			wantURL:   "http://192.168.1.100:8123",
			wantError: false,
		},
		{
			name:      "IPv6 address",
			input:     "http://[::1]:8123",
			wantURL:   "http://[::1]:8123",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, err := ValidateHTTPURL(tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateHTTPURL() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateHTTPURL() unexpected error: %v", err)
				}
				if gotURL != tt.wantURL {
					t.Errorf("ValidateHTTPURL() = %q, want %q", gotURL, tt.wantURL)
				}
			}
		})
	}
}

func TestValidateRedirectPath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantPath  string
		wantError bool
	}{
		{
			name:      "valid root path",
			input:     "/",
			wantPath:  "/",
			wantError: false,
		},
		{
			name:      "valid nested path",
			input:     "/mappings",
			wantPath:  "/mappings",
			wantError: false,
		},
		{
			name:      "valid deep path",
			input:     "/mappings/123/edit",
			wantPath:  "/mappings/123/edit",
			wantError: false,
		},
		{
			name:      "empty string defaults to root",
			input:     "",
			wantPath:  "/",
			wantError: false,
		},
		{
			name:      "path with whitespace",
			input:     "  /settings  ",
			wantPath:  "/settings",
			wantError: false,
		},
		{
			name:      "path with query string",
			input:     "/auth/login?next=/mappings",
			wantPath:  "/auth/login?next=/mappings",
			wantError: false,
		},
		{
			name:      "protocol-relative URL (open redirect)",
			input:     "//evil.com",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "protocol-relative URL with path",
			input:     "//evil.com/phishing",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "absolute URL http",
			input:     "http://evil.com",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "absolute URL https",
			input:     "https://evil.com/phishing",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "relative path without leading slash",
			input:     "mappings/123",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "javascript URL",
			input:     "javascript:alert(1)",
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "data URL",
			input:     "data:text/html,<script>alert(1)</script>",
			wantPath:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := ValidateRedirectPath(tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateRedirectPath() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateRedirectPath() unexpected error: %v", err)
				}
				if gotPath != tt.wantPath {
					t.Errorf("ValidateRedirectPath() = %q, want %q", gotPath, tt.wantPath)
				}
			}
		})
	}
}
