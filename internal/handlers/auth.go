// Package handlers provides HTTP request handlers for TapeDeck.
package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	sessionStore *sessions.CookieStore
	plexAuth     *plex.AuthClient
	db           *db.DB
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(store *sessions.CookieStore, plexAuth *plex.AuthClient, database *db.DB) *AuthHandler {
	return &AuthHandler{
		sessionStore: store,
		plexAuth:     plexAuth,
		db:           database,
	}
}

// Login handles the GET /auth/login endpoint
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Check if already authenticated
	session := getOrCreateSession(h.sessionStore, r)
	if _, ok := middleware.GetUserID(session); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Request a PIN from Plex
	pin, err := h.plexAuth.RequestPIN()
	if err != nil {
		log.Printf("Failed to request PIN: %v", err)
		http.Error(w, "Failed to initiate Plex authentication", http.StatusInternalServerError)
		return
	}

	// Store PIN ID in session for callback
	session.Values["plex_pin_id"] = pin.ID
	session.Values["plex_pin_code"] = pin.Code
	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	// Get Plex auth URL
	callbackURL := fmt.Sprintf("%s://%s/auth/callback", scheme(r), r.Host)
	authURL := h.plexAuth.GetAuthURL(pin.Code, callbackURL)

	// Render login page with auth URL
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Login - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 600px; margin: 50px auto; text-align: center; }
        .button { display: inline-block; padding: 15px 30px; background: #e5a00d; color: white;
                  text-decoration: none; border-radius: 5px; font-weight: bold; margin: 20px 0; }
        .button:hover { background: #cc8f0a; }
        .pin-code { font-size: 24px; font-weight: bold; margin: 20px 0; letter-spacing: 4px; }
    </style>
</head>
<body>
    <h1>🎬 TapeDeck</h1>
    <p>Connect your Plex account to get started</p>
    <div class="pin-code">%s</div>
    <a href="%s" class="button">Login with Plex</a>
    <p><small>Or visit <a href="https://plex.tv/link">plex.tv/link</a> and enter the code above</small></p>
</body>
</html>`, pin.Code, authURL)
}

// Callback handles the GET /auth/callback endpoint after Plex authorization
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	// Get PIN ID from session
	pinIDVal, ok := session.Values["plex_pin_id"]
	if !ok {
		http.Error(w, "No authentication in progress", http.StatusBadRequest)
		return
	}

	pinID, ok := pinIDVal.(int)
	if !ok {
		http.Error(w, "Invalid session data", http.StatusBadRequest)
		return
	}

	// Check PIN status (poll for auth token)
	var authToken string
	for i := 0; i < 10; i++ {
		check, err := h.plexAuth.CheckPIN(pinID)
		if err != nil {
			log.Printf("Failed to check PIN: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if check.AuthToken != "" {
			authToken = check.AuthToken
			break
		}

		time.Sleep(time.Second)
	}

	if authToken == "" {
		http.Error(w, "Authentication not completed. Please try again.", http.StatusUnauthorized)
		return
	}

	// Get user info from Plex using the auth token
	// For now, we'll use a placeholder - in a full implementation,
	// you'd call Plex API to get user details
	plexUserID := "plex-user-" + authToken[:10]
	plexUsername := "PlexUser"

	// Get or create user in database
	user, err := h.db.GetUserByPlexUserID(plexUserID)
	if err != nil {
		// User doesn't exist, create new one
		user = models.NewUser(plexUsername, plexUserID, authToken)
		userID, err := h.db.CreateUser(user)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			http.Error(w, "Failed to create user account", http.StatusInternalServerError)
			return
		}
		user.ID = userID
	} else {
		// Update existing user's token
		user.PlexAuthToken = authToken
		user.UpdatedAt = time.Now()
		if err := h.db.UpdateUser(user); err != nil {
			log.Printf("Failed to update user: %v", err)
		}
	}

	// Store user ID in session
	middleware.SetUserID(session, user.ID)
	delete(session.Values, "plex_pin_id")
	delete(session.Values, "plex_pin_code")

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout handles the POST /auth/logout endpoint
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	middleware.ClearSession(session)

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

// getOrCreateSession retrieves or creates a session
func getOrCreateSession(store *sessions.CookieStore, r *http.Request) *sessions.Session {
	session, _ := store.Get(r, middleware.SessionName)
	return session
}

// scheme returns the request scheme (http or https)
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
