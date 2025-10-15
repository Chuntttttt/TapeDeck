package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/Chuntttttt/tapedeck/templates/pages"
)

// RespondError writes an error response, automatically detecting JSON vs HTML based on the request
func RespondError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	if wantsJSON(r) {
		respondJSON(w, message, statusCode)
	} else {
		respondHTML(w, r, message, statusCode)
	}
}

// wantsJSON determines if the client wants a JSON response
func wantsJSON(r *http.Request) bool {
	// Check if path starts with /api
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}

	// Check Accept header
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// respondJSON writes a JSON error response
func respondJSON(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"error":  message,
		"status": statusCode,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON error response: %v", err)
	}
}

// respondHTML writes an HTML error response using the error template
func respondHTML(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	if err := pages.Error(statusCode, message).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render error template: %v", err)
		// Fallback to plain text if template fails
		http.Error(w, message, statusCode)
	}
}
