package router

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
)

// mediaRouter sets up routes for media browsing
func mediaRouter(h *handlers.MediaHandler, auth func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(auth) // Require auth for all media routes

	r.Get("/", h.Libraries)
	r.Get("/search", h.Search)
	r.Get("/{libraryKey}", h.LibraryContents)

	return r
}
