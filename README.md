# TapeDeck

[![CI](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/Chuntttttt/TapeDeck/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Chuntttttt/TapeDeck)](https://goreportcard.com/report/github.com/Chuntttttt/TapeDeck)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)

A Go-based web application that bridges physical and digital media using NFC cards. Tap NFC cards on a reader to instantly play movies, TV shows, or music on your TV through Plex and Apple TV, managed via Home Assistant.

## Project Status

TapeDeck is feature-complete for personal use. Includes Plex authentication, multi-server media browsing, card mapping with real-time NFC pairing, Home Assistant integration with WebSocket, web-based setup wizard, Docker deployment, and hardware documentation.

## Overview

TapeDeck recreates the nostalgic experience of physical media libraries for the streaming age. Kids (and adults!) can tap a physical card to play their favorite content without navigating complex streaming interfaces.

### Key Features

- **Visual Media Browser**: Browse Plex libraries with poster art and metadata
- **Real-time NFC Pairing**: Assign media to cards by tapping them during setup
- **Zero YAML Editing**: All mappings managed through web UI
- **Smart TV Show Handling**: Automatically plays next unwatched episode
- **Home Assistant Integration**: Seamless playback on Apple TV
- **Multi-Server Support**: Connect to multiple Plex servers simultaneously
- **Multi-Device Support**: Configure multiple Apple TVs for playback
- **Web-Based Setup**: First-time configuration wizard with auto-discovery

## Technology Stack

- **Language**: Go 1.25+
- **Templating**: Templ (type-safe Go templates)
- **Frontend**: DataStar (SSE-based hypermedia)
- **Database**: SQLite (pure Go driver, no CGO)
- **WebSocket**: Home Assistant integration for NFC events
- **Deployment**: Docker / Docker Compose (see [Docker Deployment](#docker-deployment))

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

## Testing

### Test Organization

- Unit tests: `*_test.go` next to implementation files
- Mock HTTP responses for external APIs (Plex, HA)

## Implemented Features

**Core Functionality:**
- Plex authentication and multi-server support
- Media browsing with library and search interfaces
- Card mapping with inline search autocomplete
- Real-time NFC pairing with live tag detection
- Home Assistant WebSocket integration
- Playback via Apple TV with multi-device support

**Production Features:**
- Web-based setup wizard with auto-discovery
- SQLite database with migrations
- Session management and authentication
- Prometheus metrics endpoint
- Docker deployment with multi-platform support
- Request logging and health checks

**Documentation:**
- Complete hardware setup guide ([docs/hardware-setup.md](docs/hardware-setup.md))
- Home Assistant integration guide ([docs/home-assistant-setup.md](docs/home-assistant-setup.md))
- ESPHome configuration examples for RC522 and PN532 readers

## Future Enhancements

The following improvements are documented for future work and may be good candidates for community contributions:

- **End-to-End Testing**: Automated integration tests covering full user workflows
- **User Documentation**: Comprehensive user guide with screenshots and video tutorials
- **Plex SDK Integration**: Replace custom Plex API client with established SDK like [plexgo](https://github.com/lukehagar/plexgo) to avoid XML parsing issues and stay current with API changes
- **Media Player Filtering**: Add ability to filter/distinguish between different types of media players (Apple TV, Chromecast, smart speakers) during setup wizard, possibly using entity ID patterns or Home Assistant device attributes
- **Responsive Design**: Mobile and tablet optimization - see [`docs/responsive-design.md`](docs/responsive-design.md)
- **Accessibility**: WCAG compliance, keyboard navigation, screen reader support - see [`docs/accessibility.md`](docs/accessibility.md)

## Contributing

Contributions are welcome. See the [Future Enhancements](#future-enhancements) section for potential areas to work on.

When contributing:
- Follow existing code style and patterns
- Add tests for new functionality
- Update documentation as needed
- Open an issue first for major changes

## License

MIT

## Acknowledgments

Inspired by the blog post ["How I Built an NFC Movie Library for My Kids"](https://simplyexplained.com/blog/how-i-built-an-nfc-movie-library-for-my-kids/) and similar projects in the home automation community.
