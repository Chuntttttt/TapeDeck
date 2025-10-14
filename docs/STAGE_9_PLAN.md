# Stage 9: Configuration Management & Setup Wizard - Implementation Plan

## Overview
Move user-configurable runtime settings from `.env` to `config.yml` with a first-time setup wizard supporting multiple Plex servers and Apple TVs.

## Configuration Changes

### Remove from .env.example (keep in .env if exists, just not tracked)
```bash
# Remove these lines:
PLEX_SERVER_ID=...
HA_URL=...
HA_TOKEN=...
APPLE_TV_ENTITY=...
PLEX_URL=...  # Optional - can be removed since we use discovered connections
```

### config.yml Structure
```yaml
version: 1

plex_servers:
  - id: "abc123def456"
    name: "Home Server"
    owner: "username"
    connections:
      - uri: "http://192.168.1.100:32400"
        local: true
      - uri: "https://plex.direct:32400"
        local: false

home_assistant:
  url: "http://homeassistant.local:8123"
  token: "eyJ..."

apple_tvs:
  - entity: "media_player.living_room"
    name: "Living Room Apple TV"
    default: true  # Last used or only one
  - entity: "media_player.bedroom"
    name: "Bedroom Apple TV"
    default: false
```

**Location:** `./config.yml` (same directory as database)
**Permissions:** 600 (owner-only, contains sensitive tokens)

---

## Database Schema Changes

### Migration: Add server/device columns to card_mappings
```sql
-- Migration: 002_add_server_device_to_mappings.sql
ALTER TABLE card_mappings ADD COLUMN plex_server_id TEXT NOT NULL DEFAULT '';
ALTER TABLE card_mappings ADD COLUMN apple_tv_entity TEXT NOT NULL DEFAULT '';

-- No backfill needed since no existing users
```

**Updated models.CardMapping struct:**
```go
type CardMapping struct {
    ID            int64
    UserID        int64
    TagID         string
    MediaType     string
    MediaID       string
    MediaTitle    string
    PlexServerID  string  // NEW
    AppleTVEntity string  // NEW
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

---

## Setup Wizard Flow

### Routes
- `GET /setup` - Wizard landing page (Step 1)
- `GET /setup/step/2` - Plex authentication
- `GET /setup/step/3` - Home Assistant configuration
- `GET /setup/step/4` - Apple TV selection (optional)
- `GET /setup/step/5` - Complete summary
- `POST /setup/plex/servers` - Save selected servers
- `POST /setup/ha/test` - Test HA connection
- `POST /setup/ha/save` - Save HA config
- `POST /setup/appletv/save` - Save Apple TV selections
- `POST /setup/complete` - Finalize setup

### Step 1: Welcome
```
🎬 Welcome to TapeDeck

TapeDeck lets you play Plex media on your Apple TV by tapping NFC cards.

Let's get you set up in 4 easy steps:
1. Connect your Plex account
2. Configure Home Assistant
3. Add Apple TVs (optional)
4. Start pairing cards!

[Get Started]
```

### Step 2: Plex Authentication
**If not logged in:**
```
Connect Your Plex Account

TapeDeck needs access to your Plex servers to browse and play media.

[Login with Plex]
```

**After OAuth, show discovered servers:**
```
Select Plex Servers

We found 2 servers connected to your account:

☑️ Home Server (username) - 192.168.1.100
☑️ Cloud Server (username) - plex.direct

All servers are selected by default. Uncheck any you don't want to use.

[Continue with Selected Servers]
```

**If no servers found:**
```
❌ No Plex servers found

Make sure you have access to at least one Plex Media Server.

[Try Again] [Plex Server Setup Guide]
```

### Step 3: Home Assistant Configuration
```
Connect Home Assistant

Home Assistant controls playback on your Apple TV.

