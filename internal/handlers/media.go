package handlers

import (
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

// PlexClientInterface defines the methods needed from the Plex client
type PlexClientInterface interface {
	GetLibraries() ([]plex.Library, error)
	GetLibraryContents(libraryKey string) ([]plex.MediaItem, error)
	Search(query string) ([]plex.MediaItem, error)
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
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
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
				http.Error(w, "No Plex servers configured", http.StatusInternalServerError)
				return
			}
		}
	}

	// Try all URLs for this server until one works
	var libraries []plex.Library
	var lastErr error

	for _, url := range selectedServer.URLs {
		plexClient := h.newPlexClient(url, selectedServer.ID, user.PlexAuthToken, h.devMode)
		libraries, lastErr = plexClient.GetLibraries()
		if lastErr == nil {
			// Success! Use these results
			break
		}
		// Try next URL
	}

	if lastErr != nil {
		log.Printf("Failed to get libraries from server %s (tried %d URLs): %v", selectedServer.Name, len(selectedServer.URLs), lastErr)
		http.Error(w, "Failed to get libraries", http.StatusInternalServerError)
		return
	}

	// Render HTML response (will be replaced with templ later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Libraries - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; padding-top: 60px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .server-selector { margin-bottom: 20px; }
        .server-selector label { font-weight: bold; margin-right: 10px; }
        .server-selector select { padding: 8px 12px; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; background: white; cursor: pointer; }
        .search-form { margin-bottom: 30px; }
        .search-form input { padding: 10px; width: 300px; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; }
        .search-form button { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; }
        .search-form button:hover { background: #cc8f0a; }
        .library-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 20px; }
        .library-card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; text-decoration: none; color: inherit; display: block; }
        .library-card:hover { background: #f5f5f5; }
        .library-title { font-size: 20px; font-weight: bold; margin-bottom: 10px; }
        .library-type { color: #666; font-size: 14px; }
    </style>
</head>
<body>
%s
%s
    <div class="header">
        <h1>🎬 TapeDeck - Libraries</h1>
    </div>
`, NavigationHTML(), ConnectionBannerHTML())

	_, _ = fmt.Fprint(w, `    <div class="server-selector">
        <label for="server_select">Plex Server:</label>
        <select id="server_select" name="server_id" onchange="window.location.href='/libraries?server_id=' + this.value">
`)

	// Render server dropdown options
	for _, srv := range h.servers {
		selected := ""
		if srv.ID == selectedServer.ID {
			selected = " selected"
		}
		_, _ = fmt.Fprintf(w, `            <option value="%s"%s>%s</option>
`, html.EscapeString(srv.ID), selected, html.EscapeString(srv.Name))
	}

	_, _ = fmt.Fprint(w, `        </select>
    </div>
    <form class="search-form" method="get" action="/search">
        <input type="text" name="q" placeholder="Search all media..." required>
        <button type="submit">Search</button>
    </form>
    <div class="library-grid">
`)

	for _, lib := range libraries {
		_, _ = fmt.Fprintf(w, `
        <a href="/libraries/%s?server_id=%s" class="library-card">
            <div class="library-title">%s</div>
            <div class="library-type">%s</div>
        </a>
`, html.EscapeString(lib.Key), html.EscapeString(selectedServer.ID), html.EscapeString(lib.Title), html.EscapeString(lib.Type))
	}

	_, _ = fmt.Fprintf(w, `
    </div>
%s
</body>
</html>`, ConnectionBannerScript())
}

// LibraryContents handles GET /libraries/{id}
func (h *MediaHandler) LibraryContents(w http.ResponseWriter, r *http.Request, libraryKey string) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
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
				http.Error(w, "No Plex servers configured", http.StatusInternalServerError)
				return
			}
		}
	}

	// Try all URLs for this server until one works
	var items []plex.MediaItem
	var lastErr error

	for _, url := range selectedServer.URLs {
		plexClient := h.newPlexClient(url, selectedServer.ID, user.PlexAuthToken, h.devMode)
		items, lastErr = plexClient.GetLibraryContents(libraryKey)
		if lastErr == nil {
			// Success! Use these results
			break
		}
		// Try next URL
	}

	if lastErr != nil {
		log.Printf("Failed to get library contents from server %s (tried %d URLs): %v", selectedServer.Name, len(selectedServer.URLs), lastErr)
		http.Error(w, "Failed to get library contents", http.StatusInternalServerError)
		return
	}

	// Render HTML response (will be replaced with templ later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Library - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; padding-top: 60px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .media-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 20px; }
        .media-card { border: 1px solid #ddd; border-radius: 8px; padding: 15px; }
        .media-title { font-weight: bold; margin-bottom: 5px; }
        .media-year { color: #666; font-size: 14px; }
        .media-type { color: #999; font-size: 12px; text-transform: uppercase; }
    </style>
</head>
<body>
%s
%s
    <div class="header">
        <h1>🎬 Library Contents</h1>
    </div>
`, NavigationHTML(), ConnectionBannerHTML())

	_, _ = fmt.Fprint(w, `    <div class="media-grid">
`)

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("(%d)", item.Year)
		}
		_, _ = fmt.Fprintf(w, `
        <div class="media-card">
            <div class="media-title">%s</div>
            <div class="media-year">%s</div>
            <div class="media-type">%s</div>
        </div>
`, html.EscapeString(item.Title), html.EscapeString(yearStr), html.EscapeString(item.Type))
	}

	_, _ = fmt.Fprintf(w, `
    </div>
%s
</body>
</html>`, ConnectionBannerScript())
}

