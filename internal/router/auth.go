// Package router provides HTTP route configuration for the TapeDeck application.
package router

import (
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
)

// authRouter sets up routes for authentication
func authRouter(h *handlers.AuthHandler) chi.Router {
	r := chi.NewRouter()

	r.Get("/login", h.Login)
	r.Get("/poll-status", h.PollStatus)
	r.Post("/logout", h.Logout)
	r.Get("/logout", h.Logout) // Support GET for convenience

	return r
}
