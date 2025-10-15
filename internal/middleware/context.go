package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	// requestIDKey is the context key for request IDs
	requestIDKey contextKey = "request_id"
	// userIDContextKey is the context key for user IDs in request context
	// (distinct from UserIDKey which is used for session storage)
	userIDContextKey contextKey = "user_id_context"
	// loggerKey is the context key for request-scoped loggers
	loggerKey contextKey = "logger"
)

// WithRequestID adds a unique request ID to the context and response headers
func WithRequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-ID", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithRequestLogger creates a request-scoped logger with request metadata
func WithRequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID, _ := ctx.Value(requestIDKey).(string)

			// Get the default logger and add request metadata
			reqLogger := slog.Default().With(
				"request_id", requestID,
				"path", r.URL.Path,
				"method", r.Method,
			)

			// Add user ID if available
			if userID, ok := ctx.Value(userIDContextKey).(int64); ok {
				reqLogger = reqLogger.With("user_id", userID)
			}

			ctx = context.WithValue(ctx, loggerKey, reqLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithUserID extracts the user ID from the session and adds it to the context
func WithUserID(store *sessions.CookieStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, SessionName)
			if err != nil {
				// If session retrieval fails, continue without user ID
				next.ServeHTTP(w, r)
				return
			}

			userID, ok := GetUserID(session)
			if ok {
				ctx := context.WithValue(r.Context(), userIDContextKey, userID)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// GetUserIDFromContext retrieves the user ID from the context
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDContextKey).(int64)
	return userID, ok
}

// GetLogger retrieves the request-scoped logger from the context
func GetLogger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	// Fallback to default logger if not in context
	return slog.Default()
}
