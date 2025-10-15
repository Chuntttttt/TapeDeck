# Context Propagation Guide

**Goal**: Understand and implement context propagation for request tracing, cancellation, and scoped logging throughout the application.

## What is Context Propagation?

Go's `context.Context` is a mechanism for carrying request-scoped data, cancellation signals, and deadlines across API boundaries. Every HTTP request in Go has a context accessible via `r.Context()`.

**Without context propagation:**
```
HTTP Request → Handler → Service → Database
      ↓            ↓         ↓          ↓
  (isolated) (isolated) (isolated) (isolated)
```
Each layer is isolated. You can't:
- Trace a request through all layers
- Cancel long-running operations when client disconnects
- Attach request-specific data (user ID, request ID)

**With context propagation:**
```
HTTP Request → Handler → Service → Database
      ↓            ↓         ↓          ↓
    ctx --------→ ctx ----→ ctx -----→ ctx
                   ↓
              (shared context with request ID, user ID, logger)
```

## Current Problems in TapeDeck

### Problem 1: No Request Tracing

When you see this in logs:
```
2025-01-14 10:23:45 Failed to create mapping: invalid tag_id
2025-01-14 10:23:45 WebSocket client connected (userID=5)
2025-01-14 10:23:45 Failed to get library contents from server
```

**Which request failed?** You can't tell. They might be from different users, different requests, or the same request at different stages.

### Problem 2: No Cancellation

```go
// handlers/media.go:273
for _, result := range results {
    if result.err != nil {
        log.Printf("Failed to search server %s: %v", result.serverName, result.err)
        continue
    }
    // ...
}
```

If the client cancels the request (closes browser tab), the handler keeps searching all servers. Wasteful.

### Problem 3: Lost User Context

```go
// handlers/mappings.go:88
userID, ok := middleware.GetUserID(session)
```

Every handler extracts user ID from session. This data should flow through context so downstream code (services, DB layer) can use it for logging/auditing.

### Problem 4: No Timeouts

```go
// plex/client.go:94
resp, err := c.httpClient.Do(req)
```

Request has no timeout. If Plex server hangs, request hangs forever.

## Solution: Context Propagation Pattern

### Step 1: Middleware Adds Request Context

Create `internal/middleware/context.go`:

```go
package middleware

import (
    "context"
    "net/http"

    "github.com/Chuntttttt/tapedeck/internal/logger"
    "github.com/google/uuid"
)

type contextKey string

const (
    RequestIDKey contextKey = "request_id"
    UserIDKey    contextKey = "user_id"
    LoggerKey    contextKey = "logger"
)

// WithRequestID adds a unique request ID to context
func WithRequestID() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            requestID := uuid.New().String()
            ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

            // Add to response headers for debugging
            w.Header().Set("X-Request-ID", requestID)

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// WithRequestLogger creates a request-scoped logger with context
func WithRequestLogger() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()

            // Build logger with request context
            requestID, _ := ctx.Value(RequestIDKey).(string)
            reqLogger := logger.Get().With(
                "request_id", requestID,
                "path", r.URL.Path,
                "method", r.Method,
            )

            // Add user ID if authenticated
            if userID, ok := ctx.Value(UserIDKey).(int64); ok {
                reqLogger = reqLogger.With("user_id", userID)
            }

            // Store logger in context
            ctx = context.WithValue(ctx, LoggerKey, reqLogger)

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// WithUserID extracts user ID from session and adds to context
func WithUserID(store *sessions.CookieStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            session, err := store.Get(r, SessionName)
            if err != nil {
                next.ServeHTTP(w, r)
                return
            }

            userID, ok := GetUserID(session)
            if ok {
                ctx := context.WithValue(r.Context(), UserIDKey, userID)
                r = r.WithContext(ctx)
            }

            next.ServeHTTP(w, r)
        })
    }
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) string {
    if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
        return requestID
    }
    return "unknown"
}

// GetUserIDFromContext retrieves user ID from context
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
    if userID, ok := ctx.Value(UserIDKey).(int64); ok {
        return userID, true
    }
    return 0, false
}

// GetLogger retrieves the request-scoped logger from context
func GetLogger(ctx context.Context) *slog.Logger {
    if log, ok := ctx.Value(LoggerKey).(*slog.Logger); ok {
        return log
    }
    return logger.Get()
}
```

