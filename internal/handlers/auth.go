// Package handlers provides HTTP request handlers for TapeDeck.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/sessions"
)

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
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	// Check if already authenticated
	session := getOrCreateSession(h.sessionStore, r)
	if _, ok := middleware.GetUserID(session); ok {
		// Use redirect parameter if provided, otherwise go to /
		redirectTo := r.URL.Query().Get("redirect")
		validatedRedirect, err := ValidateRedirectPath(redirectTo)
		if err != nil {
			log.Warn("Invalid redirect path", "error", err, "redirect", redirectTo)
			validatedRedirect = "/"
		}
		http.Redirect(w, r, validatedRedirect, http.StatusFound)
		return
	}

	// Store redirect URL in session if provided (validate it first)
	redirectTo := r.URL.Query().Get("redirect")
	if redirectTo != "" {
		validatedRedirect, err := ValidateRedirectPath(redirectTo)
		if err != nil {
			log.Warn("Invalid redirect path", "error", err, "redirect", redirectTo)
		} else {
			session.Values["auth_redirect"] = validatedRedirect
		}
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
		}
	}

	// If no existing PIN, request a new one from Plex
	if pin == nil {
		var err error
		pin, err = h.plexAuth.RequestPIN(ctx)
		if err != nil {
			log.Error("Failed to request PIN", "error", err)
			RespondError(w, r, "Failed to initiate Plex authentication", http.StatusInternalServerError)
			return
		}

		// Store PIN ID in session for callback
		session.Values["plex_pin_id"] = pin.ID
		session.Values["plex_pin_code"] = pin.Code
		if err := session.Save(r, w); err != nil {
			log.Error("Failed to save session", "error", err)
			RespondError(w, r, "Session error", http.StatusInternalServerError)
			return
		}
	}

	// Render login page with polling
	if err := pages.AuthLogin(pin.Code).Render(ctx, w); err != nil {
		log.Error("Failed to render template", "error", err)
		RespondError(w, r, "Failed to render page", http.StatusInternalServerError)
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
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	session := getOrCreateSession(h.sessionStore, r)

	// Get PIN ID from session
	pinIDVal, ok := session.Values["plex_pin_id"]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	pinID, ok := pinIDVal.(int)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Check PIN status to get auth token
	check, err := h.plexAuth.CheckPIN(ctx, pinID)
	if err != nil {
		// Don't log 429 rate limit errors - they're expected with polling
		if err.Error() != "unexpected status code: 429" {
			log.Warn("Failed to check PIN", "error", err)
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Token is ready! Create/update user and set session
	plexUserID := "plex-user-" + authToken[:10]
	plexUsername := "PlexUser"

	user, err := h.db.GetUserByPlexUserID(ctx, plexUserID)
	if err != nil {
		// User doesn't exist, create new one
		user = models.NewUser(plexUsername, plexUserID, authToken)
		userID, err := h.db.CreateUser(ctx, user)
		if err != nil {
			log.Error("Failed to create user", "error", err)
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
		if err := h.db.UpdateUser(ctx, user); err != nil {
			log.Warn("Failed to update user", "error", err)
		}
	}

	// Get redirect URL from session (if set) and validate it
	redirectTo := "/"
	if redirect, ok := session.Values["auth_redirect"].(string); ok && redirect != "" {
		validatedRedirect, err := ValidateRedirectPath(redirect)
		if err != nil {
			log.Warn("Invalid redirect path from session", "error", err, "redirect", redirect)
		} else {
			redirectTo = validatedRedirect
		}
	}

	// Store user ID in session
	middleware.SetUserID(session, user.ID)
	delete(session.Values, "plex_pin_id")
	delete(session.Values, "plex_pin_code")
	delete(session.Values, "auth_redirect")

	if err := session.Save(r, w); err != nil {
		log.Error("Failed to save session", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	log.Info("User authenticated successfully via polling")

	// Return success with redirect URL
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authorized": true,
		"redirect":   redirectTo,
	})
}

// Logout handles the POST /auth/logout endpoint
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := middleware.GetLogger(ctx)

	session := getOrCreateSession(h.sessionStore, r)

	middleware.ClearSession(session)

	if err := session.Save(r, w); err != nil {
		log.Error("Failed to save session", "error", err)
	}

	// Check for redirect parameter (validate to prevent open redirects)
	redirectTo := r.URL.Query().Get("redirect")
	if redirectTo == "" {
		// No redirect specified, use default
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	validatedRedirect, err := ValidateRedirectPath(redirectTo)
	if err != nil {
		log.Warn("Invalid redirect path", "error", err, "redirect", redirectTo)
		validatedRedirect = "/auth/login"
	}

	http.Redirect(w, r, validatedRedirect, http.StatusFound)
}

// getOrCreateSession retrieves or creates a session
func getOrCreateSession(store *sessions.CookieStore, r *http.Request) *sessions.Session {
	session, _ := store.Get(r, middleware.SessionName)
	return session
}

// handlePlexUnauthorized checks if the error is a 401 Unauthorized from Plex
// and redirects to login if the token has been revoked. Returns true if handled.
func handlePlexUnauthorized(w http.ResponseWriter, r *http.Request, err error, sessionStore *sessions.CookieStore) bool {
	if !plex.IsUnauthorized(err) {
		return false
	}

	log := middleware.GetLogger(r.Context())
	log.Warn("Plex token unauthorized - clearing session and redirecting to login")

	// Clear the session
	session := getOrCreateSession(sessionStore, r)
	middleware.ClearSession(session)
	if saveErr := session.Save(r, w); saveErr != nil {
		log.Error("Failed to save session during logout", "error", saveErr)
	}

	// Redirect to login with redirect back to current page
	redirectURL := "/auth/login?redirect=" + r.URL.Path
	http.Redirect(w, r, redirectURL, http.StatusFound)
	return true
}
