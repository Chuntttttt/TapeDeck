# Quick Wins Refactoring Plan

Small improvements that can be done quickly with high impact.

---

## 1. Remove Debug Log Hack

**Files**: `internal/handlers/auth.go:20-37`

**Current**:
```go
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
```

**Issues**:
- Leaks authentication details to `/tmp/tapedeck-auth-debug.log`
- Hardcoded path
- Not using structured logging
- Sync on every log call is slow

**Solution**: Delete it entirely, replace with `logger.Debug()`.

**Changes**:
```diff
- logDebug("DEBUG Login: Reusing existing PIN ID=%d, Code='%s'", pin.ID, pin.Code)
+ logger.Debug("Reusing existing PIN", "pin_id", pin.ID, "pin_code", pin.Code)

- logDebug("DEBUG PollStatus: Received poll request from %s", r.RemoteAddr)
+ logger.Debug("Poll status request received", "remote_addr", r.RemoteAddr)
```

**Timeline**: 15 minutes

---

## 2. Fix Silent Error Handling

**Current Pattern** (appears ~10 times):
```go
_, _ = debugLog.WriteString(...) // handlers/auth.go:34
_ = logFile.Close()              // main.go:61
_ = conn.Close()                 // multiple files
```

**Solution**: Log errors at minimum.

```go
// Before
_ = logFile.Close()

// After
if err := logFile.Close(); err != nil {
    logger.Warn("Failed to close log file", "error", err)
}
```

**For defers where we can't handle errors**:
```go
defer func() {
    if err := logFile.Close(); err != nil {
        logger.Warn("Failed to close log file", "error", err)
    }
}()
```

**Files to Update**:
- `main.go:61, 79, 206`
- `internal/ha/websocket.go:61, 67, 82, 99, 117, 122, 149, 257, 282`
- `internal/plex/client.go:98, 128, 166`
- `internal/handlers/pairing.go:169, 218, 381`
- `internal/handlers/auth.go:34`

**Timeline**: 30 minutes (grep and fix all instances)

---

## 3. Magic Numbers to Constants

**Current**:
```go
send: make(chan []byte, 256)              // pairing.go:148
time.Sleep(5 * time.Second)               // pairing.go:351
MaxAge: 86400 * 30                        // session.go:22
ReadHeaderTimeout: 10 * time.Second       // main.go:386
upgrader.HandshakeTimeout: 10 * time.Second // ha/websocket.go:51
```

**Solution**: Extract to constants with documentation.

Create `internal/constants/constants.go`:
```go
package constants

import "time"

const (
    // WebSocket Configuration
    WebSocketSendBufferSize = 256
    WebSocketPingInterval   = 54 * time.Second
    WebSocketWriteTimeout   = 10 * time.Second
    WebSocketReadTimeout    = 60 * time.Second
    WebSocketHandshakeTimeout = 10 * time.Second

    // Playback Configuration
    AppleTVWakeTime = 5 * time.Second

    // Session Configuration
    SessionMaxAge = 7 * 24 * time.Hour // 7 days

    // HTTP Server Configuration
    ServerReadHeaderTimeout = 10 * time.Second
    ServerShutdownTimeout   = 10 * time.Second

    // HTTP Client Configuration
    HTTPClientTimeout = 30 * time.Second
)
```

**Usage**:
```go
// Before
send: make(chan []byte, 256)

// After
import "github.com/Chuntttttt/tapedeck/internal/constants"

send: make(chan []byte, constants.WebSocketSendBufferSize)
```

**Timeline**: 1 hour

---

## 4. Standardize Error Responses

**Problem**: Inconsistent error responses:
- `/api/search` returns JSON errors
- `/mappings` returns HTML errors
- Some use `http.Error()`, some render templates

**Solution**: Create error helpers that detect content type.

