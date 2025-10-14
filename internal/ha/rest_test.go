package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRestClient(t *testing.T) {
	client := NewRestClient("http://localhost:8123", "test-token", false)

	if client.baseURL != "http://localhost:8123" {
		t.Errorf("baseURL = %s, want http://localhost:8123", client.baseURL)
	}

	if client.token != "test-token" {
		t.Errorf("token = %s, want test-token", client.token)
	}

	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNewRestClient_DevMode(t *testing.T) {
	client := NewRestClient("https://localhost:8123", "test-token", true)

	if client.httpClient.Transport == nil {
		t.Error("Expected custom transport for dev mode")
	}
}

func TestPlayMedia_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}

		// Verify URL
		if r.URL.Path != "/api/services/media_player/play_media" {
			t.Errorf("Path = %s, want /api/services/media_player/play_media", r.URL.Path)
		}

		// Verify headers
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("Authorization = %s, want Bearer test-token", authHeader)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", contentType)
		}

		// Verify request body
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if payload["entity_id"] != "media_player.apple_tv" {
			t.Errorf("entity_id = %v, want media_player.apple_tv", payload["entity_id"])
		}

		if payload["media_content_type"] != "url" {
			t.Errorf("media_content_type = %v, want url", payload["media_content_type"])
		}

		if payload["media_content_id"] != "plex://play/?metadataKey=/library/metadata/12345&server=abc123" {
			t.Errorf("media_content_id = %v, want plex URL", payload["media_content_id"])
		}

		// Return success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"result": "success"}]`))
	}))
	defer server.Close()

	// Create client
	client := NewRestClient(server.URL, "test-token", false)

	// Call PlayMedia
	err := client.PlayMedia(
		"media_player.apple_tv",
		"url",
		"plex://play/?metadataKey=/library/metadata/12345&server=abc123",
	)

	if err != nil {
		t.Errorf("PlayMedia() failed: %v", err)
	}
}

func TestPlayMedia_Unauthorized(t *testing.T) {
	// Create test server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Unauthorized"}`))
	}))
	defer server.Close()

	// Create client
	client := NewRestClient(server.URL, "invalid-token", false)

	// Call PlayMedia
	err := client.PlayMedia("media_player.apple_tv", "url", "test-url")

	if err == nil {
		t.Error("PlayMedia() succeeded, want error")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Error = %v, want status code 401", err)
	}
}

func TestPlayMedia_NotFound(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Service not found"}`))
	}))
	defer server.Close()

	// Create client
	client := NewRestClient(server.URL, "test-token", false)

	// Call PlayMedia
	err := client.PlayMedia("media_player.invalid", "url", "test-url")

	if err == nil {
		t.Error("PlayMedia() succeeded, want error")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error = %v, want status code 404", err)
	}
}

func TestPlayMedia_BadGateway(t *testing.T) {
	// Create test server that returns 502
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message": "Bad Gateway"}`))
	}))
	defer server.Close()

	// Create client
	client := NewRestClient(server.URL, "test-token", false)

	// Call PlayMedia
	err := client.PlayMedia("media_player.apple_tv", "url", "test-url")

	if err == nil {
		t.Error("PlayMedia() succeeded, want error")
	}

	if !strings.Contains(err.Error(), "502") {
		t.Errorf("Error = %v, want status code 502", err)
	}
}

func TestPlayMedia_InvalidURL(t *testing.T) {
	// Create client with invalid URL
	client := NewRestClient("http://invalid-host-that-does-not-exist-12345.local", "test-token", false)

	// Call PlayMedia
	err := client.PlayMedia("media_player.apple_tv", "url", "test-url")

	if err == nil {
		t.Error("PlayMedia() succeeded, want error")
	}

	if !strings.Contains(err.Error(), "failed to make request") {
		t.Errorf("Error = %v, want 'failed to make request'", err)
	}
}

func TestPlayMedia_EmptyFields(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify empty strings are passed through
		if payload["entity_id"] != "" {
			t.Errorf("entity_id = %v, want empty string", payload["entity_id"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	client := NewRestClient(server.URL, "test-token", false)

	// Call PlayMedia with empty fields
	err := client.PlayMedia("", "", "")

	if err != nil {
		t.Errorf("PlayMedia() failed: %v", err)
	}
}

func TestPlayMedia_MultipleContentTypes(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		contentID   string
	}{
		{
			name:        "URL content type",
			contentType: "url",
			contentID:   "plex://play/?metadataKey=/library/metadata/12345&server=abc",
		},
		{
			name:        "music content type",
			contentType: "music",
			contentID:   "spotify://track/12345",
		},
		{
			name:        "video content type",
			contentType: "video",
			contentID:   "https://example.com/video.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}

				if payload["media_content_type"] != tt.contentType {
					t.Errorf("media_content_type = %v, want %v", payload["media_content_type"], tt.contentType)
				}

				if payload["media_content_id"] != tt.contentID {
					t.Errorf("media_content_id = %v, want %v", payload["media_content_id"], tt.contentID)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Create client
			client := NewRestClient(server.URL, "test-token", false)

			// Call PlayMedia
			err := client.PlayMedia("media_player.test", tt.contentType, tt.contentID)

			if err != nil {
				t.Errorf("PlayMedia() failed: %v", err)
			}
		})
	}
}
