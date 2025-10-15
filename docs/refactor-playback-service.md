# Playback Service Layer Refactor

**Goal**: Extract playback logic from handlers into a testable service layer with clean interfaces.

## Problem

Currently in `internal/handlers/pairing.go:310-370`, playback logic is tightly coupled to the HTTP handler:
- Direct calls to HA REST API
- Embedded business rules (wake Apple TV, wait 5 seconds, etc.)
- Hard to unit test without mocking HTTP layer
- Can't reuse playback logic from other contexts (CLI, API, scheduled tasks)

## Proposed Architecture

```
┌─────────────────┐
│   HTTP Handler  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Playback Service│ ◄── Focus of this refactor
└────────┬────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌────────┐ ┌────────┐
│   DB   │ │   HA   │
└────────┘ └────────┘
```

## Service Interface

Create `internal/services/playback.go`:

```go
package services

import (
    "context"
    "fmt"
    "time"

    "github.com/Chuntttttt/tapedeck/internal/db"
    "github.com/Chuntttttt/tapedeck/internal/ha"
    "github.com/Chuntttttt/tapedeck/internal/logger"
    "github.com/Chuntttttt/tapedeck/internal/models"
)

// PlaybackService handles media playback orchestration
type PlaybackService struct {
    db     *db.DB
    haRest HARestClient
}

// HARestClient defines the HA REST operations needed for playback
type HARestClient interface {
    GetEntityState(entityID string) (string, error)
    TurnOn(entityID string) error
    PlayMedia(entityID, contentType, contentID string) error
}

// PlaybackRequest contains all data needed to play media
type PlaybackRequest struct {
    TagID         string
    UserID        int64
    PlexServerID  string
    AppleTVEntity string
}

// PlaybackResult contains the outcome of a playback request
type PlaybackResult struct {
    Success      bool
    MediaTitle   string
    MediaID      string
    AppleTVState string
    WokeDevice   bool
    Error        error
}

// NewPlaybackService creates a new playback service
func NewPlaybackService(database *db.DB, haRest HARestClient) *PlaybackService {
    return &PlaybackService{
        db:     database,
        haRest: haRest,
    }
}

// PlayByTagID looks up a card mapping and plays the associated media
func (s *PlaybackService) PlayByTagID(ctx context.Context, tagID string) (*PlaybackResult, error) {
    log := logger.Get()

    // Look up mapping
    mapping, err := s.db.GetCardMappingByTagID(tagID)
    if err != nil {
        return nil, fmt.Errorf("no mapping found for tag %s: %w", tagID, err)
    }

    log.Info("Found mapping for tag",
        "tag_id", tagID,
        "media_title", mapping.MediaTitle,
        "apple_tv", mapping.AppleTVEntity)

    // Play the media
    return s.Play(ctx, &PlaybackRequest{
        TagID:         tagID,
        UserID:        mapping.UserID,
        PlexServerID:  mapping.PlexServerID,
        AppleTVEntity: mapping.AppleTVEntity,
    }, mapping)
}

// Play executes the playback flow
func (s *PlaybackService) Play(ctx context.Context, req *PlaybackRequest, mapping *models.CardMapping) (*PlaybackResult, error) {
    log := logger.Get()
    result := &PlaybackResult{
        MediaTitle: mapping.MediaTitle,
        MediaID:    mapping.MediaID,
    }

    // Build Plex deep link
    plexURL := fmt.Sprintf(
        "plex://play/?metadataKey=/library/metadata/%s&server=%s",
        mapping.MediaID,
        req.PlexServerID,
    )

    // Check Apple TV state
    state, err := s.haRest.GetEntityState(req.AppleTVEntity)
    if err != nil {
        log.Warn("Failed to get Apple TV state, attempting playback anyway",
            "apple_tv", req.AppleTVEntity,
            "error", err)
        state = "unknown"
    }
    result.AppleTVState = state

    // Wake device if needed
    if state == "off" || state == "standby" {
        log.Info("Apple TV is asleep, waking up",
            "apple_tv", req.AppleTVEntity,
            "state", state)

        if err := s.wakeAppleTV(ctx, req.AppleTVEntity); err != nil {
            result.Error = fmt.Errorf("failed to wake Apple TV: %w", err)
            return result, result.Error
        }
        result.WokeDevice = true
    }

    // Play media
    if err := s.haRest.PlayMedia(req.AppleTVEntity, "url", plexURL); err != nil {
        result.Error = fmt.Errorf("failed to play media: %w", err)
        return result, result.Error
    }

    log.Info("Playback started",
        "media_title", mapping.MediaTitle,
        "apple_tv", req.AppleTVEntity)

    // Log playback
    if err := s.logPlayback(mapping.UserID, mapping.TagID, mapping.MediaID, mapping.MediaTitle); err != nil {
        log.Warn("Failed to log playback", "error", err)
        // Don't fail the request if logging fails
    }

    result.Success = true
    return result, nil
}

// wakeAppleTV wakes an Apple TV and waits for it to be ready
func (s *PlaybackService) wakeAppleTV(ctx context.Context, entityID string) error {
    if err := s.haRest.TurnOn(entityID); err != nil {
        return fmt.Errorf("turn_on failed: %w", err)
    }

    // Wait for device to wake up (configurable in future)
    wakeTime := 5 * time.Second
    logger.Debug("Waiting for Apple TV to wake up",
        "apple_tv", entityID,
        "wait_time", wakeTime)

    select {
    case <-time.After(wakeTime):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

// logPlayback creates a playback log entry
func (s *PlaybackService) logPlayback(userID int64, tagID, mediaID, mediaTitle string) error {
    playbackLog := models.NewPlaybackLog(userID, tagID, mediaID, mediaTitle)
    _, err := s.db.CreatePlaybackLog(playbackLog)
    return err
}
```