### Step 2: Apply Middleware in Main

```go
// main.go
func main() {
    // ... setup ...

    r := router.New(deps)

    // Middleware chain (order matters!)
    handler := middleware.WithRequestID()(
        middleware.WithUserID(sessionStore)(
            middleware.WithRequestLogger()(
                middleware.MetricsMiddleware()(
                    middleware.RequestLogger()(
                        middleware.SetupMiddleware("./config.yml", sessionStore)(r),
                    ),
                ),
            ),
        ),
    )

    // ... server setup ...
}
```

### Step 3: Use Context in Handlers

**Before:**
```go
// handlers/mappings.go
func (h *MappingsHandler) CreateMapping(w http.ResponseWriter, r *http.Request) {
    session, _ := h.sessionStore.Get(r, middleware.SessionName)
    userID, ok := middleware.GetUserID(session)
    if !ok {
        log.Printf("CreateMapping: User not authenticated")
        http.Error(w, "Not authenticated", http.StatusUnauthorized)
        return
    }

    log.Printf("CreateMapping: Received form data - tagID=%s, mediaType=%s, mediaID=%s",
        tagID, mediaType, mediaID)
    // ...
}
```

**After:**
```go
func (h *MappingsHandler) CreateMapping(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    log := middleware.GetLogger(ctx)

    userID, ok := middleware.GetUserIDFromContext(ctx)
    if !ok {
        log.Warn("Create mapping attempted without authentication")
        http.Error(w, "Not authenticated", http.StatusUnauthorized)
        return
    }

    log.Info("Creating mapping",
        "tag_id", tagID,
        "media_type", mediaType,
        "media_id", mediaID)

    // Pass context to service layer
    mapping, err := h.mappingService.CreateMapping(ctx, &CreateMappingRequest{
        UserID:     userID,
        TagID:      tagID,
        MediaType:  mediaType,
        MediaID:    mediaID,
        // ...
    })
    // ...
}
```

### Step 4: Propagate Through Service Layer

```go
// services/playback.go
func (s *PlaybackService) Play(ctx context.Context, req *PlaybackRequest) (*PlaybackResult, error) {
    log := middleware.GetLogger(ctx)

    log.Info("Starting playback",
        "media_title", req.MediaTitle,
        "apple_tv", req.AppleTVEntity)

    // Pass context to HA REST client
    if err := s.haRest.PlayMedia(ctx, req.AppleTVEntity, "url", plexURL); err != nil {
        log.Error("Playback failed", "error", err)
        return nil, err
    }

    // Pass context to DB
    if err := s.db.CreatePlaybackLog(ctx, playbackLog); err != nil {
        log.Warn("Failed to log playback", "error", err)
    }

    log.Info("Playback started successfully")
    return &PlaybackResult{Success: true}, nil
}
```

### Step 5: Add Timeouts with Context

**Plex Client:**
```go
// plex/client.go
func (c *Client) Search(ctx context.Context, query string) ([]MediaItem, error) {
    // Create request with context
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("X-Plex-Token", c.authToken)
    req.Header.Set("Accept", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        // If context was cancelled, this will be context.Canceled or context.DeadlineExceeded
        return nil, fmt.Errorf("failed to make request: %w", err)
    }
    defer resp.Body.Close()

    // ...
}
```

**Handler with timeout:**
```go
// handlers/media.go
func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
    // Create timeout context for search
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()

    log := middleware.GetLogger(ctx)
    query := r.URL.Query().Get("q")

    results, err := h.searchService.Search(ctx, query)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            log.Warn("Search timed out", "query", query)
            http.Error(w, "Search timed out", http.StatusGatewayTimeout)
            return
        }
        log.Error("Search failed", "error", err)
        http.Error(w, "Search failed", http.StatusInternalServerError)
        return
    }

    // ...
}
```

