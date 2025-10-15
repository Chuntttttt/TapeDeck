# TapeDeck Refactoring Plan

This document contains the step-by-step plan for addressing code quality issues identified during code review.

---

## Issue #1: Remove Debug Logging in Production Code

### Files to modify:
1. `internal/handlers/auth.go` - 10 changes
2. `internal/plex/auth.go` - 2 changes

### Detailed steps:

**Step 1: Clean up `internal/handlers/auth.go`**
- Remove lines 19-27: Delete `var debugLog *os.File` and the entire `init()` function
- Remove lines 29-36: Delete the entire `logDebug()` function
- Remove line 73: `log.Printf("DEBUG Login: Reusing existing PIN...")`
- Remove line 96: `log.Printf("DEBUG Login: Created NEW PIN...")`
- Remove line 115: `logDebug("DEBUG PollStatus: Received poll request...")`
- Remove line 122: `logDebug("DEBUG PollStatus: No plex_pin_id in session")`
- Remove line 131: `logDebug("DEBUG PollStatus: plex_pin_id type assertion failed...")`
- Remove line 138: `logDebug("DEBUG PollStatus: Checking PIN ID=%d", pinID)`
- Remove line 162: `logDebug("DEBUG PollStatus: AuthToken still empty...")`
- Remove line 170: `logDebug("✅ SUCCESS: Received auth token...")`

**Step 2: Clean up `internal/plex/auth.go`**
- Remove line 103: `log.Printf("DEBUG RequestPIN: ID=%d, Code='%s'...")`
- Remove lines 140-141: `log.Printf("DEBUG CheckPIN: ID=%d, Code='%s', AuthToken='%s'...")`

**Step 3: Remove unused imports**
After removing the code, check if these imports are still needed:
- `"os"` in `auth.go` (only used for debugLog)
- `"fmt"` in `auth.go` (might be used elsewhere, needs verification)

**Step 4: Verify**
- Run `go test ./internal/handlers/...` to ensure tests still pass
- Run `go test ./internal/plex/...`
- Start the app and verify auth flow still works
- Check that no `/tmp/tapedeck-auth-debug.log` file is created

### Impact:
- **Risk**: Low - only removing debug code
- **Breaking changes**: None
- **Testing needed**: Auth login flow, PIN polling
- **Lines removed**: ~30 lines

---

## Issue #2: Handler Initialization Pattern is Fragile

### Current Problem:
- 12 routes manually check `if handler == nil` and redirect to `/setup`
- Each route wrapped in ~7 lines of boilerplate
- This is **redundant** - `SetupMiddleware` (main.go:124) already checks config and redirects
- The middleware wraps the entire mux, so these routes can't be accessed without valid config
- Handlers are initialized synchronously before setup redirects (setup.go:482-490)
- No timing gap where config exists but handlers are nil

### Files to modify:
1. `main.go` - Simplify 12 route handlers

### Detailed steps:

**Step 1: Remove nil check wrappers from routes**

Replace verbose wrappers like this:
```go
mux.Handle("/libraries", middleware.RequireAuth(sessionStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if mediaHandler == nil {
        http.Redirect(w, r, "/setup", http.StatusSeeOther)
        return
    }
    mediaHandler.Libraries(w, r)
})))
```

With simple direct calls:
```go
mux.Handle("/libraries", middleware.RequireAuth(sessionStore)(http.HandlerFunc(mediaHandler.Libraries)))
```

Routes to simplify:
- `/libraries` - Remove mediaHandler nil check
- `/libraries/` - Remove mediaHandler nil check (keep wrapper for path parsing)
- `/search` - Remove mediaHandler nil check
- `/mappings` - Remove mappingsHandler nil check
- `/mappings/new` - Remove mappingsHandler nil check
- `/mappings/pair` - Remove pairingHandler nil check
- `/mappings/` - Remove mappingsHandler nil check (keep wrapper for path parsing)
- `/api/search` - Remove mappingsHandler nil check
- `/ws/pairing` - Remove pairingHandler nil check
- `/api/status/ha` - Remove statusHandler nil check
- `/api/status/ha/reconnect` - Remove statusHandler nil check
- `/api/play` - Remove playbackHandler nil check