```go
// internal/handlers/errors.go
package handlers

import (
    "encoding/json"
    "net/http"
    "strings"

    "github.com/Chuntttttt/tapedeck/internal/logger"
    "github.com/Chuntttttt/tapedeck/templates/pages"
)

type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
    Code    int    `json:"code"`
}

// RespondWithError sends an error response in the appropriate format
func RespondWithError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
    logger.Warn("Request error",
        "status", statusCode,
        "message", message,
        "path", r.URL.Path,
        "method", r.Method)

    // Check if client wants JSON
    accept := r.Header.Get("Accept")
    isAPI := strings.HasPrefix(r.URL.Path, "/api/")
    isWebSocket := strings.HasPrefix(r.URL.Path, "/ws/")

    if strings.Contains(accept, "application/json") || isAPI || isWebSocket {
        // Return JSON error
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(statusCode)
        json.NewEncoder(w).Encode(ErrorResponse{
            Error:   http.StatusText(statusCode),
            Message: message,
            Code:    statusCode,
        })
        return
    }

    // Return HTML error page
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(statusCode)

    errorData := pages.ErrorPageData{
        StatusCode: statusCode,
        Title:      http.StatusText(statusCode),
        Message:    message,
        RequestID:  middleware.GetRequestID(r.Context()),
    }

    if err := pages.ErrorPage(errorData).Render(r.Context(), w); err != nil {
        // Fallback to plain text if template fails
        http.Error(w, message, statusCode)
    }
}

// Convenience wrappers
func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
    RespondWithError(w, r, http.StatusBadRequest, message)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, message string) {
    RespondWithError(w, r, http.StatusUnauthorized, message)
}

func NotFound(w http.ResponseWriter, r *http.Request, message string) {
    RespondWithError(w, r, http.StatusNotFound, message)
}

func InternalError(w http.ResponseWriter, r *http.Request, message string) {
    RespondWithError(w, r, http.StatusInternalServerError, message)
}

func ServiceUnavailable(w http.ResponseWriter, r *http.Request, message string) {
    RespondWithError(w, r, http.StatusServiceUnavailable, message)
}
```

**Create error template** `templates/pages/error.templ`:
```go
package pages

type ErrorPageData struct {
    StatusCode int
    Title      string
    Message    string
    RequestID  string
}

templ ErrorPage(data ErrorPageData) {
    @layouts.Base("Error - TapeDeck") {
        <div class="error-container">
            <h1>{fmt.Sprintf("%d", data.StatusCode)}</h1>
            <h2>{data.Title}</h2>
            <p>{data.Message}</p>
            <p class="request-id">Request ID: {data.RequestID}</p>
            <a href="/" class="btn">Go Home</a>
        </div>
    }
}
```

**Usage**:
```go
// Before
http.Error(w, "Invalid tag ID", http.StatusBadRequest)

// After
handlers.BadRequest(w, r, "Invalid tag ID")
```

**Timeline**: 2 hours (create helpers + template + update all handlers)

---

## 5. Add Request Timeouts to HTTP Clients

**Current**:
```go
// plex/client.go:86
req, err := http.NewRequest(http.MethodGet, c.serverURL+"/library/sections", nil)
```

**Problem**: No per-request timeout. If Plex hangs, request waits forever.

**Solution**: Use `http.NewRequestWithContext()`.

**Prerequisite**: Context propagation must be implemented first (see `guide-context-propagation.md`).

```go
// Before
func (c *Client) GetLibraries() ([]Library, error) {
    req, err := http.NewRequest(http.MethodGet, c.serverURL+"/library/sections", nil)
    // ...
}

// After
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/library/sections", nil)
    // ...
}
```

**Handler adds timeout**:
```go
// handlers/media.go
func (h *MediaHandler) Libraries(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    libraries, err := h.plexClient.GetLibraries(ctx)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            handlers.ServiceUnavailable(w, r, "Plex server timed out")
            return
        }
        // ...
    }
    // ...
}
```

**Files to Update**:
- `internal/plex/client.go` - All methods
- `internal/plex/servers.go` - All methods
- `internal/ha/rest.go` - All methods

**Timeline**: 1 hour (after context propagation is done)

---

## 6. Fix GetCardMappingByTagID Logic

