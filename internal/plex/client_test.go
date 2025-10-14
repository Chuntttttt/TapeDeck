package plex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	serverURL := "http://localhost:32400"
	authToken := "test-token"

	client := NewClient(serverURL, "test-server-id", authToken, false)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.serverURL != serverURL {
		t.Errorf("serverURL = %s, want %s", client.serverURL, serverURL)
	}
	if client.authToken != authToken {
		t.Errorf("authToken = %s, want %s", client.authToken, authToken)
	}
}

func TestGetLibraries(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse LibrariesResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name: "successful library fetch",
			serverResponse: LibrariesResponse{
				MediaContainer: MediaContainer{
					Size: 3,
					Directory: []Library{
						{
							Key:   "1",
							Type:  "movie",
							Title: "Movies",
						},
						{
							Key:   "2",
							Type:  "show",
							Title: "TV Shows",
						},
						{
							Key:   "3",
							Type:  "artist",
							Title: "Music",
						},
					},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    3,
		},
		{
			name:         "unauthorized request",
			serverStatus: http.StatusUnauthorized,
			wantErr:      true,
			wantCount:    0,
		},
		{
			name: "empty libraries",
			serverResponse: LibrariesResponse{
				MediaContainer: MediaContainer{
					Size:      0,
					Directory: []Library{},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}
				if r.URL.Path != "/library/sections" {
					t.Errorf("Expected /library/sections path, got %s", r.URL.Path)
				}

				// Check auth token
				token := r.Header.Get("X-Plex-Token")
				if token == "" && tt.serverStatus == http.StatusOK {
					t.Error("Expected X-Plex-Token header")
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-server-id", "test-token", false)
			libraries, err := client.GetLibraries()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetLibraries() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(libraries) != tt.wantCount {
					t.Errorf("Got %d libraries, want %d", len(libraries), tt.wantCount)
				}

				// Verify library details
				if tt.wantCount > 0 {
					for i, lib := range libraries {
						expected := tt.serverResponse.MediaContainer.Directory[i]
						if lib.Key != expected.Key {
							t.Errorf("Library[%d].Key = %s, want %s", i, lib.Key, expected.Key)
						}
						if lib.Type != expected.Type {
							t.Errorf("Library[%d].Type = %s, want %s", i, lib.Type, expected.Type)
						}
						if lib.Title != expected.Title {
							t.Errorf("Library[%d].Title = %s, want %s", i, lib.Title, expected.Title)
						}
					}
				}
			}
		})
	}
}

func TestGetLibraryContents(t *testing.T) {
	tests := []struct {
		name           string
		libraryKey     string
		serverResponse LibraryContentsResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name:       "successful movie library fetch",
			libraryKey: "1",
			serverResponse: LibraryContentsResponse{
				MediaContainer: MediaItemContainer{
					Size: 2,
					Metadata: []MediaItem{
						{
							RatingKey: "123",
							Title:     "Test Movie",
							Type:      "movie",
							Year:      2023,
						},
						{
							RatingKey: "124",
							Title:     "Another Movie",
							Type:      "movie",
							Year:      2024,
						},
					},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name:         "library not found",
			libraryKey:   "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/library/sections/" + tt.libraryKey + "/all"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-server-id", "test-token", false)
			items, err := client.GetLibraryContents(tt.libraryKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetLibraryContents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(items) != tt.wantCount {
					t.Errorf("Got %d items, want %d", len(items), tt.wantCount)
				}

				if tt.wantCount > 0 {
					for i, item := range items {
						expected := tt.serverResponse.MediaContainer.Metadata[i]
						if item.RatingKey != expected.RatingKey {
							t.Errorf("Item[%d].RatingKey = %s, want %s", i, item.RatingKey, expected.RatingKey)
						}
						if item.Title != expected.Title {
							t.Errorf("Item[%d].Title = %s, want %s", i, item.Title, expected.Title)
						}
					}
				}
			}
		})
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		serverResponse SearchResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name:  "successful search",
			query: "test movie",
			serverResponse: SearchResponse{
				MediaContainer: MediaItemContainer{
					Size: 1,
					Metadata: []MediaItem{
						{
							RatingKey: "123",
							Title:     "Test Movie",
							Type:      "movie",
							Year:      2023,
						},
					},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    1,
		},
		{
			name:  "no results",
			query: "nonexistent",
			serverResponse: SearchResponse{
				MediaContainer: MediaItemContainer{
					Size:     0,
					Metadata: []MediaItem{},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/search" {
					t.Errorf("Expected /search path, got %s", r.URL.Path)
				}

				query := r.URL.Query().Get("query")
				if query != tt.query {
					t.Errorf("Expected query %s, got %s", tt.query, query)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-server-id", "test-token", false)
			results, err := client.Search(tt.query)

			if (err != nil) != tt.wantErr {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(results) != tt.wantCount {
					t.Errorf("Got %d results, want %d", len(results), tt.wantCount)
				}
			}
		})
	}
}
