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
type PlexClientFactory func(serverURL, authToken string, devMode bool) PlexClientInterface

// MediaHandler handles media browsing requests
type MediaHandler struct {
	sessionStore  *sessions.CookieStore
	db            *db.DB
	plexURL       string
	devMode       bool
	newPlexClient PlexClientFactory
}

// NewMediaHandler creates a new media handler
func NewMediaHandler(store *sessions.CookieStore, database *db.DB, plexURL string, devMode bool) *MediaHandler {
	return &MediaHandler{
		sessionStore: store,
		db:           database,
		plexURL:      plexURL,
		devMode:      devMode,
		newPlexClient: func(serverURL, authToken string, devMode bool) PlexClientInterface {
			return plex.NewClient(serverURL, authToken, devMode)
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

	// Create Plex client
	plexClient := h.newPlexClient(h.plexURL, user.PlexAuthToken, h.devMode)

	// Get libraries
	libraries, err := plexClient.GetLibraries()
	if err != nil {
		log.Printf("Failed to get libraries: %v", err)
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
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
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
    <div class="header">
        <h1>🎬 TapeDeck - Libraries</h1>
        <form method="post" action="/auth/logout">
            <button type="submit">Logout</button>
        </form>
    </div>
    <form class="search-form" method="get" action="/search">
        <input type="text" name="q" placeholder="Search all media..." required>
        <button type="submit">Search</button>
    </form>
    <div class="library-grid">
`)

	for _, lib := range libraries {
		_, _ = fmt.Fprintf(w, `
        <a href="/libraries/%s" class="library-card">
            <div class="library-title">%s</div>
            <div class="library-type">%s</div>
        </a>
`, html.EscapeString(lib.Key), html.EscapeString(lib.Title), html.EscapeString(lib.Type))
	}

	_, _ = fmt.Fprint(w, `
    </div>
</body>
</html>`)
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

	// Create Plex client
	plexClient := h.newPlexClient(h.plexURL, user.PlexAuthToken, h.devMode)

	// Get library contents
	items, err := plexClient.GetLibraryContents(libraryKey)
	if err != nil {
		log.Printf("Failed to get library contents: %v", err)
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
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
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
    <a href="/libraries" class="back-link">← Back to Libraries</a>
    <div class="header">
        <h1>🎬 Library Contents</h1>
        <form method="post" action="/auth/logout">
            <button type="submit">Logout</button>
        </form>
    </div>
    <div class="media-grid">
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

	_, _ = fmt.Fprint(w, `
    </div>
</body>
</html>`)
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
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Search - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .search-form { margin: 50px auto; max-width: 500px; text-align: center; }
        .search-form input { padding: 15px; width: 100%; font-size: 18px; border: 2px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .search-form button { padding: 15px 30px; font-size: 18px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; margin-top: 20px; }
        .search-form button:hover { background: #cc8f0a; }
    </style>
</head>
<body>
    <a href="/libraries" class="back-link">← Back to Libraries</a>
    <div class="header">
        <h1>🎬 Search Media</h1>
        <form method="post" action="/auth/logout">
            <button type="submit">Logout</button>
        </form>
    </div>
    <form class="search-form" method="get" action="/search">
        <input type="text" name="q" placeholder="Search for movies, shows, music..." required autofocus>
        <button type="submit">Search</button>
    </form>
</body>
</html>`)
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

	// Render HTML response (will be replaced with templ later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Search Results - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; }
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
        .no-results { text-align: center; color: #666; margin-top: 50px; font-size: 18px; }
    </style>
</head>
<body>
    <a href="/libraries" class="back-link">← Back to Libraries</a>
    <div class="header">
        <h1>🎬 Search Results</h1>
        <form method="post" action="/auth/logout">
            <button type="submit">Logout</button>
        </form>
    </div>
    <form class="search-form" method="get" action="/search">
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
        </div>
`, html.EscapeString(item.Title), html.EscapeString(yearStr), html.EscapeString(item.Type))
		}
		_, _ = fmt.Fprint(w, `    </div>`)
	}

	_, _ = fmt.Fprint(w, `
</body>
</html>`)
}
