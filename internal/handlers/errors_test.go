package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRespondError_WithJSONAcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	RespondError(w, req, "Test error message", http.StatusBadRequest)

	// Verify status code
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	// Verify Content-Type is JSON
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain 'application/json', got '%s'", contentType)
	}

	// Verify response body contains error message as JSON
	body := w.Body.String()
	if !strings.Contains(body, "Test error message") {
		t.Errorf("Expected body to contain error message, got: %s", body)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("Expected body to contain 'error' field, got: %s", body)
	}
}

func TestRespondError_WithHTMLAcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	RespondError(w, req, "Test error message", http.StatusNotFound)

	// Verify status code
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
	}

	// Verify Content-Type is HTML
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html', got '%s'", contentType)
	}

	// Verify response body contains error message as HTML
	body := w.Body.String()
	if !strings.Contains(body, "Test error message") {
		t.Errorf("Expected body to contain error message, got: %s", body)
	}
	// Check for HTML doctype (templ generates lowercase)
	bodyLower := strings.ToLower(body)
	if !strings.Contains(bodyLower, "<!doctype html>") {
		t.Errorf("Expected body to contain HTML doctype, got: %s", body)
	}
}

func TestRespondError_WithoutAcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	RespondError(w, req, "Test error message", http.StatusInternalServerError)

	// Verify status code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Default to HTML when no Accept header
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html' for default, got '%s'", contentType)
	}

	// Verify response body contains error message
	body := w.Body.String()
	if !strings.Contains(body, "Test error message") {
		t.Errorf("Expected body to contain error message, got: %s", body)
	}
}

func TestRespondError_WithAPIPathPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()

	RespondError(w, req, "API error", http.StatusUnauthorized)

	// Verify status code
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// API paths should default to JSON
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain 'application/json' for API path, got '%s'", contentType)
	}

	// Verify JSON response
	body := w.Body.String()
	if !strings.Contains(body, "API error") {
		t.Errorf("Expected body to contain error message, got: %s", body)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("Expected body to contain 'error' field, got: %s", body)
	}
}