Home Assistant URL
[http://homeassistant.local:8123]
Your Home Assistant instance URL

Long-Lived Access Token
[•••••••••••••••••••••••]
Create one in Home Assistant Profile settings
[How to create a token] → Links to HA docs

[Test Connection]

✓ Connected to Home Assistant v2024.10.3

[Continue]
```

**If HA URL is known after test, show direct link:**
```
[Create Token in Home Assistant] → Opens {HA_URL}/profile
```

**Error states:**
- Connection failed: Show specific error
- Invalid token: Link to token creation page
- Timeout: Check URL and network

### Step 4: Apple TV Selection
**If media players found:**
```
Select Apple TVs

We found 2 media players in Home Assistant:

☑️ Living Room Apple TV (media_player.living_room) - idle
☑️ Bedroom Apple TV (media_player.bedroom) - off

[Continue] [Skip for Now]
```

**If only one found:**
```
✓ Auto-detected: Living Room Apple TV

[Continue]
```

**If no media players:**
```
No Apple TVs Found

No media players were found in Home Assistant. You can add them later in settings.

[Skip for Now]
```

### Step 5: Complete
```
✓ Setup Complete!

Configuration Summary:
• 2 Plex servers connected
• Home Assistant connected (http://homeassistant.local:8123)
• 2 Apple TVs available

You're ready to start pairing NFC cards with your media!

[Start Using TapeDeck] → Redirects to /libraries
```

---

## Pairing Flow Changes

### Search Results (Multiple Servers)
Search queries all configured Plex servers in parallel and aggregates results.

**Display:**
```
Search: "toy story"

Results (12):

🎬 Toy Story (1995) - Movie
   Home Server • 1h 21m • 1995

🎬 Toy Story 2 (1999) - Movie
   Home Server • 1h 32m • 1999

🎬 Toy Story (1995) - Movie
   Cloud Server • 1h 21m • 1995

Filter by server: [All Servers ▼]
                   All Servers
                   Home Server
                   Cloud Server
```

**Search implementation:**
- Query each server's `/library/sections/all/search?query=...`
- Aggregate results with server information
- Remove duplicates (same media_id on multiple servers shows both)
- Sort by relevance/title

### Pairing Form
```
Pair NFC Card

Step 1: Search for Media
[Search input with results...]

Step 2: Select Playback Device
Selected Media:
🎬 Toy Story (1995) - Movie
Home Server

Play on:
[Living Room Apple TV ▼]
  Living Room Apple TV
  Bedroom Apple TV

If only one Apple TV configured, auto-select and show:
Play on: Living Room Apple TV ✓

Step 3: Tap Your NFC Card
[Start Pairing Mode]
```

### Database Record
```
card_mappings:
  tag_id: "04-16-5C-D4-2E-61-80"
  media_id: "12345"
  media_title: "Toy Story"
  media_type: "movie"
  plex_server_id: "abc123def456"        # NEW
  apple_tv_entity: "media_player.living_room"  # NEW
```

**Default Apple TV selection:**
- If only one configured: Auto-select
- If multiple: Remember last-used per user (store in session or user preferences)

---

## Playback Flow Changes

**Current flow:**
1. Tag scanned → Lookup mapping by tag_id
2. Build Plex URL with PLEX_SERVER_ID from .env
3. Play on APPLE_TV_ENTITY from .env

**New flow:**
1. Tag scanned → Lookup mapping by tag_id
2. Get plex_server_id and apple_tv_entity from mapping
3. Load server config from config.yml by server_id
4. Get connections list from server config
5. Try connections in order (local first, indicated by `local: true`)
6. Use first successful connection to build Plex deep link:
   `plex://play/?metadataKey=/library/metadata/{media_id}&server={plex_server_id}`
7. Call HA REST API to play on specified apple_tv_entity

**Connection selection logic:**
- Try each connection in order until one succeeds
- Plex SDK orders connections by preference (local first)
- Cache working connection for session (optional optimization)

---

## Admin Page (Future Enhancement - Document as Missing)

**Routes (not implemented in Stage 9):**
- `GET /admin` - Settings dashboard
- `POST /admin/plex/refresh` - Refresh servers from Plex account
- `POST /admin/servers/toggle` - Enable/disable specific server
- `POST /admin/appletv/add` - Add new Apple TV
- `POST /admin/appletv/remove` - Remove Apple TV
- `POST /admin/servers/edit` - Edit server friendly name
- `POST /admin/appletv/edit` - Edit Apple TV friendly name

**Features to document as missing:**
- Add/remove Plex servers manually without re-auth
- Enable/disable servers temporarily without removing
- Add Apple TVs from HA without full re-scan
- Remove individual Apple TVs
- Edit server/Apple TV friendly names (override discovered names)
- Re-run setup wizard
- Test individual connections
- View connection status for each server

**Note:** Admin page can be implemented in Stage 10 or later based on user needs.

---

## Implementation Order

### Phase 1: Configuration Infrastructure ✓
1. Create `internal/config/runtime.go` with RuntimeConfig struct
2. Add YAML marshal/unmarshal support (use `gopkg.in/yaml.v3`)
3. Create LoadRuntimeConfig() and Save() methods
4. Add validation logic (IsEmpty(), Validate())
5. Add file permissions handling (0600)

**Files:**
- `internal/config/runtime.go`

### Phase 2: Plex Server Discovery
1. Create `internal/plex/servers.go`
2. Implement GetServers(authToken) method
3. Parse /api/v2/resources response
4. Extract server ID, name, owner, connections
5. Map XML/JSON to PlexServer struct

**Files:**
- `internal/plex/servers.go`

**API Endpoint:**
- `GET https://plex.tv/api/v2/resources?includeHttps=1&includeRelay=1`

### Phase 3: HA Entity Discovery
1. Add GetStates() to `internal/ha/rest.go`
2. Add GetMediaPlayers() filter method
3. Parse friendly names from attributes
4. Handle missing friendly_name gracefully

**Files:**
- `internal/ha/rest.go` (existing file, add methods)

**API Endpoint:**
- `GET {HA_URL}/api/states`

### Phase 4: Database Migration
1. Create `migrations/002_add_server_device_to_mappings.sql`
2. Add plex_server_id and apple_tv_entity columns
3. Update models.CardMapping struct in `internal/models/card_mapping.go`
4. Update db methods: CreateCardMapping, UpdateCardMapping
5. Update queries to include new fields

**Files:**
- `migrations/002_add_server_device_to_mappings.sql`
- `internal/models/card_mapping.go`
- `internal/db/card_mappings.go`

### Phase 5: Setup Wizard Handler
1. Create `internal/handlers/setup.go`
2. Implement multi-step form with session state
3. Build HTML templates for each step
4. Add POST handlers for each step
5. Store partial config in session during wizard
6. Save final config to config.yml on complete

**Files:**
- `internal/handlers/setup.go`

**Session data:**
```go
type SetupState struct {
    Step          int
    PlexServers   []config.PlexServer
    HAConfig      config.HAConfig
    AppleTVs      []config.AppleTV
}
```

### Phase 6: Setup Middleware
1. Create middleware to check if setup is complete
2. Redirect to /setup if config.yml missing or invalid
3. Allow /setup, /auth routes without check
4. Add to main.go before other routes

**Files:**
- `internal/middleware/setup.go`
- `main.go` (add middleware)

### Phase 7: Update Pairing Handler
1. Modify search to query multiple servers in parallel
2. Add server information to search results
3. Add server filter dropdown to search UI
4. Add Apple TV dropdown to pairing form
5. Store server_id + apple_tv_entity in mapping
6. Default Apple TV selection logic

**Files:**
- `internal/handlers/pairing.go` (modify existing)
- `internal/handlers/mappings.go` (modify search)

### Phase 8: Update Playback Handler
1. Load server config by server_id from config.yml
2. Try connections in order (local first)
3. Build Plex deep link with first successful connection
4. Use specified apple_tv_entity for playback
5. Handle missing server/entity errors

**Files:**
- `internal/handlers/pairing.go` (modify playMedia method)

### Phase 9: Update .env.example
1. Remove PLEX_SERVER_ID
2. Remove HA_URL
3. Remove HA_TOKEN
4. Remove APPLE_TV_ENTITY
5. Remove PLEX_URL (optional)
6. Add comment about config.yml

**Files:**
- `.env.example`

### Phase 10: Update Documentation
1. Update README.md with new setup instructions
2. Document config.yml structure
3. Add troubleshooting for setup wizard
4. Document multiple server/Apple TV features

**Files:**
- `README.md`

---

## Technical Details

### RuntimeConfig Structure
```go
type RuntimeConfig struct {
    Version        int            `yaml:"version"`
    PlexServers    []PlexServer   `yaml:"plex_servers"`
    HomeAssistant  HAConfig       `yaml:"home_assistant"`
    AppleTVs       []AppleTV      `yaml:"apple_tvs"`
}

type PlexServer struct {
    ID          string       `yaml:"id"`
    Name        string       `yaml:"name"`
    Owner       string       `yaml:"owner"`
    Connections []Connection `yaml:"connections"`
}

type Connection struct {
    URI   string `yaml:"uri"`
    Local bool   `yaml:"local"`
}

type HAConfig struct {
    URL   string `yaml:"url"`
    Token string `yaml:"token"`
}

type AppleTV struct {
    Entity  string `yaml:"entity"`
    Name    string `yaml:"name"`
    Default bool   `yaml:"default"`
}
```

### Plex Server Discovery Response
```xml
<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
  <Device name="Home Server"
          clientIdentifier="abc123"
          owned="1"
          ownerId="123">
    <Connection uri="http://192.168.1.100:32400" local="1"/>
    <Connection uri="https://plex.direct:32400" local="0"/>
  </Device>
</MediaContainer>
```

### HA States Response
```json
[
  {
    "entity_id": "media_player.living_room",
    "state": "idle",
    "attributes": {
      "friendly_name": "Living Room Apple TV",
      "supported_features": 152463
    }
  }
]
```

### Migration Strategy
No migration needed for existing users - they will go through setup wizard on first launch after update.

---

## Security Considerations

1. **config.yml permissions:** Set to 0600 (owner read/write only)
2. **Token storage:** Tokens stored in plaintext in config.yml (consider encryption in future)
3. **Setup wizard access:** Requires Plex authentication first
4. **First user is admin:** First Plex account to complete setup is treated as admin
5. **Config file location:** Same directory as database, not in web-accessible path

---

## User Experience Considerations

1. **Clear error messages:** Show specific errors (connection failed, invalid token, etc.)
2. **Help links:** Link to HA docs for token creation, Plex server setup
3. **Direct links:** If HA URL known, link directly to {HA_URL}/profile
4. **Auto-detection:** Auto-select if only one server/Apple TV
5. **Default selection:** Remember last-used Apple TV for convenience
6. **Progress indication:** Show current step in wizard (Step 2 of 4)
7. **Validation:** Test connections before saving config
8. **Graceful degradation:** Allow skipping optional steps (Apple TV)

---

## Testing Strategy

1. **Unit tests:** Test config loading/saving, validation
2. **Integration tests:** Test Plex server discovery, HA entity fetch
3. **Manual testing:** Full wizard flow with real Plex/HA instances
4. **Error cases:** Test with invalid tokens, unreachable servers
5. **Multiple servers:** Test with 0, 1, 2+ Plex servers
6. **Multiple Apple TVs:** Test with 0, 1, 2+ media players
7. **Migration:** Test fresh install (no existing config)

---

## Future Enhancements (Post-Stage 9)

1. **Admin UI:** Full settings page for managing servers/devices
2. **Server health monitoring:** Show connection status for each server
3. **Token encryption:** Encrypt sensitive tokens in config.yml
4. **Per-user config:** Allow different users to have different Apple TV preferences
5. **Custom names:** Override discovered names for servers/devices
6. **Server enable/disable:** Temporarily disable servers without removing
7. **Connection caching:** Cache working connections for performance
8. **Network detection:** Automatically prefer local connections on same network
9. **Multi-device support:** Support other streaming devices (Roku, Fire TV, etc.)
10. **Alternative deep links:** YouTube, Disney+, Apple TV+ support

---

## Decisions & Rationale

### Why config.yml instead of database?
- Users may want to hand-edit configuration
- Easier to backup/restore
- Can be version controlled
- Clear separation of user data (database) vs configuration

### Why YAML instead of JSON?
- More human-readable
- Supports comments for documentation
- Standard for config files

### Why multiple servers/Apple TVs?
- More flexible for power users
- Supports multiple locations
- Better for shared households
- Future-proofs architecture

### Why setup wizard instead of admin-first?
- Better first-run experience
- Ensures valid config before use
- Guides user through necessary steps
- Reduces support burden

### Why auto-detect instead of manual entry?
- Reduces errors (typos in IDs)
- Faster setup
- Better user experience
- Leverages existing API capabilities