### Step 6: Database Context Support

```go
// db/db.go
func (db *DB) CreateCardMapping(ctx context.Context, mapping *models.CardMapping) (int64, error) {
    if err := mapping.Validate(); err != nil {
        return 0, fmt.Errorf("invalid card mapping: %w", err)
    }

    // Use context for query (allows cancellation)
    result, err := db.conn.ExecContext(ctx,
        `INSERT INTO card_mappings (user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        mapping.UserID,
        mapping.TagID,
        mapping.MediaType,
        mapping.MediaID,
        mapping.MediaTitle,
        mapping.PlexServerID,
        mapping.AppleTVEntity,
        mapping.CreatedAt,
        mapping.UpdatedAt,
    )
    if err != nil {
        return 0, fmt.Errorf("failed to insert card mapping: %w", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return 0, fmt.Errorf("failed to get last insert ID: %w", err)
    }

    return id, nil
}
```

## Example: Tracing a Request

**Before (unstructured):**
```
2025-01-14 10:23:45 CreateMapping: Received form data - tagID=abc123
2025-01-14 10:23:45 Failed to search server plex-1
2025-01-14 10:23:45 WebSocket client connected (userID=5)
2025-01-14 10:23:45 CreateMapping: Successfully created mapping with ID=42
```
Which logs belong together? Unclear.

**After (with context):**
```
time=2025-01-14T10:23:45 level=INFO msg="Creating mapping" request_id=a1b2c3d4 user_id=5 tag_id=abc123 path=/mappings
time=2025-01-14T10:23:46 level=ERROR msg="Server search failed" request_id=a1b2c3d4 user_id=5 server=plex-1 error="connection timeout"
time=2025-01-14T10:23:47 level=INFO msg="Mapping created" request_id=a1b2c3d4 user_id=5 mapping_id=42
time=2025-01-14T10:23:48 level=INFO msg="WebSocket client connected" request_id=e5f6g7h8 user_id=5
```
Now you can `grep request_id=a1b2c3d4` to see the entire request flow.

## Migration Checklist

- [ ] Install `github.com/google/uuid` for request IDs
- [ ] Create `middleware/context.go` with context helpers
- [ ] Add `WithRequestID()` middleware
- [ ] Add `WithRequestLogger()` middleware
- [ ] Add `WithUserID()` middleware
- [ ] Update main.go middleware chain
- [ ] Update all handlers to use `middleware.GetLogger(ctx)`
- [ ] Update all handlers to use `middleware.GetUserIDFromContext(ctx)`
- [ ] Add `context.Context` parameter to service methods
- [ ] Add `context.Context` parameter to DB methods (use `ExecContext`, `QueryContext`, `QueryRowContext`)
- [ ] Add `context.Context` parameter to Plex client methods (use `http.NewRequestWithContext`)
- [ ] Add `context.Context` parameter to HA REST client methods
- [ ] Add timeout contexts where appropriate (searches, external API calls)
- [ ] Update tests to pass `context.Background()` or `context.TODO()`

## Benefits After Implementation

✅ **Trace requests end-to-end** with request ID
✅ **Cancel operations** when client disconnects
✅ **Enforce timeouts** on long-running operations
✅ **User context flows** through all layers (no repeated session lookups)
✅ **Scoped logging** automatically includes request metadata
✅ **Better observability** for debugging production issues

## Timeline Estimate

- Middleware creation: 1 hour
- Handler updates: 3 hours
- Service layer updates: 2 hours
- DB layer updates: 1 hour
- Plex/HA client updates: 1 hour
- Testing: 2 hours

**Total: 10 hours (1.5 days)**

## Further Reading

- [Go Context Best Practices](https://go.dev/blog/context)
- [Context-aware database operations](https://pkg.go.dev/database/sql#DB.ExecContext)
- [Request ID tracking patterns](https://www.alexedwards.net/blog/how-to-use-request-context)
