package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckWebSocketOrigin_NoOriginHeader(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	// No Origin header

	if !handler.checkWebSocketOrigin(req) {
		t.Error("Expected to allow connection without Origin header")
	}
}

func TestCheckWebSocketOrigin_SameOrigin_HTTP(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "localhost:3001"
	req.Header.Set("Origin", "http://localhost:3001")

	if !handler.checkWebSocketOrigin(req) {
		t.Error("Expected to allow same-origin connection")
	}
}

func TestCheckWebSocketOrigin_SameOrigin_HTTPS(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "tapedeck.example.com"
	req.Header.Set("Origin", "https://tapedeck.example.com")
	req.TLS = &tls.ConnectionState{} // Mark as HTTPS

	if !handler.checkWebSocketOrigin(req) {
		t.Error("Expected to allow same-origin HTTPS connection")
	}
}

func TestCheckWebSocketOrigin_SameOrigin_IP(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "192.168.1.100:3001"
	req.Header.Set("Origin", "http://192.168.1.100:3001")

	if !handler.checkWebSocketOrigin(req) {
		t.Error("Expected to allow same-origin connection via IP")
	}
}

func TestCheckWebSocketOrigin_CrossOrigin_Blocked(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "localhost:3001"
	req.Header.Set("Origin", "http://evil-site.com")

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block cross-origin connection")
	}
}

func TestCheckWebSocketOrigin_DifferentPort_Blocked(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "localhost:3001"
	req.Header.Set("Origin", "http://localhost:3002")

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block connection from different port in production")
	}
}

func TestCheckWebSocketOrigin_DevMode_LocalhostAnyPort(t *testing.T) {
	handler := &PairingHandler{devMode: true}

	tests := []struct {
		name   string
		host   string
		origin string
	}{
		{
			name:   "localhost different port",
			host:   "localhost:3001",
			origin: "http://localhost:3002",
		},
		{
			name:   "127.0.0.1 different port",
			host:   "127.0.0.1:3001",
			origin: "http://127.0.0.1:3002",
		},
		{
			name:   "localhost no port",
			host:   "localhost:3001",
			origin: "http://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
			req.Host = tt.host
			req.Header.Set("Origin", tt.origin)

			if !handler.checkWebSocketOrigin(req) {
				t.Errorf("Expected to allow %s in dev mode", tt.origin)
			}
		})
	}
}

func TestCheckWebSocketOrigin_DevMode_NonLocalhost_Blocked(t *testing.T) {
	handler := &PairingHandler{devMode: true}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "192.168.1.100:3001"
	req.Header.Set("Origin", "http://evil-site.com")

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block non-localhost cross-origin even in dev mode")
	}
}

func TestCheckWebSocketOrigin_DevMode_DifferentIP_Blocked(t *testing.T) {
	handler := &PairingHandler{devMode: true}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "192.168.1.100:3001"
	req.Header.Set("Origin", "http://192.168.1.101:3001")

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block different IP even in dev mode")
	}
}

func TestCheckWebSocketOrigin_InvalidOriginHeader(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "localhost:3001"
	req.Header.Set("Origin", "not-a-valid-url:::///")

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block connection with invalid Origin header")
	}
}

func TestCheckWebSocketOrigin_MixedProtocol_Blocked(t *testing.T) {
	handler := &PairingHandler{devMode: false}

	req := httptest.NewRequest(http.MethodGet, "/ws/pairing", nil)
	req.Host = "localhost:3001"
	req.Header.Set("Origin", "https://localhost:3001")
	// req.TLS is nil, so server is HTTP

	if handler.checkWebSocketOrigin(req) {
		t.Error("Expected to block HTTPS origin on HTTP server")
	}
}