**Step 2: Add safety check at startup (belt and suspenders)**

Even though handlers are initialized before routes are accessible, add a panic check in initializeHandlers callback to catch programmer errors:

```go
initializeHandlers := func() error {
    // ... existing initialization code ...

    // Sanity check that all handlers were initialized
    if mediaHandler == nil || mappingsHandler == nil || playbackHandler == nil ||
       pairingHandler == nil || statusHandler == nil {
        return fmt.Errorf("handler initialization incomplete")
    }

    return nil
}
```

**Step 3: Document the design**

Add comment explaining why there are no nil checks:

```go
// Register application routes (protected by SetupMiddleware)
// SetupMiddleware ensures config exists before reaching these routes,
// and initializeHandlers() is called synchronously during setup,
// so handlers are guaranteed to be initialized when these routes are accessible.
```

**Step 4: Verify**

- Run `go test ./...` to ensure no tests break
- Test the complete setup flow
- Test the settings update flow (which re-initializes handlers)
- Verify routes return 404/redirect appropriately before setup completes

### Alternative (if concerned about race conditions):

If there's concern about Settings page re-initialization causing brief nil window:

**Option: Add mutex protection to handler access**
- Create `sync.RWMutex` around handler initialization
- Lock during re-initialization in settings
- RLock during handler access
- More complexity, probably overkill

### Impact:
- **Risk**: Low - middleware already protects these routes
- **Breaking changes**: None
- **Testing needed**: Setup flow, settings update, route access
- **Lines removed**: ~80 lines of boilerplate

---

## Issue #3: Dual Configuration Systems Create Confusion

### Current Problem:

Two overlapping configuration systems:

**1. Env-based config (`internal/config/config.go`)**:
```go
type Config struct {
    PlexURL       string  // ← Not used anywhere
    PlexServerID  string  // ← Not used anywhere
    HAURL         string  // ← Not used anywhere
    HAToken       string  // ← Used ONCE in wrong place (status.go:91)
    AppleTVEntity string  // ← Not used anywhere
    Port          string  // ✓ Actually used
    DatabasePath  string  // ✓ Actually used
    LogLevel      string  // ✓ Actually used
    SessionSecret string  // ✓ Actually used
    DevMode       bool    // ✓ Actually used
}
```

**2. YAML config (`internal/config/runtime.go`)**:
- Used by setup wizard
- Contains Plex servers, HA config, Apple TVs
- Actually read and used by handlers

**The confusion:**
- `Config.Load()` validates Plex/HA fields as required (line 46-59)
- But `main.go` ignores the error and uses defaults (line 42-52)
- Comments say "will be deprecated" but it's a new project
- `status.go` incorrectly loads env-based HAToken instead of reading from config.yml

### Decision: Commit to YAML for User Data

**Keep env vars ONLY for:**
- Port
- DatabasePath
- LogLevel
- SessionSecret
- DevMode

**Keep config.yml for:**
- Plex servers (multi-server support)
- Home Assistant URL/token
- Apple TVs (multi-device support)

### Files to modify:
1. `internal/config/config.go` - Remove legacy fields
2. `internal/config/config_test.go` - Update tests
3. `internal/handlers/status.go` - Fix HAToken source
4. `.env.example` - Already updated (verify)

### Detailed steps:

**Step 1: Simplify Config struct (`config.go`)**

Remove these fields:
```go
- PlexURL       string
- PlexServerID  string
- HAURL         string
- HAToken       string
- AppleTVEntity string
```

Remove validation for those fields (lines 46-59). Keep only:
```go
// Validate SESSION_SECRET is set
if cfg.SessionSecret == "" {
    return nil, fmt.Errorf("required environment variable SESSION_SECRET is not set")
}
```

Update comments to clarify separation:
```go
// Config holds application-level configuration from environment variables.
// User data (Plex servers, Home Assistant, etc.) is stored in config.yml
// and loaded via LoadRuntimeConfig().
```

**Step 2: Fix status.go HAReconnect**

Current (wrong):
```go
cfg, err := config.Load()  // Loads from .env
err = h.haClient.Reconnect(cfg.HAToken)
```