## Handler Refactor

Update `internal/handlers/pairing.go`:

```go
type PairingHandler struct {
    sessionStore    *sessions.CookieStore
    db              *db.DB
    haClient        HAClientInterface
    playbackService *services.PlaybackService // NEW
    configPath      string
    upgrader        websocket.Upgrader
    clients         map[*pairingClient]bool
    clientsMu       sync.Mutex
}

func NewPairingHandler(
    store *sessions.CookieStore,
    database *db.DB,
    haClient HAClientInterface,
    playbackService *services.PlaybackService, // NEW
    configPath string,
) *PairingHandler {
    // ... constructor logic
}

// playMedia now just delegates to service
func (h *PairingHandler) playMedia(tagID string) {
    ctx := context.Background() // TODO: propagate from request
    result, err := h.playbackService.PlayByTagID(ctx, tagID)
    if err != nil {
        logger.Error("Playback failed", "tag_id", tagID, "error", err)
        return
    }

    if !result.Success {
        logger.Error("Playback unsuccessful", "tag_id", tagID, "error", result.Error)
        return
    }

    logger.Info("Playback completed",
        "tag_id", tagID,
        "media", result.MediaTitle,
        "woke_device", result.WokeDevice)
}
```

## Testing

Now we can test playback logic without HTTP layer:

```go
// internal/services/playback_test.go
package services

import (
    "context"
    "errors"
    "testing"

    "github.com/Chuntttttt/tapedeck/internal/models"
)

type mockHARestClient struct {
    state      string
    stateErr   error
    turnOnErr  error
    playErr    error
    turnOnCalls int
    playCalls   int
}

func (m *mockHARestClient) GetEntityState(entityID string) (string, error) {
    return m.state, m.stateErr
}

func (m *mockHARestClient) TurnOn(entityID string) error {
    m.turnOnCalls++
    return m.turnOnErr
}

func (m *mockHARestClient) PlayMedia(entityID, contentType, contentID string) error {
    m.playCalls++
    return m.playErr
}

func TestPlaybackService_WakesDeviceWhenOff(t *testing.T) {
    mockHA := &mockHARestClient{state: "off"}
    mockDB := newMockDB() // You'll need to create this

    svc := NewPlaybackService(mockDB, mockHA)

    mapping := &models.CardMapping{
        UserID:        1,
        TagID:         "test-tag",
        MediaID:       "123",
        MediaTitle:    "Test Movie",
        PlexServerID:  "server-1",
        AppleTVEntity: "media_player.living_room",
    }

    req := &PlaybackRequest{
        TagID:         "test-tag",
        UserID:        1,
        PlexServerID:  "server-1",
        AppleTVEntity: "media_player.living_room",
    }

    result, err := svc.Play(context.Background(), req, mapping)

    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    if !result.Success {
        t.Errorf("Expected success=true, got false")
    }

    if !result.WokeDevice {
        t.Errorf("Expected WokeDevice=true when Apple TV is off")
    }

    if mockHA.turnOnCalls != 1 {
        t.Errorf("Expected 1 turn_on call, got %d", mockHA.turnOnCalls)
    }

    if mockHA.playCalls != 1 {
        t.Errorf("Expected 1 play call, got %d", mockHA.playCalls)
    }
}

func TestPlaybackService_SkipsWakeWhenPlaying(t *testing.T) {
    mockHA := &mockHARestClient{state: "playing"}
    mockDB := newMockDB()

    svc := NewPlaybackService(mockDB, mockHA)

    // ... similar setup ...

    result, err := svc.Play(context.Background(), req, mapping)

    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    if result.WokeDevice {
        t.Errorf("Expected WokeDevice=false when Apple TV is already playing")
    }

    if mockHA.turnOnCalls != 0 {
        t.Errorf("Expected 0 turn_on calls, got %d", mockHA.turnOnCalls)
    }
}

func TestPlaybackService_ContinuesOnStateCheckFailure(t *testing.T) {
    mockHA := &mockHARestClient{
        stateErr: errors.New("HA unavailable"),
        state:    "",
    }
    mockDB := newMockDB()

    svc := NewPlaybackService(mockDB, mockHA)

    // ... setup ...

    // Should still attempt playback even if state check fails
    result, err := svc.Play(context.Background(), req, mapping)

    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    if mockHA.playCalls != 1 {
        t.Errorf("Expected playback to be attempted despite state check failure")
    }
}
```

## Migration Checklist

- [ ] Create `internal/services/playback.go`
- [ ] Define `PlaybackService` struct and interfaces
- [ ] Implement `PlayByTagID()` method
- [ ] Implement `Play()` method
- [ ] Implement `wakeAppleTV()` helper
- [ ] Implement `logPlayback()` helper
- [ ] Update `main.go` to initialize `PlaybackService`
- [ ] Update `PairingHandler` to use service
- [ ] Update `PlaybackHandler` to use service (if applicable)
- [ ] Write unit tests for playback service
- [ ] Remove old playback logic from handlers
- [ ] Add context propagation throughout

## Future Enhancements

Once service layer exists, we can easily add:

1. **Configurable wake time**: Move `5 * time.Second` to config
2. **Retry logic**: Retry playback if first attempt fails
3. **TV show episode tracking**: "Play next unwatched episode" logic
4. **Playback queue**: Queue multiple items
5. **Alternative players**: Support Roku, Chromecast, etc.
6. **Playback stats**: Track what gets played most
7. **CLI tool**: `tapedeck play --tag abc123`

## Timeline Estimate

- Service creation: 2 hours
- Handler refactor: 1 hour
- Testing: 2 hours
- Integration: 1 hour

**Total: 6 hours (1 day)**
