# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TapeDeck is a Go web application that bridges physical and digital media using NFC cards. Tap NFC cards to play movies/TV shows on Apple TV via Plex and Home Assistant. Features Plex OAuth, multi-server support, real-time NFC pairing with WebSocket, and web-based setup wizard.

**Tech Stack**: Go 1.25+, Templ templates, Vanilla JavaScript + WebSocket, SQLite (pure Go, no CGO).

## Development Commands

### Setup
```bash
# Install dependencies
go mod download

# Install development tools
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/air-verse/air@latest

# Optional: Create .env to customize settings (all have defaults)
# cp .env.example .env
# SESSION_SECRET is auto-generated and stored in .session_secret on first run
```

### Running the Application
```bash
# Development server with hot reload (watches .go and .templ files)
air

# Access the app
# http://localhost:3001 - Direct app server
# http://localhost:3002 - Air proxy with auto-refresh

# Run without hot reload
go run .

# Build binary
go build -o tapedeck .
```

### Code Quality
```bash
# IMPORTANT: Run checks.sh before every commit
# This runs go fmt, golangci-lint, and tests
./checks.sh

# Individual commands (checks.sh runs all of these):
go fmt ./...                        # Format code
golangci-lint run --timeout=5m     # Lint
go test -v -race ./...             # Test with race detector

# Generate templ files (Air does this automatically)
templ generate
```

### Testing
```bash
# Run all tests
go test ./...

# Run with verbose output and race detector
go test -v -race ./...

# Run specific package
go test ./internal/plex

# Run with coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run specific test
go test -v -run TestHAClient_Connect ./internal/ha
```

### Docker
```bash
# Build and run with docker-compose
docker-compose build
docker-compose up -d

# View logs
docker-compose logs -f tapedeck
```

## Architecture

### Entry Point and Request Flow
- `main.go` (482 lines): HTTP server setup, graceful shutdown, middleware chain, route registration
- Request flow: MetricsMiddleware → RequestLogger → SetupMiddleware (redirects to /setup if config.yml missing) → route handlers
- Handlers are nil at startup if config.yml is missing; initialized after setup wizard completes via `initializeHandlers()` callback

### Configuration System
- Environment variables (`.env`): Optional overrides for basic settings (PORT, DATABASE_PATH, LOG_LEVEL, DEV_MODE). All have sensible defaults.
- Session secret (`.session_secret`): Auto-generated on first run, persists across restarts (gitignored, 0600 permissions)
- Runtime config (`config.yml`): Plex servers, Home Assistant URL/token, Apple TVs (created by setup wizard)
- Setup wizard creates config.yml and triggers handler initialization
- `internal/config/`: Loads both env-based config and runtime config from config.yml

### Core Components

**Plex Integration** (`internal/plex/`):
- `auth.go`: OAuth flow for Plex.tv (PIN-based authentication)
- `servers.go`: Server discovery via Plex.tv API
- `client.go`: Library browsing, search, media metadata retrieval
- Multi-server support: Each Plex server has multiple connection URLs; handlers iterate through servers

**Home Assistant Integration** (`internal/ha/`):
- `websocket.go`: WebSocket client for tag_scanned events (listens for NFC taps)
- `rest.go`: REST API client for controlling Apple TV playback
- Connection lifecycle: Connect → Auth → Subscribe to events → handleMessages loop
- Reconnect logic handles token updates from settings page

**HTTP Handlers** (`internal/handlers/`):
- `setup.go`: Multi-step wizard (Welcome → Plex → HA → Apple TVs → Complete)
- `auth.go`: Plex OAuth login/logout, session management
- `media.go`: Browse libraries, view library contents, search across servers
- `mappings.go`: CRUD for card-to-media mappings with inline search
- `pairing.go`: Real-time NFC pairing via WebSocket (bidirectional: UI ↔ server ↔ HA)
- `playback.go`: Handles playback requests triggered by NFC taps
- `settings.go`: Update servers/HA config and reload handlers without restart

**Database** (`internal/db/`, `internal/models/`):
- SQLite with pure Go driver (no CGO)
- Migrations in `migrations/*.sql` (up/down pairs)
- Models: User, CardMapping (card_uid → plex_key, server_id, apple_tv_entity), PlaybackLog
- `db.go`: Connection management, migration runner

**Templates** (`templates/`):
- Templ templates (type-safe Go templates)
- Air automatically runs `templ generate` on .templ file changes
- Generated `*_templ.go` files are gitignored
- Structure: `layouts/` (base), `pages/` (full pages), `components/` (reusable)

### Key Data Flows

