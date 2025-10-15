// Package middleware provides HTTP middleware for TapeDeck.
package middleware

import (
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/constants"
	"github.com/gorilla/sessions"
)

const (
	// SessionName is the name of the session cookie
	SessionName = "tapedeck-session"
	// UserIDKey is the session key for storing user ID
	UserIDKey = "user_id"
)

// NewSessionStore creates a new session store with the given secret key
func NewSessionStore(secret []byte, requireTLS bool) *sessions.CookieStore {
	store := sessions.NewCookieStore(secret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(constants.SessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   requireTLS,
		SameSite: http.SameSiteLaxMode,
	}
	return store
}

// SetUserID stores the user ID in the session
func SetUserID(session *sessions.Session, userID int64) {
	session.Values[UserIDKey] = userID
}

// GetUserID retrieves the user ID from the session
func GetUserID(session *sessions.Session) (int64, bool) {
	val, ok := session.Values[UserIDKey]
	if !ok {
		return 0, false
	}

	userID, ok := val.(int64)
	if !ok {
		return 0, false
	}

	return userID, true
}

// ClearSession removes all values from the session
func ClearSession(session *sessions.Session) {
	session.Values = make(map[interface{}]interface{})
	session.Options.MaxAge = -1 // Mark for deletion
}

// RequireAuth is middleware that requires authentication
func RequireAuth(store *sessions.CookieStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, SessionName)
			if err != nil {
				// Session error, redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			_, ok := GetUserID(session)
			if !ok {
				// Not authenticated, redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			// Authenticated, continue
			next.ServeHTTP(w, r)
		})
	}
}