Fix to load from config.yml:
```go
runtimeCfg, err := config.LoadRuntimeConfig("./config.yml")
if err != nil {
    // return error
}
err = h.haClient.Reconnect(runtimeCfg.HomeAssistant.Token)
```

**Alternative approach** (better): Pass config path to StatusHandler constructor:
```go
type StatusHandler struct {
    haClient   HAStatusInterface
    configPath string  // Add this
}

func NewStatusHandler(haClient HAStatusInterface, configPath string) *StatusHandler {
    return &StatusHandler{
        haClient:   haClient,
        configPath: configPath,
    }
}

func (h *StatusHandler) HAReconnect(w http.ResponseWriter, r *http.Request) {
    cfg, err := config.LoadRuntimeConfig(h.configPath)
    // ... use cfg.HomeAssistant.Token
}
```

**Step 3: Update config tests**

Remove tests for legacy fields in `config_test.go`:
- Remove PlexURL assertions
- Remove PlexServerID assertions
- Remove HAURL assertions
- Remove HAToken assertions
- Remove AppleTVEntity assertions

Keep tests for:
- Port (with default)
- DatabasePath (with default)
- LogLevel (with default)
- SessionSecret (required)
- DevMode (boolean)

**Step 4: Update main.go error handling**

Current (lines 42-52):
```go
cfg, err := config.Load()
if err != nil {
    log.Printf("Warning: Failed to load env config: %v", err)
    // Use defaults...
}
```

After removing Plex/HA validation, `Load()` will only fail for missing SESSION_SECRET, so:
```go
cfg, err := config.Load()
if err != nil {
    log.Fatalf("Failed to load configuration: %v", err)
}
```

**Step 5: Verify .env.example**

Check it doesn't have legacy vars (looks already done):
```bash
# Should NOT contain:
# PLEX_URL
# PLEX_SERVER_ID
# HA_URL
# HA_TOKEN
# APPLE_TV_ENTITY
```

**Step 6: Update documentation**

Update CLAUDE.md section on configuration to clarify the split:
- Environment variables = app runtime settings
- config.yml = user data (created by setup wizard)

### Impact:
- **Risk**: Low - only one place uses legacy HAToken incorrectly
- **Breaking changes**: **YES** - if anyone set Plex/HA in .env (unlikely since setup wizard exists)
- **Testing needed**: HA reconnect flow, config loading, setup wizard
- **Lines removed**: ~20 lines of dead code
- **Clarity gained**: High - one clear way to configure

### Migration notes:

If any users have .env with Plex/HA vars (unlikely):
- Setup wizard will prompt them to reconfigure
- SetupMiddleware will redirect to /setup
- No data loss, just need to re-run setup

---

## Issue #4: WebSocket Security - Allow All Origins (CSRF Vulnerability)

### Current Problem:

**Location**: `internal/handlers/pairing.go:79-82`

```go
upgrader: websocket.Upgrader{
    CheckOrigin: func(_ *http.Request) bool {
        return true // Allow all origins for now (same-origin in production)
    },
},
```

**Why this is dangerous:**
- Allows WebSocket connections from ANY domain
- CSRF attack vector: Malicious site can connect to `ws://localhost:3001/ws/pairing`
- Can create card mappings, trigger NFC pairing
- Comment says "for now" but will ship to production
- WebSocket endpoint is authenticated (RequireAuth middleware) but that doesn't prevent CSRF

**Attack scenario:**
1. User visits malicious-site.com while logged into TapeDeck
2. Malicious site opens WebSocket to `ws://victim-tapedeck:3001/ws/pairing`
3. Browser sends session cookie automatically (authenticated!)
4. Malicious site can create mappings, intercept NFC events

### Files to modify:
1. `internal/handlers/pairing.go` - Fix CheckOrigin

### Detailed steps:

**Step 1: Implement proper origin checking**

**Option A: Same-origin policy** (strictest, recommended for local-only app):
```go
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        // Allow requests without Origin header (direct WS connections)
        return true
    }

    // Parse origin and host
    originURL, err := url.Parse(origin)
    if err != nil {
        return false
    }

    // Compare with request host
    return originURL.Host == r.Host
},
```

