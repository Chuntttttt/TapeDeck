# TapeDeck

[![CI](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Chuntttttt/TapeDeck)](https://goreportcard.com/report/github.com/Chuntttttt/TapeDeck)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)

A Go-based web application that bridges physical and digital media using NFC cards. Tap NFC cards on a reader to instantly play movies, TV shows, or music on your TV through Plex and Apple TV, managed via Home Assistant.

## Project Status

🚧 **In Development** - Stages 0-7 complete, Stage 8 next

Core functionality complete: authentication, media browsing, card mapping, Home Assistant integration, and real-time NFC pairing via WebSocket.

## Known Issues

### ⚠️ Plex OAuth Workaround (HIGH PRIORITY)

**Current Implementation**: Using PIN polling with 5-second intervals

**Why**: Plex broke OAuth for 3rd party applications in Web update v4.152.0 (September 2025). The recommended `forwardUrl` redirect flow no longer works due to `Cross-Origin-Opener-Policy` changes.

**Forum Thread**: https://forums.plex.tv/t/plex-oauth-authenticate-with-plex-broken-after-plex-web-update-v4-152-0/931098

**TODO**: Switch back to `forwardUrl` redirect flow once Plex fixes their OAuth implementation. This is the proper authentication method for web applications and should be prioritized when available.

**Current Limitations**:
- Slower authentication (5-second polling vs instant redirect)
- Risk of rate limiting with aggressive testing
- Manual retry required if rate limited

**Code Location**: `internal/handlers/auth.go` (Login handler with JavaScript polling)

## Overview

TapeDeck recreates the nostalgic experience of physical media libraries for the streaming age. Kids (and adults!) can tap a physical card to play their favorite content without navigating complex streaming interfaces.

### Key Features (Planned)

- **Visual Media Browser**: Browse Plex libraries with poster art and metadata
- **Real-time NFC Pairing**: Assign media to cards by tapping them during setup
- **Zero YAML Editing**: All mappings managed through web UI
- **Smart TV Show Handling**: Automatically plays next unwatched episode
- **Home Assistant Integration**: Seamless playback on Apple TV

## Technology Stack

- **Language**: Go 1.25+
- **Templating**: Templ (type-safe Go templates)
- **Frontend**: DataStar (SSE-based hypermedia)
- **Database**: SQLite (pure Go driver, no CGO)
- **WebSocket**: Home Assistant integration for NFC events
- **Deployment**: Docker on Synology NAS

## Prerequisites

- Go 1.25 or higher
- Air (hot reload during development)
- Templ CLI (template generation)
- Plex Media Server
- Home Assistant (with ESPHome NFC reader)
- Apple TV with Plex app

## Quick Start

### 1. Install Dependencies

```bash
# Install Go tools
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/air-verse/air@latest
```

### 2. Clone and Setup

```bash
cd TapeDeck
go mod download

# Copy environment template
cp .env.example .env

# Edit .env with your configuration
# - PLEX_URL: Your Plex server URL
# - PLEX_SERVER_ID: Your Plex server identifier
# - HA_URL: Your Home Assistant URL
# - HA_TOKEN: Long-lived access token from HA
```

### 3. Run Development Server

```bash
# Start with hot reload
air

# Server will start on http://localhost:3001
```

### 4. Verify Installation

```bash
# Test health check endpoint
curl http://localhost:3001/health

# Expected response:
# {"status":"ok"}

# Run tests
go test -v ./...
```

## Development Workflow

### Hot Reload

Air automatically watches for changes and rebuilds:

```bash
# Start development server
air

# Make changes to .go or .templ files
# Air will detect changes, rebuild, and restart automatically

# With proxy mode enabled (.air.toml), access via:
# http://localhost:3002 - Proxy with auto-refresh
# http://localhost:3001 - Direct app server
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/config

# Run with coverage
go test -cover ./...
```

### Code Formatting

```bash
# Format Go code
go fmt ./...

# Generate Templ files manually (Air does this automatically)
templ generate
```

## Project Structure

```
tapedeck/
├── main.go                    # Entry point, HTTP server
├── main_test.go              # Tests for main package
├── go.mod                    # Go dependencies
├── .env.example              # Environment variable template
├── .air.toml                 # Hot reload configuration
├── Dockerfile                # Production build
├── docker-compose.yml        # Local Docker setup
│
├── internal/                 # Private application code
│   ├── config/              # Configuration loading
│   ├── db/                  # Database operations
│   ├── models/              # Data models
│   ├── auth/                # Plex OAuth, sessions
│   ├── plex/                # Plex API client
│   ├── ha/                  # Home Assistant WebSocket
│   ├── handlers/            # HTTP handlers
│   ├── middleware/          # Auth, logging middleware
│   └── pairing/             # NFC pairing logic
│
├── templates/               # Templ components
│   ├── layouts/            # Base layouts
│   ├── pages/              # Full pages
│   └── components/         # Reusable components
│
├── static/                  # Static assets
│   ├── css/                # Stylesheets
│   ├── js/                 # JavaScript
│   └── icons/              # Icons and images
│
├── migrations/              # SQL migration files
└── data/                    # SQLite database (gitignored)
```

## Environment Variables

See `.env.example` for all required variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `PLEX_URL` | Plex Media Server URL | `http://192.168.1.100:32400` |
| `PLEX_SERVER_ID` | Plex server identifier | `your_plex_server_id_here` |
| `HA_URL` | Home Assistant URL | `http://192.168.1.101:8123` |
| `HA_TOKEN` | HA long-lived access token | `eyJhbGc...` |
| `APPLE_TV_ENTITY` | HA media player entity | `media_player.apple_tv` |
| `PORT` | HTTP server port | `3001` |
| `DATABASE_PATH` | SQLite database location | `./data/tapedeck.db` |
| `LOG_LEVEL` | Logging level | `info` |
| `SESSION_SECRET` | Session encryption key | (auto-generated) |
| `DEV_MODE` | Skip TLS verification (macOS dev) | `true` (optional) |

## Docker Deployment

### Build Image

```bash
# Build for production
docker build -t tapedeck:latest .

# Build with docker-compose
docker-compose build
```

### Run Container

```bash
# Using docker-compose (recommended)
docker-compose up -d

# Or directly with docker
docker run -d \
  --name tapedeck \
  -p 3001:3001 \
  -v $(pwd)/data:/data \
  --env-file .env \
  tapedeck:latest
```

### View Logs

```bash
docker-compose logs -f tapedeck
```

## Testing Strategy

Following Test-Driven Development (TDD):

1. **Red**: Write failing test describing desired behavior
2. **Green**: Write minimal code to make test pass
3. **Refactor**: Clean up while keeping tests green

### Test Organization

- Unit tests: `*_test.go` next to implementation files
- Table-driven tests for multiple scenarios
- Mock HTTP responses for external APIs (Plex, HA)
- Integration tests in later stages

## Roadmap

- [x] **Stage 0**: Project scaffolding ✅
  - Go module initialized
  - Directory structure created
  - Hot reload configured
  - Basic health check endpoint
  - Docker build setup
  - CI/CD with linting, testing, and security scanning

- [x] **Stage 1**: Database & Configuration ✅
  - SQLite with migrations
  - Config loading from environment
  - User model with Plex authentication
  - Full test coverage with TDD

- [x] **Stage 2**: Plex API Client ✅
  - PIN-based OAuth flow
  - Library browsing
  - Media search
  - Integration tests for real server testing

- [x] **Stage 3**: Basic Web UI & Authentication ✅
  - Plex PIN OAuth with client-side polling
  - Session management (cookie-based)
  - Air proxy mode with auto-refresh
  - DEV_MODE for TLS bypass

- [x] **Stage 4**: Media Browser UI ✅
  - Browse libraries
  - Search media with dedicated page
  - View library contents
  - **Future enhancements:**
    - Display poster art/thumbnails for media items
    - Prettier card layouts with hover effects
    - Responsive grid layouts
    - Media detail modal/page with full metadata

- [x] **Stage 5**: Manual Card Mapping ✅
  - Create mappings with inline search autocomplete
  - Edit/delete mappings
  - Dashboard view with card list

- [x] **Stage 6**: Home Assistant REST Integration ✅
  - POST /api/play endpoint for playback triggers
  - Usage tracking with playback_logs table
  - Cross-user tag lookup

- [x] **Stage 7**: Real-time NFC Pairing ✅
  - WebSocket client connects to Home Assistant
  - WebSocket server for browser clients
  - Live NFC tag detection during pairing
  - Real-time UI feedback with connection status
  - Duplicate tag detection

- [ ] **Stage 8**: Enhanced UI & Polish
  - HA connection status banner (red alert if disconnected)
  - Real-time connection health monitoring
  - Search and filter improvements on mappings dashboard
  - **Deferred to future**: Responsive design (see `docs/responsive-design.md`)
  - **Deferred to future**: Accessibility enhancements (see `docs/accessibility.md`)

- [ ] **Stage 9**: Admin Interface
  - **Settings page** (`/settings`) for managing all configuration
  - **Home Assistant Settings**:
    - Update HA URL and token from UI
    - Auto-discover and select Apple TV entity (dropdown from available media_player entities)
    - Test HA connection and WebSocket
    - Token rotation and validation
  - **Plex Settings**:
    - Update Plex URL and server ID
    - Test Plex connection and authentication
    - Validate server is reachable
  - **Connection Health Dashboard**:
    - Real-time status of HA WebSocket connection
    - Real-time status of Plex server
    - Last successful connection timestamps
    - Error logs for failed connections
  - **Secure Storage**:
    - Evaluate database vs encrypted config file
    - Encrypt sensitive tokens at rest
    - Audit log for configuration changes
  - **User Roles** (optional):
    - Admin users can access settings
    - Regular users can only manage their own mappings
  - See `docs/home-assistant-setup.md` for current manual setup process

- [ ] **Stage 10**: Production Deployment
  - Docker optimization
  - Logging and metrics
  - Backup scripts

- [ ] **Stage 11**: Integration Testing & Documentation
  - End-to-end tests
  - Hardware setup guide
  - User documentation

**Estimated Timeline**: 60-82 hours total (~6-8 weeks at 10-15 hours/week)

## Future Enhancements

The following improvements are documented for future work and may be good candidates for community contributions:

- **Responsive Design**: Mobile and tablet optimization - see [`docs/responsive-design.md`](docs/responsive-design.md)
- **Accessibility**: WCAG compliance, keyboard navigation, screen reader support - see [`docs/accessibility.md`](docs/accessibility.md)

## Contributing

This is a personal project under active development. Once v1 is complete and stable, contributions will be welcome.

## License

MIT

## Acknowledgments

Inspired by the blog post ["How I Built an NFC Movie Library for My Kids"](https://simplyexplained.com/blog/how-i-built-an-nfc-movie-library-for-my-kids/) and similar projects in the home automation community.
