package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
)

// MappingsHandler handles card mapping requests
type MappingsHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	servers       []ServerInfo
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewMappingsHandler creates a new mappings handler
func NewMappingsHandler(store *sessions.CookieStore, database *db.DB, servers []ServerInfo, devMode bool) *MappingsHandler {
	return &MappingsHandler{
		sessionStore: store,
		db:           database,
		servers:      servers,
		devMode:      devMode,
		newPlexClient: func(serverURL, serverID, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, serverID, authToken, devMode)
		},
	}
}

// Dashboard handles GET /mappings
func (h *MappingsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get all mappings for the user
	mappings, err := h.db.GetCardMappingsByUserID(ctx, userID)
	if err != nil {
		log.Error("Failed to get card mappings", "error", err)
		RespondError(w, r, "Failed to get card mappings", http.StatusInternalServerError)
		return
	}

	// Render using templ template
	if err := pages.MappingsDashboard(mappings, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// NewMappingForm handles GET /mappings/new
func (h *MappingsHandler) NewMappingForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	_, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Render using templ template
	if err := pages.MappingsNewForm(NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// CreateMapping handles POST /mappings
func (h *MappingsHandler) CreateMapping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		log.Warn("User not authenticated")
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		log.Error("Failed to parse form", "error", err)
		RespondError(w, r, "Failed to parse form", http.StatusBadRequest)
		return
	}

	tagID := r.FormValue("tag_id")
	mediaType := r.FormValue("media_type")
	mediaID := r.FormValue("media_id")
	mediaTitle := r.FormValue("media_title")
	plexServerID := r.FormValue("plex_server_id")
	appleTVEntity := r.FormValue("apple_tv_entity")

	log.Info("Received form data", "tag_id", tagID, "media_type", mediaType, "media_id", mediaID, "media_title", mediaTitle, "plex_server_id", plexServerID, "apple_tv_entity", appleTVEntity)

	// Validate required fields
	if tagID == "" || mediaType == "" || mediaID == "" || mediaTitle == "" {
		log.Warn("Missing required fields", "tag_id", tagID, "media_type", mediaType, "media_id", mediaID, "media_title", mediaTitle)
		RespondError(w, r, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Create mapping
	mapping := models.NewCardMapping(userID, tagID, mediaType, mediaID, mediaTitle, plexServerID, appleTVEntity)
	mappingID, err := h.db.CreateCardMapping(ctx, mapping)
	if err != nil {
		log.Error("Failed to create card mapping", "error", err)
		RespondError(w, r, "Failed to create card mapping: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("Successfully created mapping", "mapping_id", mappingID)

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// EditMappingForm handles GET /mappings/{id}/edit
func (h *MappingsHandler) EditMappingForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	idStr := chi.URLParam(r, "id")
	mappingID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RespondError(w, r, "Invalid mapping ID", http.StatusBadRequest)
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping
	mapping, err := h.db.GetCardMappingByID(ctx, mappingID)
	if err != nil {
		log.Error("Failed to get card mapping", "error", err, "mapping_id", mappingID)
		RespondError(w, r, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		RespondError(w, r, "Forbidden", http.StatusForbidden)
		return
	}

	// Render using templ template
	if err := pages.MappingsEditForm(mapping, NavigationHTML(), ConnectionBannerHTML(), ConnectionBannerScript()).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
	}
}

// UpdateMapping handles POST /mappings/{id}
func (h *MappingsHandler) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	idStr := chi.URLParam(r, "id")
	mappingID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RespondError(w, r, "Invalid mapping ID", http.StatusBadRequest)
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping
	mapping, err := h.db.GetCardMappingByID(ctx, mappingID)
	if err != nil {
		log.Error("Failed to get card mapping", "error", err, "mapping_id", mappingID)
		RespondError(w, r, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		RespondError(w, r, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		RespondError(w, r, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Update fields
	mapping.TagID = r.FormValue("tag_id")
	mapping.MediaType = r.FormValue("media_type")
	mapping.MediaID = r.FormValue("media_id")
	mapping.MediaTitle = r.FormValue("media_title")
	mapping.UpdatedAt = time.Now()

	// Update in database
	if err := h.db.UpdateCardMapping(ctx, mapping); err != nil {
		log.Error("Failed to update card mapping", "error", err)
		RespondError(w, r, "Failed to update card mapping", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// DeleteMapping handles POST /mappings/{id}/delete
func (h *MappingsHandler) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	idStr := chi.URLParam(r, "id")
	mappingID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		RespondError(w, r, "Invalid mapping ID", http.StatusBadRequest)
		return
	}

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping to verify ownership
	mapping, err := h.db.GetCardMappingByID(ctx, mappingID)
	if err != nil {
		log.Error("Failed to get card mapping", "error", err, "mapping_id", mappingID)
		RespondError(w, r, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		RespondError(w, r, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete mapping
	if err := h.db.DeleteCardMapping(ctx, mappingID); err != nil {
		log.Error("Failed to delete card mapping", "error", err)
		RespondError(w, r, "Failed to delete card mapping", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// SearchJSON handles GET /api/search?q=query and returns JSON for autocomplete
func (h *MappingsHandler) SearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Get user from context
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		RespondError(w, r, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get search query
	query := r.URL.Query().Get("q")
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"results":[]}`)
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
	var allItems []plex.MediaItem
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

		allItems = append(allItems, result.items...)
	}

	// If all servers failed, return error
	if len(searchErrors) == len(h.servers) && len(h.servers) > 0 {
		log.Error("All servers failed to search")
		RespondError(w, r, "Failed to search all servers", http.StatusInternalServerError)
		return
	}

	// Convert to JSON response
	type SearchResult struct {
		RatingKey  string `json:"ratingKey"`
		Title      string `json:"title"`
		Type       string `json:"type"`
		Year       int    `json:"year,omitempty"`
		ServerID   string `json:"serverID"`
		ServerName string `json:"serverName"`
	}

	type SearchResponse struct {
		Results []SearchResult `json:"results"`
	}

	results := make([]SearchResult, len(allItems))
	for i, item := range allItems {
		results[i] = SearchResult{
			RatingKey:  item.RatingKey,
			Title:      item.Title,
			Type:       item.Type,
			Year:       item.Year,
			ServerID:   item.ServerID,
			ServerName: item.ServerName,
		}
	}

	response := SearchResponse{Results: results}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