**File**: `internal/db/db.go:308-334`

**Current**:
```go
// If multiple users have the same tag_id, returns the most recently created mapping
func (db *DB) GetCardMappingByTagID(tagID string) (*models.CardMapping, error) {
    // ...
    `SELECT ... FROM card_mappings WHERE tag_id = ? ORDER BY created_at DESC LIMIT 1`
    // ...
}
```

**Issue**: Allows tag ID collision across users (though app only supports single user).

**Your Question**: "This app should only ever allow one user. Does that simplify things?"

**Answer**: YES! If single-user app, enforce at database level.

**Solution 1** (If truly single-user forever):

Add migration to enforce single user:
```sql
-- migrations/006_single_user_constraint.up.sql
-- Ensure only one user can exist
CREATE TRIGGER prevent_multiple_users
BEFORE INSERT ON users
WHEN (SELECT COUNT(*) FROM users) >= 1
BEGIN
    SELECT RAISE(FAIL, 'Only one user allowed');
END;
```

Then `GetCardMappingByTagID` is safe as-is (only one user exists).

**Solution 2** (If multi-user might come later):

Make tag_id globally unique:
```sql
-- migrations/006_unique_tag_id.up.sql
-- Remove old unique constraint
DROP INDEX IF EXISTS idx_card_mappings_tag_id;

-- Add global unique constraint
CREATE UNIQUE INDEX idx_card_mappings_tag_id_unique ON card_mappings(tag_id);

-- Note: This will fail if duplicate tag_ids exist across users
-- Run data cleanup first if needed
```

Update query:
```go
func (db *DB) GetCardMappingByTagID(tagID string) (*models.CardMapping, error) {
    mapping := &models.CardMapping{}
    err := db.conn.QueryRow(
        `SELECT id, user_id, tag_id, media_type, media_id, media_title, plex_server_id, apple_tv_entity, created_at, updated_at
        FROM card_mappings WHERE tag_id = ?`, // No ORDER BY needed, unique constraint guarantees one result
        tagID,
    ).Scan(...)
    // ...
}
```

**Recommendation**: Use Solution 1 (single-user constraint) since you confirmed single-user app. Simpler and matches your use case.

**Timeline**: 30 minutes (migration + testing)

---

## 7. Structured Logging Migration

**See**: `guide-context-propagation.md` for full plan.

**Quick version**:

1. **Replace all `log.Printf` with `logger.*`**:
```bash
# Find all instances
rg "log\.(Printf|Println|Print)" --type go
```

2. **Convert each one**:
```go
// Before
log.Printf("Failed to create mapping: %v", err)

// After
logger.Error("Failed to create mapping", "error", err)
```

3. **Add context where available**:
```go
// Before
log.Printf("WebSocket client connected (userID=%d)", userID)

// After
log := middleware.GetLogger(r.Context()) // After context propagation implemented
log.Info("WebSocket client connected")    // user_id already in context
```

**Files to migrate** (in order):
1. `internal/handlers/pairing.go` (15 log calls)
2. `internal/handlers/mappings.go` (13 log calls)
3. `internal/handlers/media.go` (8 log calls)
4. `internal/handlers/setup.go` (20 log calls)
5. `internal/handlers/settings.go` (5 log calls)
6. `internal/handlers/auth.go` (7 log calls, remove debug hack)
7. `internal/ha/websocket.go` (4 log calls)
8. `main.go` (12 log calls)

**Timeline**: 3 hours (mechanical replacement, then cleanup)

---

## Priority Order

1. **Remove debug log hack** (15 min) - Security risk
2. **Fix silent errors** (30 min) - Reliability
3. **Magic numbers → constants** (1 hour) - Code quality
4. **GetCardMappingByTagID fix** (30 min) - Data integrity
5. **Standardize error responses** (2 hours) - UX consistency
6. **Add request timeouts** (1 hour) - Requires context propagation first
7. **Structured logging migration** (3 hours) - After context propagation

**Total**: ~8.5 hours

Can be done over a few evenings without blocking other work.
