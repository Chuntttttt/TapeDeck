# TapeDeck

[![CI](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Chuntttttt/TapeDeck)](https://goreportcard.com/report/github.com/Chuntttttt/TapeDeck)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)

A Go-based web application that bridges physical and digital media using NFC cards. Tap NFC cards on a reader to instantly play movies, TV shows, or music on your TV through Plex and Apple TV, managed via Home Assistant.

## Project Status

✅ **Feature Complete** - All 11 development stages complete!

TapeDeck is ready for production use. Complete feature set includes: Plex authentication, multi-server media browsing, card mapping with real-time NFC pairing, Home Assistant integration with WebSocket, web-based setup wizard, production deployment with Docker and metrics, and comprehensive hardware documentation.

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

### Shared Plex Servers

**Current Limitation**: Direct connections to shared Plex servers (servers not owned by you) may return 401 Unauthorized errors even with valid authentication tokens. This appears to be a Plex permission limitation.

**Impact**: The application will try multiple connection URLs but may not be able to access content from shared servers. Your own servers will work normally.

**Workaround**: None currently available. This is a low-priority issue as most users primarily use their own servers.

**Code Location**: `internal/plex/client.go` (Search method with TODO comment)

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

### Software
- Go 1.25 or higher
- Air (hot reload during development)
- Templ CLI (template generation)
- Plex Media Server
- Home Assistant with Apple TV integration

### Hardware
- ESPHome-compatible device (ESP32 or ESP8266)
- NFC reader (RC522 or PN532)
- NFC cards/tags (NTAG213/215/216 or MIFARE Classic)
- Apple TV with Plex app

**See the [Hardware Setup Guide](docs/hardware-setup.md) for detailed instructions on wiring and configuring your NFC reader.**

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

# Copy environment template (contains only basic settings)
cp .env.example .env

# Generate a secure session secret
# On Linux/macOS:
openssl rand -hex 32 >> .env
# Or manually edit .env and replace SESSION_SECRET value
```

### 3. Run Development Server

```bash
# Start with hot reload
air

# Server will start on http://localhost:3001
```

### 4. Complete Setup Wizard

On first run, the application will redirect you to the setup wizard at `http://localhost:3001/setup`:

1. **Welcome**: Introduction to the setup process
2. **Plex Authentication**: Log in with your Plex account to discover servers
3. **Server Selection**: Choose which Plex servers to connect to (supports multiple)
4. **Home Assistant**: Enter your HA URL and long-lived access token
5. **Apple TV Selection**: Choose which Apple TVs to use for playback (supports multiple)
6. **Completion**: Review configuration and finish setup

The wizard creates `config.yml` with your settings. You can re-run setup anytime by deleting this file.

### 5. Verify Installation

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

## Configuration

### Environment Variables

Basic application settings are configured via environment variables (`.env` file):

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP server port | `3001` | Yes |
| `DATABASE_PATH` | SQLite database location | `./data/tapedeck.db` | Yes |
| `LOG_LEVEL` | Logging level | `info` | Yes |
| `SESSION_SECRET` | Session encryption key (32+ chars) | (generate with `openssl rand -hex 32`) | Yes |
| `DEV_MODE` | Skip TLS verification (dev only) | `true` | No |

### Runtime Configuration (config.yml)

Plex servers, Home Assistant, and Apple TVs are configured through the **setup wizard**, which creates a `config.yml` file:

```yaml
version: 1
plex_servers:
  - id: "server-id-123"
    name: "Home Server"
    owner: "username"
    connections:
      - uri: "http://192.168.1.100:32400"
        local: true
home_assistant:
  url: "http://192.168.1.101:8123"
  token: "your-long-lived-access-token"
apple_tvs:
  - entity: "media_player.living_room_apple_tv"
    name: "Living Room"
    default: true
  - entity: "media_player.bedroom_apple_tv"
    name: "Bedroom"
    default: false
```

**Multiple Server Support**: You can connect to multiple Plex servers. When pairing cards, you'll select which server the media comes from.

**Multiple Apple TV Support**: Configure multiple Apple TVs and choose which one to use when pairing each card.

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

- [x] **Stage 8**: Enhanced UI & Polish ✅
  - HA connection status banner (red alert if disconnected)
  - **Deferred to future**: Search and filter improvements on mappings dashboard
  - **Deferred to future**: Responsive design (see `docs/responsive-design.md`)
  - **Deferred to future**: Accessibility enhancements (see `docs/accessibility.md`)

- [x] **Stage 9**: Configuration Management ✅
  - **Web-based Setup Wizard**:
    - First-time configuration flow (`/setup`)
    - Plex OAuth authentication with server discovery
    - Home Assistant URL and token configuration with connection testing
    - Apple TV selection with auto-discovery
    - Multi-step wizard with session state management
  - **Runtime Configuration (config.yml)**:
    - YAML-based configuration storage
    - Multiple Plex server support
    - Multiple Apple TV support
    - Validation and error handling
  - **Setup Middleware**:
    - Automatic redirect to setup wizard if not configured
    - Exempts /auth and /setup routes
  - **Database Migration**:
    - Added plex_server_id and apple_tv_entity columns to card_mappings
    - Tracks which server and device each card uses
  - **Enhanced Pairing**:
    - Apple TV selection dropdown during pairing
    - Server-aware media search results
  - **Enhanced Playback**:
    - Uses mapping's stored server and device info
    - Falls back to defaults for backward compatibility
  - **Future enhancements** (deferred to Stage 11+):
    - Settings page to modify existing configuration
    - Token rotation and validation
    - Connection health dashboard
    - User roles and permissions

- [x] **Stage 10**: Production Deployment ✅
  - Multi-stage Docker build with layer caching
  - Non-root user and security hardening
  - Prometheus metrics endpoint (`/metrics`)
  - Request logging middleware with duration tracking
  - Health checks and graceful shutdown
  - Multi-platform builds (amd64, arm64)

- [x] **Stage 11**: Hardware Setup Documentation ✅
  - Complete hardware setup guide with component recommendations
  - Wiring diagrams for RC522 and PN532 readers
  - ESPHome configuration examples for ESP32/ESP8266
  - Home Assistant integration and event verification
  - Comprehensive troubleshooting guide
  - Physical installation recommendations

**Estimated Timeline**: 60-82 hours total (~6-8 weeks at 10-15 hours/week)

## Future Enhancements

The following improvements are documented for future work and may be good candidates for community contributions:

- **End-to-End Testing**: Automated integration tests covering full user workflows
- **User Documentation**: Comprehensive user guide with screenshots and video tutorials
- **Plex SDK Integration**: Replace custom Plex API client with established SDK like [plexgo](https://github.com/lukehagar/plexgo) to avoid XML parsing issues and stay current with API changes
- **Media Player Filtering**: Add ability to filter/distinguish between different types of media players (Apple TV, Chromecast, smart speakers) during setup wizard, possibly using entity ID patterns or Home Assistant device attributes
- **Responsive Design**: Mobile and tablet optimization - see [`docs/responsive-design.md`](docs/responsive-design.md)
- **Accessibility**: WCAG compliance, keyboard navigation, screen reader support - see [`docs/accessibility.md`](docs/accessibility.md)

## Contributing

This is a personal project under active development. Once v1 is complete and stable, contributions will be welcome.

## License

MIT

## Acknowledgments

Inspired by the blog post ["How I Built an NFC Movie Library for My Kids"](https://simplyexplained.com/blog/how-i-built-an-nfc-movie-library-for-my-kids/) and similar projects in the home automation community.
