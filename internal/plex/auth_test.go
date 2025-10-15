package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestPIN(t *testing.T) {
	tests := []struct {
		name           string
		clientID       string
		productName    string
		serverResponse PINResponse
		serverStatus   int
		wantErr        bool
	}{
		{
			name:        "successful PIN request",
			clientID:    "test-client-123",
			productName: "TapeDeck",
			serverResponse: PINResponse{
				ID:        12345,
				Code:      "ABC123",
				ExpiresIn: 900,
				CreatedAt: time.Now(),
			},
			serverStatus: http.StatusCreated,
			wantErr:      false,
		},
		{
			name:         "missing client ID",
			clientID:     "",
			productName:  "TapeDeck",
			serverStatus: http.StatusBadRequest,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.URL.Path != "/api/v2/pins" {
					t.Errorf("Expected /api/v2/pins path, got %s", r.URL.Path)
				}

				// Check headers
				if tt.clientID != "" {
					clientID := r.Header.Get("X-Plex-Client-Identifier")
					if clientID != tt.clientID {
						t.Errorf("Expected client ID %s, got %s", tt.clientID, clientID)
					}
					product := r.Header.Get("X-Plex-Product")
					if product != tt.productName {
						t.Errorf("Expected product %s, got %s", tt.productName, product)
					}
				}

				// Send response
				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusCreated {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			// Create client and make request
			client := NewAuthClient(server.URL, tt.clientID, tt.productName, false)
			ctx := context.Background()
			pin, err := client.RequestPIN(ctx)

			// Check results
			if (err != nil) != tt.wantErr {
				t.Errorf("RequestPIN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if pin.ID != tt.serverResponse.ID {
					t.Errorf("PIN ID = %d, want %d", pin.ID, tt.serverResponse.ID)
				}
				if pin.Code != tt.serverResponse.Code {
					t.Errorf("PIN Code = %s, want %s", pin.Code, tt.serverResponse.Code)
				}
				if pin.ExpiresIn != tt.serverResponse.ExpiresIn {
					t.Errorf("PIN ExpiresIn = %d, want %d", pin.ExpiresIn, tt.serverResponse.ExpiresIn)
				}
			}
		})
	}
}

func TestCheckPIN(t *testing.T) {
	tests := []struct {
		name           string
		pinID          int
		authToken      string
		serverResponse PINCheckResponse
		serverStatus   int
		wantErr        bool
	}{
		{
			name:  "PIN not yet authorized",
			pinID: 12345,
			serverResponse: PINCheckResponse{
				ID:        12345,
				Code:      "ABC123",
				AuthToken: "",
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:  "PIN authorized with token",
			pinID: 12345,
			serverResponse: PINCheckResponse{
				ID:        12345,
				Code:      "ABC123",
				AuthToken: "test-auth-token-xyz",
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "invalid PIN ID",
			pinID:        99999,
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
		{
			name:         "rate limited (429)",
			pinID:        12345,
			serverStatus: http.StatusTooManyRequests,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				expectedPath := "/api/v2/pins/12345"
				if tt.serverStatus == http.StatusNotFound || tt.serverStatus == http.StatusTooManyRequests {
					if tt.pinID == 99999 {
						expectedPath = "/api/v2/pins/99999"
					}
				}

				if r.URL.Path != expectedPath {
					t.Errorf("Expected %s path, got %s", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewAuthClient(server.URL, "test-client", "TapeDeck", false)
			ctx := context.Background()
			response, err := client.CheckPIN(ctx, tt.pinID)

			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPIN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if response.ID != tt.serverResponse.ID {
					t.Errorf("Response ID = %d, want %d", response.ID, tt.serverResponse.ID)
				}
				if response.AuthToken != tt.serverResponse.AuthToken {
					t.Errorf("AuthToken = %s, want %s", response.AuthToken, tt.serverResponse.AuthToken)
				}
			}
		})
	}
}

func TestGetAuthURL(t *testing.T) {
	tests := []struct {
		name           string
		clientID       string
		pinCode        string
		forwardURL     string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:       "with forward URL",
			clientID:   "test-client-123",
			pinCode:    "ABC123",
			forwardURL: "http://localhost:3001/auth/callback",
			wantContains: []string{
				"https://app.plex.tv/auth#!?",
				"clientID=test-client-123",
				"code=ABC123",
				"forwardUrl=",
			},
		},
		{
			name:       "without forward URL (polling mode)",
			clientID:   "test-client-456",
			pinCode:    "XYZ789",
			forwardURL: "",
			wantContains: []string{
				"https://app.plex.tv/auth#!?",
				"clientID=test-client-456",
				"code=XYZ789",
			},
			wantNotContain: []string{
				"forwardUrl=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewAuthClient("https://plex.tv", tt.clientID, "TapeDeck", false)
			url := client.GetAuthURL(tt.pinCode, tt.forwardURL)

			if url == "" {
				t.Error("GetAuthURL() returned empty string")
			}

			for _, want := range tt.wantContains {
				if !contains(url, want) {
					t.Errorf("URL should contain %q but doesn't. Got: %s", want, url)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if contains(url, notWant) {
					t.Errorf("URL should not contain %q but does. Got: %s", notWant, url)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
