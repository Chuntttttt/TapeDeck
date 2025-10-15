# Chi Router Migration Plan

**Goal**: Replace stdlib `http.ServeMux` with `go-chi/chi` to eliminate route parsing, reduce main.go complexity, and improve handler organization.

**Related Issues**: #3

## Benefits

- Path parameters: `r.Get("/mappings/{id}/edit", ...)` replaces `fmt.Sscanf` parsing
- Middleware groups: Apply middleware to route groups, not individual handlers
- Sub-routers: Break main.go routing into logical modules
- Better HTTP method handling: `r.Get()`, `r.Post()`, `r.Delete()` vs manual `if r.Method == ...`
- Eliminates 200+ lines of boilerplate from main.go

## Migration Strategy

### Phase 1: Install and Basic Setup
```bash
go get -u github.com/go-chi/chi/v5
```

Create `internal/router/router.go`:
```go
package router

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func New(deps *RouterDependencies) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middlewareLogger())
    r.Use(customMiddleware.MetricsMiddleware())

    // Mount sub-routers
    r.Mount("/auth", authRouter(deps.AuthHandler))
    r.Mount("/setup", setupRouter(deps.SetupHandler))
    r.Mount("/mappings", mappingsRouter(deps.MappingsHandler))
    // ... etc

    return r
}
```

### Phase 2: Convert Route Groups (One at a Time)

**Example: Mappings Routes**

Before (main.go):
```go
mux.Handle("/mappings", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if mappingsHandler == nil {
        http.Redirect(w, r, "/setup", http.StatusSeeOther)
        return
    }
    if r.Method == http.MethodPost {
        mappingsHandler.CreateMapping(w, r)
    } else {
        mappingsHandler.Dashboard(w, r)
    }
})))

mux.Handle("/mappings/", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if mappingsHandler == nil {
        http.Redirect(w, r, "/setup", http.StatusSeeOther)
        return
    }
    mappingsRouteHandler(mappingsHandler)(w, r)
})))
```

After (internal/router/mappings.go):
```go
func mappingsRouter(h *handlers.MappingsHandler, auth func(http.Handler) http.Handler) chi.Router {
    r := chi.NewRouter()
    r.Use(auth) // Apply auth to all routes
    r.Use(requireInitialized(h)) // Check handler not nil

    r.Get("/", h.Dashboard)
    r.Post("/", h.CreateMapping)
    r.Get("/new", h.NewMappingForm)
    r.Get("/pair", h.PairForm) // This is actually in pairingHandler, fix later

    r.Route("/{id}", func(r chi.Router) {
        r.Get("/edit", h.EditMappingForm)
        r.Post("/", h.UpdateMapping)
        r.Post("/delete", h.DeleteMapping)
    })

    return r
}
```

**Handler changes** - Extract path params in handler methods:
```go
// Before
func mappingsRouteHandler(h *handlers.MappingsHandler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path
        remainder := path[len("/mappings/"):]
        var mappingID int64
        if n, err := fmt.Sscanf(remainder, "%d/edit", &mappingID); n == 1 && err == nil {
            h.EditMappingForm(w, r, mappingID)
            return
        }
        // ... more Sscanf hell
    }
}

// After
func (h *MappingsHandler) EditMappingForm(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    mappingID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "Invalid mapping ID", http.StatusBadRequest)
        return
    }

    // Existing logic...
}
```

### Phase 3: Handler Nil Check Middleware