// Search handles GET /search
func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get search query from URL
	query := r.URL.Query().Get("q")

	// If no query, show empty search page
	if query == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Search - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; padding-top: 60px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .search-form { margin: 50px auto; max-width: 500px; text-align: center; }
        .search-form input { padding: 15px; width: 100%%; font-size: 18px; border: 2px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .search-form button { padding: 15px 30px; font-size: 18px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; margin-top: 20px; }
        .search-form button:hover { background: #cc8f0a; }
    </style>
</head>
<body>
%s
%s
    <div class="header">
        <h1>🎬 Search Media</h1>
    </div>
`, NavigationHTML(), ConnectionBannerHTML())

	_, _ = fmt.Fprint(w, `    <form class="search-form" method="get" action="/search">
        <input type="text" name="q" placeholder="Search for movies, shows, music..." required autofocus>
        <button type="submit">Search</button>
    </form>
`)

	_, _ = fmt.Fprintf(w, `
%s
</body>
</html>`, ConnectionBannerScript())
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
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
				items, lastErr = plexClient.Search(query)
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
			log.Printf("Failed to search server %s: %v", result.serverName, result.err)
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
		log.Printf("All servers failed to search")
		http.Error(w, "Failed to search all servers", http.StatusInternalServerError)
		return
	}

	// Render HTML response (will be replaced with templ later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Search Results - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; padding-top: 60px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .search-form { margin-bottom: 30px; }
        .search-form input { padding: 10px; width: 300px; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; }
        .search-form button { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; }
        .search-form button:hover { background: #cc8f0a; }
        .results-info { color: #666; margin-bottom: 20px; }
        .media-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 20px; }
        .media-card { border: 1px solid #ddd; border-radius: 8px; padding: 15px; }
        .media-title { font-weight: bold; margin-bottom: 5px; }
        .media-year { color: #666; font-size: 14px; }
        .media-type { color: #999; font-size: 12px; text-transform: uppercase; }
        .media-server { color: #1976d2; font-size: 12px; margin-top: 5px; font-weight: 500; }
        .no-results { text-align: center; color: #666; margin-top: 50px; font-size: 18px; }
    </style>
</head>
<body>
%s
%s
    <div class="header">
        <h1>🎬 Search Results</h1>
    </div>
`, NavigationHTML(), ConnectionBannerHTML())

	_, _ = fmt.Fprintf(w, `    <form class="search-form" method="get" action="/search">
        <input type="text" name="q" value="%s" placeholder="Search media..." required>
        <button type="submit">Search</button>
    </form>
    <div class="results-info">Found %d result(s) for "%s"</div>
`, html.EscapeString(query), len(items), html.EscapeString(query))

	if len(items) == 0 {
		_, _ = fmt.Fprint(w, `    <div class="no-results">No results found. Try a different search term.</div>`)
	} else {
		_, _ = fmt.Fprint(w, `    <div class="media-grid">`)
		for _, item := range items {
			yearStr := ""
			if item.Year > 0 {
				yearStr = fmt.Sprintf("(%d)", item.Year)
			}
			_, _ = fmt.Fprintf(w, `
        <div class="media-card">
            <div class="media-title">%s</div>
            <div class="media-year">%s</div>
            <div class="media-type">%s</div>
            <div class="media-server">📡 %s</div>
        </div>
`, html.EscapeString(item.Title), html.EscapeString(yearStr), html.EscapeString(item.Type), html.EscapeString(item.ServerName))
		}
		_, _ = fmt.Fprint(w, `    </div>`)
	}

	_, _ = fmt.Fprintf(w, `
%s
</body>
</html>`, ConnectionBannerScript())
}
