package handlers

import (
	"context"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

// PlexClientInterface defines the methods needed from the Plex client
type PlexClientInterface interface {
	GetLibraries(ctx context.Context) ([]plex.Library, error)
	GetLibraryContents(ctx context.Context, libraryKey string) ([]plex.MediaItem, error)
	Search(ctx context.Context, query string) ([]plex.MediaItem, error)
}

// PlexClientFactory creates a new Plex client
type PlexClientFactory func(serverURL, serverID, authToken string, devMode bool) PlexClientInterface

// ServerInfo holds information about a Plex server for the media handler
type ServerInfo struct {
	ID   string
	Name string
	URLs []string // All connection URLs, ordered by priority
}

// MediaHandler handles media browsing requests
type MediaHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	servers       []ServerInfo
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewMediaHandler creates a new media handler
func NewMediaHandler(store *sessions.CookieStore, database *db.DB, servers []ServerInfo, devMode bool) *MediaHandler {
	return &MediaHandler{
		sessionStore: store,
		db:           database,
		servers:      servers,
		devMode:      devMode,
		newPlexClient: func(serverURL, serverID, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, serverID, authToken, devMode)
		},
	}
}

// Libraries handles GET /libraries
func (h *MediaHandler) Libraries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("Failed to get user", "error", err)
		RespondError(w, r, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Get selected server from query param, default to first server
	selectedServerID := r.URL.Query().Get("server_id")
	var selectedServer ServerInfo
	if selectedServerID == "" && len(h.servers) > 0 {
		selectedServer = h.servers[0]
	} else {
		// Find the requested server
		found := false
		for _, srv := range h.servers {
			if srv.ID == selectedServerID {
				selectedServer = srv
				found = true
				break
			}
		}
		if !found {
			if len(h.servers) > 0 {
				selectedServer = h.servers[0]
			} else {
				RespondError(w, r, "No Plex servers configured", http.StatusInternalServerError)
				return
			}
		}
	}

	// Try all URLs for this server until one works
	var libraries []plex.Library
	var lastErr error

	for _, url := range selectedServer.URLs {
		plexClient := h.newPlexClient(url, selectedServer.ID, user.PlexAuthToken, h.devMode)
		libraries, lastErr = plexClient.GetLibraries(ctx)
		if lastErr == nil {
			// Success! Use these results
			break
		}
		// Try next URL
	}

	if lastErr != nil {
		log.Error("Failed to get libraries from server", "server", selectedServer.Name, "urls_tried", len(selectedServer.URLs), "error", lastErr)
		RespondError(w, r, "Failed to get libraries", http.StatusInternalServerError)
		return
	}

	// Convert ServerInfo to pages.ServerInfo
	var pageServers []pages.ServerInfo
	for _, s := range h.servers {
		pageServers = append(pageServers, pages.ServerInfo{ID: s.ID, Name: s.Name})
	}
	pageSelectedServer := pages.ServerInfo{ID: selectedServer.ID, Name: selectedServer.Name}

	// Render using templ template
	if err := pages.MediaLibraries(pageServers, pageSelectedServer, libraries, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// LibraryContents handles GET /libraries/{libraryKey}
func (h *MediaHandler) LibraryContents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	libraryKey := chi.URLParam(r, "libraryKey")
	if libraryKey == "" {
		RespondError(w, r, "Library key required", http.StatusBadRequest)
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("Failed to get user", "error", err)
		RespondError(w, r, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Get selected server from query param, default to first server
	selectedServerID := r.URL.Query().Get("server_id")
	var selectedServer ServerInfo
	if selectedServerID == "" && len(h.servers) > 0 {
		selectedServer = h.servers[0]
	} else {
		// Find the requested server
		found := false
		for _, srv := range h.servers {
			if srv.ID == selectedServerID {
				selectedServer = srv
				found = true
				break
			}
		}
		if !found {
			if len(h.servers) > 0 {
				selectedServer = h.servers[0]
			} else {
				RespondError(w, r, "No Plex servers configured", http.StatusInternalServerError)
				return
			}
		}
	}

	// Try all URLs for this server until one works
	var items []plex.MediaItem
	var lastErr error

	for _, url := range selectedServer.URLs {
		plexClient := h.newPlexClient(url, selectedServer.ID, user.PlexAuthToken, h.devMode)
		items, lastErr = plexClient.GetLibraryContents(ctx, libraryKey)
		if lastErr == nil {
			// Success! Use these results
			break
		}
		// Try next URL
	}

	if lastErr != nil {
		log.Error("Failed to get library contents from server", "server", selectedServer.Name, "urls_tried", len(selectedServer.URLs), "error", lastErr)
		RespondError(w, r, "Failed to get library contents", http.StatusInternalServerError)
		return
	}

	// Render using templ template
	if err := pages.MediaLibraryContents(items, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// Search handles GET /search
func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get search query from URL
	query := r.URL.Query().Get("q")

	// If no query, show empty search page
	if query == "" {
		if err := pages.MediaSearchEmpty(NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
			log.Error("Failed to render template", "error", err)
			RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil {
		log.Error("Failed to get user", "error", err)
		RespondError(w, r, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Search across all servers in parallel
	type serverResult struct {
		serverName string
		serverID   string
		items      []plex.MediaItem
		err        error
	}

	resultChan := make(chan serverResult, len(h.servers))

	// Launch goroutine for each server
	for _, server := range h.servers {
		go func(srv ServerInfo) {
			// Try all URLs for this server until one works
			var items []plex.MediaItem
			var lastErr error

			for _, url := range srv.URLs {
				plexClient := h.newPlexClient(url, srv.ID, user.PlexAuthToken, h.devMode)
				items, lastErr = plexClient.Search(ctx, query)
				if lastErr == nil {
					// Success! Use these results
					break
				}
				// Try next URL
			}

			resultChan <- serverResult{
				serverName: srv.Name,
				serverID:   srv.ID,
				items:      items,
				err:        lastErr,
			}
		}(server)
	}

	// Collect results from all servers
	var items []plex.MediaItem
	var searchErrors []error

	for i := 0; i < len(h.servers); i++ {
		result := <-resultChan
		if result.err != nil {
			log.Warn("Failed to search server", "server", result.serverName, "error", result.err)
			searchErrors = append(searchErrors, result.err)
			continue
		}

		// Add server name and ID to each item
		for j := range result.items {
			result.items[j].ServerName = result.serverName
			result.items[j].ServerID = result.serverID
		}

		items = append(items, result.items...)
	}

	// If all servers failed, return error
	if len(searchErrors) == len(h.servers) {
		log.Error("All servers failed to search")
		RespondError(w, r, "Failed to search all servers", http.StatusInternalServerError)
		return
	}

	// Render using templ template
	if err := pages.MediaSearchResults(query, items, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}
