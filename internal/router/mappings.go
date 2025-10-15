package router

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
)

// mappingsRouter sets up routes for card mappings
func mappingsRouter(h *handlers.MappingsHandler, pairingH *handlers.PairingHandler, auth func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(auth) // Require auth for all mappings routes

	r.Get("/", h.Dashboard)
	r.Post("/", h.CreateMapping)
	r.Get("/new", h.NewMappingForm)
	r.Get("/pair", pairingH.PairForm)

	// Routes with ID parameter
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/edit", h.EditMappingForm)
		r.Post("/", h.UpdateMapping)
		r.Post("/delete", h.DeleteMapping)
	})

	return r
}
