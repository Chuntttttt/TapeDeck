package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gorilla/sessions"

	"github.com/Chuntttttt/tapedeck/internal/models"
)

// userContextKey is the context key for the full User object
const userContextKey contextKey = "user"

// DBReader defines the interface for reading user data from the database
type DBReader interface {
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
}

// WithUser fetches the user from the database and adds it to the request context.
// This eliminates the need for handlers to repeatedly fetch the user.
//
// Unlike WithUserID (which only adds the user ID), this middleware fetches the
// complete User object including the Plex auth token.
//
// If the user cannot be fetched (no session, database error), the request continues
// without a user in context. Use GetUserFromContext to check if a user is present.
func WithUser(store *sessions.CookieStore, db DBReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			log := slog.Default()

			// Get user ID from context (added by WithUserID middleware)
			userID, ok := GetUserIDFromContext(ctx)
			if !ok {
				// No user ID in context, continue without user
				next.ServeHTTP(w, r)
				return
			}

			// Fetch user from database
			user, err := db.GetUserByID(ctx, userID)
			if err != nil {
				log.Warn("Failed to fetch user for context", "user_id", userID, "error", err)
				// Continue without user in context
				next.ServeHTTP(w, r)
				return
			}

			// Add user to context
			ctx = context.WithValue(ctx, userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves the User object from the request context.
// Returns nil and false if no user is in the context.
func GetUserFromContext(ctx context.Context) (*models.User, bool) {
	user, ok := ctx.Value(userContextKey).(*models.User)
	return user, ok
}