**Setup Wizard Flow**:
1. User visits /setup, redirected if config.yml missing
2. Step 2: Plex OAuth → Fetch servers from Plex.tv → User selects servers → Saved to config.yml
3. Step 3: HA URL/token input → Test connection → Fetch media players → User selects Apple TVs → Saved to config.yml
4. Step 5: Complete → Calls initializeHandlers() → Initializes all handlers with new config

**NFC Pairing Flow**:
1. User opens /mappings/pair (WebSocket connection established)
2. Server subscribes to HA tag_scanned events
3. User taps NFC card on reader → HA emits event → WebSocket → Server
4. Server sends tag UID to browser via WebSocket
5. Browser auto-fills card UID in form, user selects media and Apple TV
6. Form submission creates CardMapping record

**Playback Flow**:
1. NFC card tapped → HA → WebSocket → Server receives tag_scanned event
2. Server looks up CardMapping by card_uid
3. Server fetches media metadata from Plex
4. For TV shows: Find next unwatched episode
5. Server calls HA REST API to play media on specified Apple TV entity
6. Server logs playback to PlaybackLog table

## Development Workflow

### Before Committing Code Changes

**ALWAYS run `./checks.sh` after making code changes and before telling the user you're done.** This script runs:
1. `go fmt ./...` - Format all Go files
2. `golangci-lint run --timeout=5m` - Lint for code quality issues
3. `go test -v -race ./...` - Run all tests with race detector

This ensures all code changes pass the same checks that run in CI, preventing build failures.

## Development Notes

### Templ Integration
- `.templ` files are Go code (not HTML templates)
- Must run `templ generate` to create `*_templ.go` files before building
- Air automatically runs this in the build command (.air.toml)
- Never edit `*_templ.go` files directly
- Common CSS in `static/css/main.css`
- Static files served via `/static/` route

### Session Management
- Uses gorilla/sessions with securecookie
- Session secret auto-generated (32 bytes, cryptographically secure) and stored in .session_secret on first run
- Session store initialized with secret from .session_secret file (0600 permissions, gitignored)
- RequireAuth middleware checks for plex_user_id in session
- Sessions survive server restart (cookies encrypted with persistent secret)

### Multi-Server Architecture
- Handlers receive `[]ServerInfo` with multiple Plex servers and connection URLs
- Each server may have local (192.168.x.x) and remote (.plex.direct) URLs
- Client tries all URLs per server until one succeeds
- Shared servers (Owner="Shared") are skipped due to 401 Unauthorized issues with direct URLs

### WebSocket Patterns
- HA integration: Server is WebSocket client to Home Assistant
- Pairing UI: Server is WebSocket server, browser is client
- Both use gorilla/websocket

### Testing Patterns
- Unit tests next to implementation files (`*_test.go`)
- Mock HTTP responses using httptest for Plex/HA API calls
- Use in-memory SQLite (`:memory:`) for database tests
- Race detector enabled in CI (`-race` flag)

### CI/CD Pipeline
GitHub Actions workflow (.github/workflows/ci.yml):
1. Lint: go fmt check → golangci-lint
2. Test: go test with race detector and coverage
3. Build: Multi-platform Docker image (linux/amd64, linux/arm64) → Push to ghcr.io → Trivy security scan

### Known Limitations
- Shared Plex servers return 401 Unauthorized (see internal/plex/client.go:143)
- Docker internal IPs (172.17.0.x) are filtered out from server connections (see main.go:142)

## Common Tasks

### Adding a New Route
1. Add handler method in `internal/handlers/`
2. Register route in `main.go` (with appropriate middleware)
3. If route needs initialized handlers, wrap in nil check and redirect to /setup
4. Add tests in `internal/handlers/*_test.go`

### Adding a Database Migration
1. Create pair of files: `migrations/NNN_description.up.sql` and `migrations/NNN_description.down.sql`
2. Migrations run automatically on startup via `db.RunMigrations()`
3. Test with: Delete `data/tapedeck.db` → Restart server → Verify schema

### Modifying Configuration
1. Update `internal/config/runtime.go` structs
2. Update setup wizard handlers in `internal/handlers/setup.go`
3. Update settings handlers in `internal/handlers/settings.go`
4. Update `initializeHandlers()` in `main.go` to use new config fields
5. Consider adding migration if config.yml format changed

### Debugging WebSocket Issues
- Check `tapedeck.log` for connection/auth errors
- HA WebSocket logs show message types: auth_required, auth_ok, event, result
- Browser DevTools → Network → WS shows pairing WebSocket messages
- Use DEV_MODE=true to skip TLS verification for local HA instances
