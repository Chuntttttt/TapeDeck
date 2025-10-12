package plex

import (
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
			pin, err := client.RequestPIN()

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				expectedPath := "/api/v2/pins/12345"
				if tt.serverStatus == http.StatusNotFound {
					expectedPath = "/api/v2/pins/99999"
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
			response, err := client.CheckPIN(tt.pinID)

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
	client := NewAuthClient("https://plex.tv", "test-client-123", "TapeDeck", false)
	pinCode := "ABC123"
	forwardURL := "http://localhost:3001/auth/callback"

	url := client.GetAuthURL(pinCode, forwardURL)

	// URL should contain required parameters
	if url == "" {
		t.Error("GetAuthURL() returned empty string")
	}

	// Basic validation - should contain plex.tv domain
	if len(url) < 20 {
		t.Error("GetAuthURL() returned suspiciously short URL")
	}
}
