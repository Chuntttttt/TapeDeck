package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

// MappingsHandler handles card mapping requests
type MappingsHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	plexURL       string
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewMappingsHandler creates a new mappings handler
func NewMappingsHandler(store *sessions.CookieStore, database *db.DB, plexURL string, devMode bool) *MappingsHandler {
	return &MappingsHandler{
		sessionStore: store,
		db:           database,
		plexURL:      plexURL,
		devMode:      devMode,
		newPlexClient: func(serverURL, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, authToken, devMode)
		},
	}
}

// Dashboard handles GET /mappings
func (h *MappingsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get all mappings for the user
	mappings, err := h.db.GetCardMappingsByUserID(userID)
	if err != nil {
		log.Printf("Failed to get card mappings: %v", err)
		http.Error(w, "Failed to get card mappings", http.StatusInternalServerError)
		return
	}

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Card Mappings - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
        .header-actions { display: flex; gap: 10px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .btn { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn:hover { background: #cc8f0a; }
        .btn-danger { background: #d32f2f; }
        .btn-danger:hover { background: #b71c1c; }
        .btn-small { padding: 5px 10px; font-size: 14px; }
        table { width: 100%%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f5f5f5; font-weight: bold; }
        .actions { display: flex; gap: 10px; }
        .empty-state { text-align: center; padding: 50px 20px; color: #666; }
        .empty-state h2 { margin-bottom: 10px; }
    </style>
</head>
<body>
    <a href="/libraries" class="back-link">← Back to Libraries</a>
    <div class="header">
        <h1>Card Mappings</h1>
        <div class="header-actions">
            <a href="/mappings/new" class="btn">+ New Mapping</a>
            <form method="post" action="/auth/logout" style="margin: 0;">
                <button type="submit" class="btn">Logout</button>
            </form>
        </div>
    </div>
`)

	if len(mappings) == 0 {
		_, _ = fmt.Fprint(w, `
    <div class="empty-state">
        <h2>No card mappings yet</h2>
        <p>Create your first mapping to link an NFC card with media.</p>
        <a href="/mappings/new" class="btn">Create Mapping</a>
    </div>
`)
	} else {
		_, _ = fmt.Fprint(w, `
    <table>
        <thead>
            <tr>
                <th>Tag ID</th>
                <th>Media Title</th>
                <th>Type</th>
                <th>Media ID</th>
                <th>Created</th>
                <th>Actions</th>
            </tr>
        </thead>
        <tbody>
`)

		for _, mapping := range mappings {
			createdAt := mapping.CreatedAt.Format("2006-01-02 15:04")
			_, _ = fmt.Fprintf(w, `
            <tr>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
                <td class="actions">
                    <a href="/mappings/%d/edit" class="btn btn-small">Edit</a>
                    <form method="post" action="/mappings/%d/delete" style="margin: 0;">
                        <button type="submit" class="btn btn-small btn-danger" onclick="return confirm('Delete this mapping?')">Delete</button>
                    </form>
                </td>
            </tr>
`, html.EscapeString(mapping.TagID), html.EscapeString(mapping.MediaTitle), html.EscapeString(mapping.MediaType), html.EscapeString(mapping.MediaID), html.EscapeString(createdAt), mapping.ID, mapping.ID)
		}

		_, _ = fmt.Fprint(w, `
        </tbody>
    </table>
`)
	}

	_, _ = fmt.Fprint(w, `
</body>
</html>`)
}

// NewMappingForm handles GET /mappings/new
func (h *MappingsHandler) NewMappingForm(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>New Card Mapping - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; font-weight: bold; }
        input[type="text"] { padding: 10px; width: 100%; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .btn { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; }
        .btn:hover { background: #cc8f0a; }
        .btn:disabled { background: #ccc; cursor: not-allowed; }
        .search-container { position: relative; }
        .search-results { position: absolute; top: 100%; left: 0; right: 0; background: white; border: 1px solid #ddd; border-top: none; max-height: 300px; overflow-y: auto; display: none; z-index: 10; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .search-result-item { padding: 12px; border-bottom: 1px solid #f0f0f0; cursor: pointer; }
        .search-result-item:hover { background: #f5f5f5; }
        .result-title { font-weight: bold; margin-bottom: 3px; }
        .result-meta { font-size: 14px; color: #666; }
        .selected-media { padding: 12px; background: #f5f5f5; border: 1px solid #ddd; border-radius: 4px; margin-top: 10px; display: none; }
        .selected-media-title { font-weight: bold; margin-bottom: 5px; }
        .selected-media-meta { font-size: 14px; color: #666; }
        .error { color: #d32f2f; margin-top: 10px; display: none; }
        .help-text { font-size: 14px; color: #666; margin-top: 5px; }
    </style>
</head>
<body>
    <a href="/mappings" class="back-link">← Back to Mappings</a>
    <h1>Create New Card Mapping</h1>

    <form method="post" action="/mappings" id="mappingForm">
        <div class="form-group">
            <label for="tag_id">NFC Tag ID *</label>
            <input type="text" id="tag_id" name="tag_id" required placeholder="e.g., nfc-12345">
            <div class="help-text">Enter the unique ID from your NFC card</div>
        </div>

        <div class="form-group">
            <label for="media_search">Search for Media *</label>
            <div class="search-container">
                <input type="text" id="media_search" placeholder="Start typing to search..." autocomplete="off">
                <div class="search-results" id="searchResults"></div>
            </div>
            <div class="selected-media" id="selectedMedia">
                <div class="selected-media-title" id="selectedTitle"></div>
                <div class="selected-media-meta" id="selectedMeta"></div>
            </div>
            <div class="error" id="searchError">Failed to search. Please try again.</div>
        </div>

        <input type="hidden" id="media_type" name="media_type" required>
        <input type="hidden" id="media_id" name="media_id" required>
        <input type="hidden" id="media_title" name="media_title" required>

        <button type="submit" class="btn" id="submitBtn" disabled>Create Mapping</button>
    </form>

    <script>
        const searchInput = document.getElementById('media_search');
        const searchResults = document.getElementById('searchResults');
        const selectedMedia = document.getElementById('selectedMedia');
        const selectedTitle = document.getElementById('selectedTitle');
        const selectedMeta = document.getElementById('selectedMeta');
        const searchError = document.getElementById('searchError');
        const submitBtn = document.getElementById('submitBtn');
        const mediaTypeInput = document.getElementById('media_type');
        const mediaIdInput = document.getElementById('media_id');
        const mediaTitleInput = document.getElementById('media_title');

        let searchTimeout;
        let selectedItem = null;

        searchInput.addEventListener('input', function() {
            const query = this.value.trim();

            clearTimeout(searchTimeout);

            if (query.length < 2) {
                searchResults.style.display = 'none';
                searchResults.innerHTML = '';
                return;
            }

            searchTimeout = setTimeout(() => {
                performSearch(query);
            }, 300);
        });

        async function performSearch(query) {
            try {
                searchError.style.display = 'none';
                const response = await fetch('/api/search?q=' + encodeURIComponent(query));

                if (!response.ok) {
                    throw new Error('Search failed');
                }

                const data = await response.json();

                if (data.results.length === 0) {
                    searchResults.innerHTML = '<div class="search-result-item">No results found</div>';
                    searchResults.style.display = 'block';
                    return;
                }

                searchResults.innerHTML = '';

                data.results.forEach(result => {
                    const item = document.createElement('div');
                    item.className = 'search-result-item';
                    const yearStr = result.year ? ' (' + result.year + ')' : '';
                    item.innerHTML = '<div class="result-title">' + result.title + '</div><div class="result-meta">' + result.type + yearStr + '</div>';
                    item.dataset.title = result.title;
                    item.dataset.year = result.year || '';
                    item.dataset.type = result.type;
                    item.dataset.ratingKey = result.ratingKey;

                    item.addEventListener('click', function() {
                        selectMedia(this.dataset.title, this.dataset.type, this.dataset.ratingKey, this.dataset.year);
                    });

                    searchResults.appendChild(item);
                });

                searchResults.style.display = 'block';
            } catch (error) {
                console.error('Search error:', error);
                searchError.style.display = 'block';
            }
        }

        function selectMedia(title, type, ratingKey, year) {
            selectedItem = { title, type, ratingKey, year };

            selectedTitle.textContent = title;
            selectedMeta.textContent = type + (year ? ' (' + year + ')' : '');
            selectedMedia.style.display = 'block';

            mediaTypeInput.value = type;
            mediaIdInput.value = ratingKey;
            mediaTitleInput.value = title;

            searchResults.style.display = 'none';
            searchInput.value = title;

            submitBtn.disabled = false;
        }

        // Hide search results when clicking outside
        document.addEventListener('click', function(e) {
            if (!searchInput.contains(e.target) && !searchResults.contains(e.target)) {
                searchResults.style.display = 'none';
            }
        });

        // Validate form before submit
        document.getElementById('mappingForm').addEventListener('submit', function(e) {
            if (!mediaIdInput.value || !mediaTypeInput.value || !mediaTitleInput.value) {
                e.preventDefault();
                alert('Please select media from search results');
            }
        });
    </script>
</body>
</html>`)
}

// CreateMapping handles POST /mappings
func (h *MappingsHandler) CreateMapping(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	tagID := r.FormValue("tag_id")
	mediaType := r.FormValue("media_type")
	mediaID := r.FormValue("media_id")
	mediaTitle := r.FormValue("media_title")

	// Create mapping
	mapping := models.NewCardMapping(userID, tagID, mediaType, mediaID, mediaTitle)
	_, err := h.db.CreateCardMapping(mapping)
	if err != nil {
		log.Printf("Failed to create card mapping: %v", err)
		http.Error(w, "Failed to create card mapping", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// EditMappingForm handles GET /mappings/{id}/edit
func (h *MappingsHandler) EditMappingForm(w http.ResponseWriter, r *http.Request, mappingID int64) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping
	mapping, err := h.db.GetCardMappingByID(mappingID)
	if err != nil {
		log.Printf("Failed to get card mapping: %v", err)
		http.Error(w, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Edit Card Mapping - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; font-weight: bold; }
        input[type="text"] { padding: 10px; width: 100%%; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .btn { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; }
        .btn:hover { background: #cc8f0a; }
        .help-text { font-size: 14px; color: #666; margin-top: 5px; }
        .current-media { padding: 12px; background: #f5f5f5; border: 1px solid #ddd; border-radius: 4px; margin-top: 10px; }
        .current-media-label { font-size: 14px; color: #666; margin-bottom: 5px; }
        .current-media-title { font-weight: bold; margin-bottom: 5px; }
        .current-media-meta { font-size: 14px; color: #666; }
    </style>
</head>
<body>
    <a href="/mappings" class="back-link">← Back to Mappings</a>
    <h1>Edit Card Mapping</h1>

    <form method="post" action="/mappings/%d">
        <div class="form-group">
            <label for="tag_id">NFC Tag ID *</label>
            <input type="text" id="tag_id" name="tag_id" value="%s" required placeholder="e.g., nfc-12345">
            <div class="help-text">Enter the unique ID from your NFC card</div>
        </div>

        <div class="form-group">
            <label>Current Media</label>
            <div class="current-media">
                <div class="current-media-title">%s</div>
                <div class="current-media-meta">%s - %s</div>
            </div>
            <div class="help-text">To change media, please delete this mapping and create a new one</div>
        </div>

        <input type="hidden" name="media_type" value="%s">
        <input type="hidden" name="media_id" value="%s">
        <input type="hidden" name="media_title" value="%s">

        <button type="submit" class="btn">Update Mapping</button>
    </form>
</body>
</html>`, mapping.ID, html.EscapeString(mapping.TagID), html.EscapeString(mapping.MediaTitle), html.EscapeString(mapping.MediaType), html.EscapeString(mapping.MediaID), html.EscapeString(mapping.MediaType), html.EscapeString(mapping.MediaID), html.EscapeString(mapping.MediaTitle))
}

// UpdateMapping handles POST /mappings/{id}
func (h *MappingsHandler) UpdateMapping(w http.ResponseWriter, r *http.Request, mappingID int64) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping
	mapping, err := h.db.GetCardMappingByID(mappingID)
	if err != nil {
		log.Printf("Failed to get card mapping: %v", err)
		http.Error(w, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Update fields
	mapping.TagID = r.FormValue("tag_id")
	mapping.MediaType = r.FormValue("media_type")
	mapping.MediaID = r.FormValue("media_id")
	mapping.MediaTitle = r.FormValue("media_title")
	mapping.UpdatedAt = time.Now()

	// Update in database
	if err := h.db.UpdateCardMapping(mapping); err != nil {
		log.Printf("Failed to update card mapping: %v", err)
		http.Error(w, "Failed to update card mapping", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// DeleteMapping handles POST /mappings/{id}/delete
func (h *MappingsHandler) DeleteMapping(w http.ResponseWriter, r *http.Request, mappingID int64) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get mapping to verify ownership
	mapping, err := h.db.GetCardMappingByID(mappingID)
	if err != nil {
		log.Printf("Failed to get card mapping: %v", err)
		http.Error(w, "Card mapping not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if mapping.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete mapping
	if err := h.db.DeleteCardMapping(mappingID); err != nil {
		log.Printf("Failed to delete card mapping: %v", err)
		http.Error(w, "Failed to delete card mapping", http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/mappings", http.StatusFound)
}

// SearchJSON handles GET /api/search?q=query and returns JSON for autocomplete
func (h *MappingsHandler) SearchJSON(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
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
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Create Plex client
	plexClient := h.newPlexClient(h.plexURL, user.PlexAuthToken, h.devMode)

	// Search
	items, err := plexClient.Search(query)
	if err != nil {
		log.Printf("Failed to search: %v", err)
		http.Error(w, "Failed to search", http.StatusInternalServerError)
		return
	}

	// Convert to JSON response
	type SearchResult struct {
		RatingKey string `json:"ratingKey"`
		Title     string `json:"title"`
		Type      string `json:"type"`
		Year      int    `json:"year,omitempty"`
	}

	type SearchResponse struct {
		Results []SearchResult `json:"results"`
	}

	results := make([]SearchResult, len(items))
	for i, item := range items {
		results[i] = SearchResult{
			RatingKey: item.RatingKey,
			Title:     item.Title,
			Type:      item.Type,
			Year:      item.Year,
		}
	}

	response := SearchResponse{Results: results}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
