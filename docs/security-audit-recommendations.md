# Security Audit & Recommendations

**Last Updated**: 2025-01-14

This document outlines security issues found in the TapeDeck codebase and provides actionable remediation steps.

## Critical Issues (Fix Before Production)

### 🔴 CRIT-1: Session Cookies Not Secure

**File**: `internal/middleware/session.go:24`

**Issue**:
```go
Secure:   false, // Set to true in production with HTTPS
```

**Risk**: Session hijacking via man-in-the-middle (MITM) attacks. Attacker can intercept session cookie over HTTP and impersonate user.

**Recommendation**:

For self-hosted apps like TapeDeck (similar to Sonarr, Radarr, Overseerr), support both HTTP and HTTPS:

```go
// config/config.go - Add to Config struct
type Config struct {
    // ... existing fields ...
    RequireTLS bool   `env:"REQUIRE_TLS" envDefault:"false"`
}

// middleware/session.go
func NewSessionStore(secret []byte, requireTLS bool) *sessions.CookieStore {
    store := sessions.NewCookieStore(secret)
    store.Options = &sessions.Options{
        Path:     "/",
        MaxAge:   86400 * 7, // Reduced to 7 days (was 30)
        HttpOnly: true,
        Secure:   requireTLS, // Configurable
        SameSite: http.SameSiteLaxMode,
    }
    return store
}
```

Add clear warning on startup:
```go
// main.go
if !cfg.RequireTLS {
    logger.Warn("⚠️  TLS NOT REQUIRED - Session cookies will be sent over HTTP")
    logger.Warn("⚠️  This is INSECURE for internet-exposed deployments")
    logger.Warn("⚠️  Set REQUIRE_TLS=true and use HTTPS in production")
}
```

**Timeline**: 30 minutes

---

### 🔴 CRIT-2: WebSocket Allows All Origins

**File**: `internal/handlers/pairing.go:79-81`

**Issue**:
```go
CheckOrigin: func(_ *http.Request) bool {
    return true // Allow all origins for now (same-origin in production)
}
```

**Risk**: Cross-Site WebSocket Hijacking (CSWSH). Malicious website can:
1. Open WebSocket to `ws://tapedeck.local/ws/pairing`
2. Receive tag_scanned events when user taps NFC cards
3. Exfiltrate user's card UIDs and media preferences

**Recommendation**:

For self-hosted apps, localhost/LAN origins are typically safe:

```go
// middleware/websocket.go
func CheckWebSocketOrigin(allowedOrigins []string, devMode bool) func(*http.Request) bool {
    return func(r *http.Request) bool {
        origin := r.Header.Get("Origin")

        // No origin header = same-origin request (safe)
        if origin == "" {
            return true
        }

        // Dev mode: allow localhost
        if devMode {
            if strings.HasPrefix(origin, "http://localhost") ||
               strings.HasPrefix(origin, "http://127.0.0.1") {
                return true
            }
        }

        // Check allowed origins
        for _, allowed := range allowedOrigins {
            if origin == allowed {
                return true
            }
        }

        logger.Warn("WebSocket origin rejected",
            "origin", origin,
            "allowed_origins", allowedOrigins)
        return false
    }
}
```

**Config**:
```yaml
# config.yml
version: 1
allowed_origins:
  - "http://tapedeck.local"
  - "https://tapedeck.local"
```

**Usage**:
```go
// handlers/pairing.go
func NewPairingHandler(..., allowedOrigins []string, devMode bool) *PairingHandler {
    handler := &PairingHandler{
        // ...
        upgrader: websocket.Upgrader{
            CheckOrigin: middleware.CheckWebSocketOrigin(allowedOrigins, devMode),
        },
    }
    // ...
}
```

**For self-hosted apps**: Document that users should add their LAN URL to `allowed_origins` in config.yml.

**Timeline**: 1 hour

---

### 🔴 CRIT-3: Default Session Secret

**File**: `main.go:49`

**Issue**:
```go
SessionSecret: "change-me-in-production",
```

**Risk**: If default secret is used, attacker can forge session cookies for any user.

**Recommendation**:

Fail fast if default secret is detected:

```go
// config/config.go
func (c *Config) Validate() error {
    if c.SessionSecret == "change-me-in-production" ||
       c.SessionSecret == "" ||
       len(c.SessionSecret) < 32 {
        return fmt.Errorf("SESSION_SECRET must be set to a random 32+ character string (generate with: openssl rand -hex 32)")
    }
    return nil
}

// main.go
cfg, err := config.Load()
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

if err := cfg.Validate(); err != nil {
    log.Fatalf("Configuration invalid: %v", err)
}
```

**Docker/Setup Wizard**: Auto-generate secret on first run:
```go
// setup/wizard.go
func generateSessionSecret() string {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil {
        panic(err)
    }
    return hex.EncodeToString(bytes)
}
```

**Timeline**: 30 minutes

---

### 🔴 CRIT-4: No CSRF Protection

**Files**: All POST handlers

**Risk**: Attacker can craft malicious page that submits forms to TapeDeck:
```html
<form action="http://tapedeck.local/mappings" method="POST">
  <input name="tag_id" value="attacker-tag">
  <input name="media_id" value="malicious-content">
</form>
<script>document.forms[0].submit()</script>
```

**Recommendation**:

Use `gorilla/csrf` middleware:

```bash
go get github.com/gorilla/csrf
```

```go
// main.go
import "github.com/gorilla/csrf"

func main() {
    // ... setup ...

    // Generate CSRF key (store in config alongside session secret)
    csrfKey := []byte(cfg.CSRFSecret) // 32-byte key

    csrfMiddleware := csrf.Protect(
        csrfKey,
        csrf.Secure(cfg.RequireTLS),
        csrf.Path("/"),
        csrf.SameSite(csrf.SameSiteLaxMode),
    )

    handler := csrfMiddleware(
        middleware.WithRequestID()(
            // ... rest of middleware chain
        ),
    )

    // ...
}
```

**Templates**: Add CSRF token to forms:
```html
<form method="POST" action="/mappings">
    {{ .CSRFField }}
    <!-- form fields -->
</form>
```

**Exemptions**: WebSocket and API endpoints don't need CSRF (they don't use cookies).

**Timeline**: 1 hour

---

### 🔴 CRIT-5: No Rate Limiting

**Risk**: Brute force attacks on `/auth/poll-status`, DoS on all endpoints.

**Recommendation**:

Use `golang.org/x/time/rate` for simple rate limiting:

```go
// middleware/ratelimit.go
package middleware

import (
    "net/http"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

type visitor struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

type RateLimiter struct {
    visitors map[string]*visitor
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

func NewRateLimiter(requestsPerSecond int, burst int) *RateLimiter {
    rl := &RateLimiter{
        visitors: make(map[string]*visitor),
        rate:     rate.Limit(requestsPerSecond),
        burst:    burst,
    }

    // Cleanup old visitors every minute
    go rl.cleanupVisitors()

    return rl
}

func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    v, exists := rl.visitors[ip]
    if !exists {
        limiter := rate.NewLimiter(rl.rate, rl.burst)
        rl.visitors[ip] = &visitor{limiter, time.Now()}
        return limiter
    }

    v.lastSeen = time.Now()
    return v.limiter
}

func (rl *RateLimiter) cleanupVisitors() {
    for {
        time.Sleep(1 * time.Minute)

        rl.mu.Lock()
        for ip, v := range rl.visitors {
            if time.Since(v.lastSeen) > 3*time.Minute {
                delete(rl.visitors, ip)
            }
        }
        rl.mu.Unlock()
    }
}

func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := getIP(r)
            limiter := rl.getVisitor(ip)

            if !limiter.Allow() {
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

func getIP(r *http.Request) string {
    // Check X-Forwarded-For for proxies
    forwarded := r.Header.Get("X-Forwarded-For")
    if forwarded != "" {
        return strings.Split(forwarded, ",")[0]
    }

    // Check X-Real-IP
    realIP := r.Header.Get("X-Real-IP")
    if realIP != "" {
        return realIP
    }

    // Fall back to RemoteAddr
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    return ip
}
```

**Usage**:
```go
// main.go
globalRateLimit := middleware.NewRateLimiter(10, 20) // 10 req/s, burst 20
authRateLimit := middleware.NewRateLimiter(1, 5)     // 1 req/s for auth

r := chi.NewRouter()
r.Use(globalRateLimit.Middleware())

r.Route("/auth", func(r chi.Router) {
    r.Use(authRateLimit.Middleware())
    r.Get("/login", authHandler.Login)
    r.Get("/poll-status", authHandler.PollStatus)
})
```

