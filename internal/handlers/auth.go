// Package handlers provides HTTP request handlers for TapeDeck.
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/sessions"
)

var debugLog *os.File

func init() {
	var err error
	debugLog, err = os.OpenFile("/tmp/tapedeck-auth-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Failed to open debug log: %v", err)
	}
}

func logDebug(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print(msg)
	if debugLog != nil {
		_, _ = debugLog.WriteString(time.Now().Format("2006/01/02 15:04:05") + " " + msg + "\n")
		_ = debugLog.Sync()
	}
}

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	sessionStore *sessions.CookieStore
	plexAuth     plex.AuthClientInterface
	db           *db.DB
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(store *sessions.CookieStore, plexAuth plex.AuthClientInterface, database *db.DB) *AuthHandler {
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

	// Check if we already have a PIN in the session (avoid generating multiple PINs)
	var pin *plex.PINResponse
	if existingPinID, ok := session.Values["plex_pin_id"].(int); ok {
		if existingPinCode, ok := session.Values["plex_pin_code"].(string); ok {
			// Reuse existing PIN from session
			pin = &plex.PINResponse{
				ID:   existingPinID,
				Code: existingPinCode,
			}
			log.Printf("DEBUG Login: Reusing existing PIN ID=%d, Code='%s'", pin.ID, pin.Code)
		}
	}

	// If no existing PIN, request a new one from Plex
	if pin == nil {
		var err error
		pin, err = h.plexAuth.RequestPIN()
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

		log.Printf("DEBUG Login: Created NEW PIN ID=%d, Code='%s', stored in session", pin.ID, pin.Code)
	}

	// Render login page with polling
	if err := pages.AuthLogin(pin.Code).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// PollStatus handles the GET /auth/poll-status endpoint for JavaScript polling
//
// NOTE: This polling approach is a workaround because Plex broke OAuth for 3rd party apps
// in v4.152.0 (Sept 2025). The recommended forwardUrl redirect flow no longer works.
// We poll every 5 seconds to check if the user has authorized the PIN on plex.tv/link.
//
// See: https://forums.plex.tv/t/plex-oauth-authenticate-with-plex-broken-after-plex-web-update-v4-152-0/931098
// TODO: Switch back to forwardUrl redirect flow when Plex fixes their OAuth implementation
func (h *AuthHandler) PollStatus(w http.ResponseWriter, r *http.Request) {
	logDebug("DEBUG PollStatus: Received poll request from %s", r.RemoteAddr)

	session := getOrCreateSession(h.sessionStore, r)

	// Get PIN ID from session
	pinIDVal, ok := session.Values["plex_pin_id"]
	if !ok {
		logDebug("DEBUG PollStatus: No plex_pin_id in session")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	pinID, ok := pinIDVal.(int)
	if !ok {
		logDebug("DEBUG PollStatus: plex_pin_id type assertion failed, got type %T", pinIDVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	logDebug("DEBUG PollStatus: Checking PIN ID=%d", pinID)

	// Check PIN status to get auth token
	check, err := h.plexAuth.CheckPIN(pinID)
	if err != nil {
		// Don't log 429 rate limit errors - they're expected with polling
		if err.Error() != "unexpected status code: 429" {
			log.Printf("Failed to check PIN: %v", err)
		}
		// Return 429 status so client can back off
		if err.Error() == "unexpected status code: 429" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	authToken := check.AuthToken
	if authToken == "" {
		logDebug("DEBUG PollStatus: AuthToken still empty for PIN ID=%d, Code=%s", pinID, check.Code)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Token is ready! Create/update user and set session
	logDebug("✅ SUCCESS: Received auth token from Plex via polling! Token='%s' (len=%d)", authToken, len(authToken))
	plexUserID := "plex-user-" + authToken[:10]
	plexUsername := "PlexUser"

	user, err := h.db.GetUserByPlexUserID(plexUserID)
	if err != nil {
		// User doesn't exist, create new one
		user = models.NewUser(plexUsername, plexUserID, authToken)
		userID, err := h.db.CreateUser(user)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	log.Printf("User authenticated successfully via polling")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": true})
}

// Logout handles the POST /auth/logout endpoint
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	middleware.ClearSession(session)

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	// Check for redirect parameter
	redirectTo := r.URL.Query().Get("redirect")
	if redirectTo == "" {
		redirectTo = "/auth/login"
	}

	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// getOrCreateSession retrieves or creates a session
func getOrCreateSession(store *sessions.CookieStore, r *http.Request) *sessions.Session {
	session, _ := store.Get(r, middleware.SessionName)
	return session
}
