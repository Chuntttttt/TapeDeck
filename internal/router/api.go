// Package router provides HTTP routing configuration for TapeDeck.
package router

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
)

// apiRouter sets up API and WebSocket routes
func apiRouter(
	mappingsH *handlers.MappingsHandler,
	playbackH *handlers.PlaybackHandler,
	statusH *handlers.StatusHandler,
	auth func(http.Handler) http.Handler,
) chi.Router {
	r := chi.NewRouter()

	// API routes
	r.Group(func(r chi.Router) {
		r.Use(auth)
		r.Get("/search", mappingsH.SearchJSON)
		r.Post("/play", playbackH.Play)
	})

	// Status API (some endpoints don't need auth)
	r.Get("/status/ha", statusH.HAStatus)
	r.Post("/status/ha/reconnect", statusH.HAReconnect)

	return r
}

// wsRouter sets up WebSocket routes
func wsRouter(pairingH *handlers.PairingHandler, auth func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(auth)

	r.Get("/pairing", pairingH.WebSocketPairing)

	return r
}