**Timeline**: 2 hours

---

## High Priority Issues

### 🟠 HIGH-1: Tokens Stored Unencrypted

**Files**:
- Database: `users.plex_auth_token`
- Config: `config.yml` (HA token)

**Risk**: If database or config file is compromised, attacker gains full access to user's Plex account and Home Assistant.

**Recommendation**:

Implement envelope encryption:

```go
// crypto/envelope.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "io"
)

// Encrypt encrypts plaintext using AES-GCM with the given key
func Encrypt(plaintext string, key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-GCM with the given key
func Decrypt(ciphertext string, key []byte) (string, error) {
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return "", err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    if len(data) < gcm.NonceSize() {
        return "", errors.New("ciphertext too short")
    }

    nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }

    return string(plaintext), nil
}
```

**Usage**:
```go
// db/db.go
func (db *DB) CreateUser(user *models.User) (int64, error) {
    // Encrypt token before storage
    encryptedToken, err := crypto.Encrypt(user.PlexAuthToken, db.encryptionKey)
    if err != nil {
        return 0, fmt.Errorf("failed to encrypt token: %w", err)
    }

    result, err := db.conn.Exec(
        `INSERT INTO users (plex_username, plex_user_id, plex_auth_token, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?)`,
        user.PlexUsername,
        user.PlexUserID,
        encryptedToken, // Store encrypted
        user.CreatedAt,
        user.UpdatedAt,
    )
    // ...
}

func (db *DB) GetUserByID(id int64) (*models.User, error) {
    // ... query ...

    // Decrypt token after retrieval
    decryptedToken, err := crypto.Decrypt(user.PlexAuthToken, db.encryptionKey)
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt token: %w", err)
    }
    user.PlexAuthToken = decryptedToken

    return user, nil
}
```

**Key Management**:
```bash
# Generate encryption key (do once, store in environment)
openssl rand -hex 32 > /etc/tapedeck/encryption.key
chmod 600 /etc/tapedeck/encryption.key
```

```go
// config/config.go
type Config struct {
    // ...
    EncryptionKeyPath string `env:"ENCRYPTION_KEY_PATH" envDefault:"/etc/tapedeck/encryption.key"`
}
```

**Note**: This is defense-in-depth. If attacker has filesystem access, they likely have the key too. But it prevents casual database snooping.

**Timeline**: 4 hours (includes migration for existing data)

---

### 🟠 HIGH-2: Long Session Duration

**File**: `middleware/session.go:22`

**Issue**: 30-day sessions increase window for session theft.

**Recommendation**: Reduce to 7 days, implement "remember me" separately.

```go
MaxAge: 86400 * 7, // 7 days
```

**Timeline**: 5 minutes

---

### 🟠 HIGH-3: Unauthenticated Metrics Endpoint

**File**: `main.go:251`

**Issue**: `/metrics` exposes internal state without authentication.

**Recommendation**:

Add basic auth or IP whitelist:

```go
// middleware/metrics_auth.go
func MetricsAuth(username, password string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user, pass, ok := r.BasicAuth()
            if !ok || user != username || pass != password {
                w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

```go
// main.go
if cfg.MetricsUsername != "" {
    metricsAuth := middleware.MetricsAuth(cfg.MetricsUsername, cfg.MetricsPassword)
    r.With(metricsAuth).Handle("/metrics", promhttp.Handler())
} else {
    logger.Warn("⚠️  /metrics endpoint is unprotected")
    r.Handle("/metrics", promhttp.Handler())
}
```

**Timeline**: 30 minutes

---

### 🟠 HIGH-4: No Input Validation

**Files**: Handlers throughout

**Recommendation**:

Create validation helpers:

```go
// validation/validators.go
package validation

import (
    "errors"
    "regexp"
)

var (
    tagIDRegex    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,255}$`)
    mediaIDRegex  = regexp.MustCompile(`^[0-9]{1,20}$`)
    entityIDRegex = regexp.MustCompile(`^[a-z_]+\.[a-z0-9_]+$`)
)

func ValidateTagID(id string) error {
    if !tagIDRegex.MatchString(id) {
        return errors.New("invalid tag_id format (must be alphanumeric, dash, underscore, 1-255 chars)")
    }
    return nil
}

func ValidateMediaID(id string) error {
    if !mediaIDRegex.MatchString(id) {
        return errors.New("invalid media_id format (must be numeric, 1-20 digits)")
    }
    return nil
}

func ValidateEntityID(id string) error {
    if !entityIDRegex.MatchString(id) {
        return errors.New("invalid entity_id format (must be domain.object_id)")
    }
    return nil
}
```