**Option B: Configurable allowed origins** (more flexible):
```go
type PairingHandler struct {
    // ...existing fields...
    allowedOrigins map[string]bool  // Add this
}

// In NewPairingHandler:
allowedOrigins := map[string]bool{
    "http://localhost:3001":  true,
    "http://localhost:3002":  true,  // Air proxy
    "http://127.0.0.1:3001":  true,
}

upgrader: websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        if origin == "" {
            return true  // Allow connections without Origin
        }
        return h.allowedOrigins[origin]
    },
},
```

**Step 2: Add origin to allowed list from config** (for remote access):

If user accesses TapeDeck from `http://tapedeck.local:3001`, allow that:

```go
// In NewPairingHandler, accept serverURL param:
func NewPairingHandler(..., serverURL string) *PairingHandler {
    allowedOrigins := map[string]bool{
        serverURL: true,
    }
    // ...
}
```

**Step 3: Handle development mode**

For local development with Air proxy on different ports:

```go
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true
    }

    // In dev mode, allow localhost on any port
    if h.devMode {
        originURL, err := url.Parse(origin)
        if err != nil {
            return false
        }
        host, _, _ := net.SplitHostPort(originURL.Host)
        if host == "localhost" || host == "127.0.0.1" {
            return true
        }
    }

    // Production: strict same-origin
    return origin == "http://"+r.Host || origin == "https://"+r.Host
},
```

**Step 4: Add devMode flag to PairingHandler**

Pass devMode from main.go to NewPairingHandler (already has it):
```go
// Already in main.go:
pairingHandler = handlers.NewPairingHandler(
    sessionStore,
    database,
    haClient,
    haRest,
    defaultAppleTV,
    plexServerID,
    "./config.yml",
)

// Need to add:
pairingHandler = handlers.NewPairingHandler(
    sessionStore,
    database,
    haClient,
    haRest,
    defaultAppleTV,
    plexServerID,
    "./config.yml",
    cfg.DevMode,  // Add this
)
```

**Step 5: Test**

- Test WebSocket connection from same origin works
- Test connection from different origin fails (use browser devtools)
- Test Air proxy still works in dev mode (localhost:3002 → localhost:3001)
- Test that removing Origin header still works (for native clients)

### Recommended Implementation:

**Simple and secure for local-only app:**

```go
type PairingHandler struct {
    // ...existing fields...
    devMode bool
}

func NewPairingHandler(..., devMode bool) *PairingHandler {
    handler := &PairingHandler{
        // ...existing init...
        devMode: devMode,
        upgrader: websocket.Upgrader{
            CheckOrigin: checkWebSocketOrigin,
        },
    }
    return handler
}

func (h *PairingHandler) checkWebSocketOrigin(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        // Allow non-browser WebSocket clients
        return true
    }

    originURL, err := url.Parse(origin)
    if err != nil {
        log.Printf("Invalid Origin header: %v", err)
        return false
    }

    // Development: Allow localhost on any port
    if h.devMode {
        hostname, _, _ := net.SplitHostPort(originURL.Host)
        if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "" {
            return true
        }
    }

    // Production: Strict same-origin
    expectedOrigin := "http://" + r.Host
    if r.TLS != nil {
        expectedOrigin = "https://" + r.Host
    }

    allowed := origin == expectedOrigin
    if !allowed {
        log.Printf("WebSocket origin check failed: origin=%s expected=%s", origin, expectedOrigin)
    }
    return allowed
}
```

### Impact:
- **Risk**: Medium - requires careful testing of dev/prod scenarios
- **Breaking changes**: None if implemented correctly
- **Testing needed**: WebSocket pairing in dev and prod modes
- **Security improvement**: High - prevents CSRF attacks

### Alternative: CSRF Tokens

Instead of origin checking, use CSRF tokens:
- Generate token on page load
- Include in WebSocket connection (query param or first message)
- Validate token before accepting connection

This is more complex but works if origin checking has issues.

---

## Issue #5: Race Condition in HA Reconnect

(Planning in progress...)
