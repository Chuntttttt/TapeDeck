package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/models"
)

// mockDB implements the DB methods needed for PlaybackService
type mockDB struct {
	getMappingFunc    func(ctx context.Context, tagID string) (*models.CardMapping, error)
	createLogFunc     func(ctx context.Context, log *models.PlaybackLog) (int64, error)
	getCardMappingErr error
	createLogErr      error
}

func (m *mockDB) GetCardMappingByTagID(ctx context.Context, tagID string) (*models.CardMapping, error) {
	if m.getMappingFunc != nil {
		return m.getMappingFunc(ctx, tagID)
	}
	if m.getCardMappingErr != nil {
		return nil, m.getCardMappingErr
	}
	return &models.CardMapping{
		TagID:         tagID,
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}, nil
}

func (m *mockDB) CreatePlaybackLog(ctx context.Context, log *models.PlaybackLog) (int64, error) {
	if m.createLogFunc != nil {
		return m.createLogFunc(ctx, log)
	}
	if m.createLogErr != nil {
		return 0, m.createLogErr
	}
	return 1, nil
}

// mockHARestClient implements HARestClient interface
type mockHARestClient struct {
	getStateFunc  func(ctx context.Context, entityID string) (string, error)
	turnOnFunc    func(ctx context.Context, entityID string) error
	playMediaFunc func(ctx context.Context, entityID, contentType, contentID string) error
	getStateErr   error
	turnOnErr     error
	playMediaErr  error
	entityState   string
}

func (m *mockHARestClient) GetEntityState(ctx context.Context, entityID string) (string, error) {
	if m.getStateFunc != nil {
		return m.getStateFunc(ctx, entityID)
	}
	if m.getStateErr != nil {
		return "", m.getStateErr
	}
	return m.entityState, nil
}

func (m *mockHARestClient) TurnOn(ctx context.Context, entityID string) error {
	if m.turnOnFunc != nil {
		return m.turnOnFunc(ctx, entityID)
	}
	return m.turnOnErr
}

func (m *mockHARestClient) PlayMedia(ctx context.Context, entityID, contentType, contentID string) error {
	if m.playMediaFunc != nil {
		return m.playMediaFunc(ctx, entityID, contentType, contentID)
	}
	return m.playMediaErr
}

func TestPlayByTagID_Success(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{entityState: "idle"}
	service := NewPlaybackService(db, haRest)

	result, err := service.PlayByTagID(context.Background(), "test-tag-123")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success=true, got false")
	}
	if result.MediaTitle != "Test Movie" {
		t.Errorf("Expected media title 'Test Movie', got %s", result.MediaTitle)
	}
}

func TestPlayByTagID_MappingNotFound(t *testing.T) {
	db := &mockDB{
		getCardMappingErr: errors.New("card mapping not found"),
	}
	haRest := &mockHARestClient{}
	service := NewPlaybackService(db, haRest)

	result, err := service.PlayByTagID(context.Background(), "unknown-tag")

	if err == nil {
		t.Fatal("Expected error for missing mapping, got nil")
	}
	if result != nil {
		t.Errorf("Expected nil result, got %+v", result)
	}
}

func TestPlay_DeviceIdle(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{entityState: "idle"}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success=true, got false")
	}
	if result.WokeDevice {
		t.Errorf("Expected woke_device=false, got true")
	}
	if result.AppleTVState != "idle" {
		t.Errorf("Expected state 'idle', got %s", result.AppleTVState)
	}
}

func TestPlay_DeviceAsleep_WakesDevice(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{entityState: "off"}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success=true, got false")
	}
	if !result.WokeDevice {
		t.Errorf("Expected woke_device=true, got false")
	}
}

func TestPlay_StateCheckFails_ContinuesAnyway(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{
		getStateErr: errors.New("failed to get state"),
	}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	// Should succeed despite state check failure
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success=true (graceful degradation), got false")
	}
	if result.AppleTVState != "unknown" {
		t.Errorf("Expected state 'unknown', got %s", result.AppleTVState)
	}
}

func TestPlay_WakeDeviceFails(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{
		entityState: "off",
		turnOnErr:   errors.New("failed to turn on"),
	}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	if err == nil {
		t.Fatal("Expected error when wake fails, got nil")
	}
	if result.Success {
		t.Errorf("Expected success=false, got true")
	}
	if result.Error == nil {
		t.Errorf("Expected result.Error to be set")
	}
}

func TestPlay_PlayMediaFails(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{
		entityState:  "idle",
		playMediaErr: errors.New("failed to play media"),
	}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	if err == nil {
		t.Fatal("Expected error when PlayMedia fails, got nil")
	}
	if result.Success {
		t.Errorf("Expected success=false, got true")
	}
	if result.Error == nil {
		t.Errorf("Expected result.Error to be set")
	}
}

func TestPlay_LoggingFails_DoesNotFailRequest(t *testing.T) {
	db := &mockDB{
		createLogErr: errors.New("failed to create log"),
	}
	haRest := &mockHARestClient{entityState: "idle"}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-123",
		AppleTVEntity: "media_player.living_room",
	}

	result, err := service.Play(context.Background(), req)

	// Should succeed despite logging failure
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success=true (logging failure should not fail request), got false")
	}
}

func TestPlay_PlexURLFormat(t *testing.T) {
	db := &mockDB{}
	var capturedContentID string
	haRest := &mockHARestClient{
		entityState: "idle",
		playMediaFunc: func(ctx context.Context, entityID, contentType, contentID string) error {
			capturedContentID = contentID
			return nil
		},
	}
	service := NewPlaybackService(db, haRest)

	req := &PlaybackRequest{
		TagID:         "tag-123",
		UserID:        1,
		MediaID:       "12345",
		MediaTitle:    "Test Movie",
		PlexServerID:  "server-abc",
		AppleTVEntity: "media_player.living_room",
	}

	_, err := service.Play(context.Background(), req)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedURL := "plex://play/?metadataKey=/library/metadata/12345&server=server-abc"
	if capturedContentID != expectedURL {
		t.Errorf("Expected Plex URL %s, got %s", expectedURL, capturedContentID)
	}
}

func TestWakeAppleTV_Success(t *testing.T) {
	db := &mockDB{}
	turnOnCalled := false
	haRest := &mockHARestClient{
		turnOnFunc: func(ctx context.Context, entityID string) error {
			turnOnCalled = true
			return nil
		},
	}
	service := NewPlaybackService(db, haRest)

	err := service.wakeAppleTV(context.Background(), "media_player.living_room")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !turnOnCalled {
		t.Errorf("Expected TurnOn to be called")
	}
}

func TestWakeAppleTV_ContextCancelled(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{}
	service := NewPlaybackService(db, haRest)

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.wakeAppleTV(ctx, "media_player.living_room")

	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestWakeAppleTV_TurnOnFails(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{
		turnOnErr: errors.New("device unavailable"),
	}
	service := NewPlaybackService(db, haRest)

	err := service.wakeAppleTV(context.Background(), "media_player.living_room")

	if err == nil {
		t.Fatal("Expected error when TurnOn fails, got nil")
	}
}

func TestWakeAppleTV_Timeout(t *testing.T) {
	db := &mockDB{}
	haRest := &mockHARestClient{}
	service := NewPlaybackService(db, haRest)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait a bit to ensure timeout
	time.Sleep(5 * time.Millisecond)

	err := service.wakeAppleTV(ctx, "media_player.living_room")

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded error, got %v", err)
	}
}