**Usage**:
```go
// handlers/mappings.go
func (h *MappingsHandler) CreateMapping(w http.ResponseWriter, r *http.Request) {
    // ... parse form ...

    if err := validation.ValidateTagID(tagID); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := validation.ValidateMediaID(mediaID); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // ... proceed with validated input
}
```

**Timeline**: 2 hours

---

## Medium Priority Issues

### 🟡 MED-1: Session Fixation

**Recommendation**: Rotate session ID after login.

```go
// handlers/auth.go - After successful login
oldSession := session
session.Options.MaxAge = -1
if err := oldSession.Save(r, w); err != nil {
    logger.Error("Failed to invalidate old session", "error", err)
}

// Create new session
session, _ = h.sessionStore.New(r, middleware.SessionName)
middleware.SetUserID(session, user.ID)
if err := session.Save(r, w); err != nil {
    // ...
}
```

**Timeline**: 30 minutes

---

### 🟡 MED-2: DevMode TLS Bypass

**File**: `plex/client.go:72`

**Recommendation**: Add loud startup warning.

```go
if devMode {
    logger.Warn("⚠️⚠️⚠️  DEV_MODE ENABLED  ⚠️⚠️⚠️")
    logger.Warn("TLS certificate verification is DISABLED")
    logger.Warn("This is INSECURE and should NEVER be used in production")
    logger.Warn("Man-in-the-middle attacks are possible")
}
```

**Timeline**: 5 minutes

---

### 🟡 MED-3: User Enumeration

**File**: `handlers/auth.go`

**Recommendation**: Use constant-time responses.

```go
// Make login timing consistent whether user exists or not
time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
```

**Timeline**: 15 minutes

---

### 🟡 MED-4: No Audit Logging

**Recommendation**: Log sensitive operations.

```go
// After creating mapping
logger.Info("Mapping created",
    "user_id", userID,
    "tag_id", tagID,
    "media_id", mediaID,
    "ip", getIP(r),
    "user_agent", r.UserAgent())

// After deleting mapping
logger.Info("Mapping deleted",
    "user_id", userID,
    "mapping_id", mappingID,
    "ip", getIP(r))

// After settings change
logger.Info("Settings updated",
    "user_id", userID,
    "servers_count", len(servers),
    "ip", getIP(r))
```

**Timeline**: 1 hour

---

## Implementation Priority

**Week 1** (Critical):
1. Default session secret validation (30 min)
2. Session cookie Secure flag config (30 min)
3. WebSocket origin checking (1 hour)
4. CSRF protection (1 hour)
5. Rate limiting (2 hours)

**Week 2** (High):
6. Input validation (2 hours)
7. Metrics authentication (30 min)
8. Session duration reduction (5 min)
9. Token encryption (4 hours)

**Week 3** (Medium):
10. Session fixation fix (30 min)
11. Audit logging (1 hour)
12. DevMode warning (5 min)

**Total**: ~13 hours over 3 weeks

## Testing Security Fixes

```bash
# Test CSRF protection
curl -X POST http://localhost:3001/mappings \
  -d "tag_id=test&media_id=123" \
  # Should fail with 403 Forbidden

# Test rate limiting
for i in {1..100}; do
  curl http://localhost:3001/auth/poll-status
done
# Should return 429 after threshold

# Test WebSocket origin
wscat -c ws://localhost:3001/ws/pairing \
  -H "Origin: http://evil.com"
# Should be rejected

# Test metrics auth
curl http://localhost:3001/metrics
# Should return 401 if auth enabled
```

## Security Checklist for Deployment

- [ ] SESSION_SECRET is set to random 32+ character string
- [ ] CSRF_SECRET is set to random 32-byte hex string
- [ ] REQUIRE_TLS=true if exposed to internet
- [ ] Allowed origins configured in config.yml
- [ ] Metrics endpoint has authentication
- [ ] Rate limiting is enabled
- [ ] Encryption key is generated and secured
- [ ] DevMode is disabled (DEV_MODE=false)
- [ ] Firewall rules restrict access (if self-hosted on LAN)
- [ ] Regular backups of database and config