Create `internal/middleware/initialized.go`:
```go
// requireInitialized checks if handler dependencies are ready
func RequireInitialized(ready func() bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !ready() {
                http.Redirect(w, r, "/setup", http.StatusSeeOther)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Usage in router:
```go
func New(deps *RouterDependencies) *chi.Mux {
    r := chi.NewRouter()

    // Routes that need handlers initialized
    initialized := middleware.RequireInitialized(deps.HandlersReady)

    r.Group(func(r chi.Router) {
        r.Use(initialized)
        r.Mount("/mappings", mappingsRouter(deps.MappingsHandler, deps.AuthMiddleware))
        r.Mount("/libraries", mediaRouter(deps.MediaHandler, deps.AuthMiddleware))
        // ...
    })

    return r
}
```

Main.go simplification:
```go
type RouterDependencies struct {
    AuthHandler     *handlers.AuthHandler
    SetupHandler    *handlers.SetupHandler
    MappingsHandler *handlers.MappingsHandler
    MediaHandler    *handlers.MediaHandler
    PairingHandler  *handlers.PairingHandler
    SettingsHandler *handlers.SettingsHandler
    StatusHandler   *handlers.StatusHandler
    PlaybackHandler *handlers.PlaybackHandler

    AuthMiddleware  func(http.Handler) http.Handler
    HandlersReady   func() bool
}

func main() {
    // ... setup code ...

    deps := &RouterDependencies{
        AuthHandler:  authHandler,
        SetupHandler: setupHandler,
        // Initially nil handlers
        AuthMiddleware: middleware.RequireAuth(sessionStore),
        HandlersReady: func() bool {
            return mediaHandler != nil && mappingsHandler != nil // etc
        },
    }

    // After config loaded, update deps
    initializeHandlers := func() error {
        // ... initialization logic ...
        deps.MappingsHandler = mappingsHandler
        deps.MediaHandler = mediaHandler
        // ...
        return nil
    }

    r := router.New(deps)
    server := &http.Server{
        Addr:    ":" + cfg.Port,
        Handler: r,
    }
}
```

### Phase 4: File Structure

```
internal/
  router/
    router.go        # Main router factory
    auth.go          # Auth sub-router
    setup.go         # Setup wizard sub-router
    mappings.go      # Mappings sub-router
    media.go         # Media browsing sub-router
    api.go           # API routes (/api/*)
    websocket.go     # WebSocket routes (/ws/*)
```

## Migration Checklist

- [ ] Install chi dependency
- [ ] Create `internal/router/` package structure
- [ ] Create `RouterDependencies` struct
- [ ] Migrate auth routes → `router/auth.go`
- [ ] Migrate setup routes → `router/setup.go`
- [ ] Migrate mappings routes → `router/mappings.go`
  - [ ] Update handler methods to extract path params with `chi.URLParam()`
  - [ ] Remove `mappingsRouteHandler` helper
- [ ] Migrate media routes → `router/media.go`
  - [ ] Remove `libraryContentsHandler` helper
- [ ] Migrate API routes → `router/api.go`
- [ ] Migrate WebSocket routes → `router/websocket.go`
- [ ] Create `RequireInitialized` middleware
- [ ] Test all routes with curl/browser
- [ ] Update tests to use chi test helpers
- [ ] Remove old route handler helpers from main.go
- [ ] Confirm main.go is < 200 lines

## Testing Strategy

Chi provides test helpers:
```go
func TestMappingsRoutes(t *testing.T) {
    r := chi.NewRouter()
    r.Mount("/mappings", mappingsRouter(mockHandler, mockAuth))

    req := httptest.NewRequest("GET", "/mappings/123/edit", nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("id", "123")
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}
```

## Breaking Changes

None for users. Internal refactor only.

## Timeline Estimate

- Phase 1: 1 hour
- Phase 2: 4-6 hours (route groups)
- Phase 3: 1 hour (middleware)
- Phase 4: 1 hour (organization)
- Testing: 2 hours

**Total: 1-2 days**

## Risks

- Chi uses `context.Context` for URL params; ensure all handlers have access to request context
- Route matching behavior may differ slightly from stdlib patterns (test thoroughly)
- Middleware ordering matters in Chi (more explicit than stdlib)

## Alternative Considered

**Echo** - More opinionated, includes built-in validation/binding. Overkill for this app.
**Gorilla Mux** - Similar to Chi but less idiomatic (uses custom types vs stdlib).
**Chi** - Best balance: stdlib-compatible, minimal overhead, idiomatic Go.
