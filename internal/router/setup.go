package router

import (
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/go-chi/chi/v5"
)

// setupRouter sets up routes for the setup wizard
func setupRouter(h *handlers.SetupHandler) chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.Step1Welcome)
	r.Get("/plex", h.Step2Plex)
	r.Post("/plex/save", h.SavePlexServers)
	r.Get("/ha", h.Step3HomeAssistant)
	r.Post("/ha/test", h.TestHomeAssistant)
	r.Post("/ha/save", h.SaveHomeAssistant)
	r.Get("/appletv", h.Step4AppleTVs)
	r.Post("/appletv/save", h.SaveAppleTVs)
	r.Get("/complete", h.Step5Complete)
	r.Post("/finish", h.CompleteSetup)

	return r
}
