// Package app provides application initialization logic.
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/handlers"
	"github.com/Chuntttttt/tapedeck/internal/logger"
	"github.com/Chuntttttt/tapedeck/internal/services"
	"github.com/gorilla/sessions"
)

// Initializer handles application initialization and handler setup.
type Initializer struct {
	ConfigPath   string
	SessionStore *sessions.CookieStore
	Database     *db.DB
	DevMode      bool
}

// Handlers contains all initialized application handlers.
type Handlers struct {
	Media       *handlers.MediaHandler
	MediaDetail *handlers.MediaDetailHandler
	Mappings    *handlers.MappingsHandler
	Playback    *handlers.PlaybackHandler
	Play        *handlers.PlayHandler
	Pairing     *handlers.PairingHandler
	Status      *handlers.StatusHandler
	HAClient    *ha.HAClient
}

// Initialize sets up all runtime handlers from config.yml.
//
// This function is called in three scenarios:
// 1. At startup if config.yml exists and is valid
// 2. After setup wizard completion (via callback)
// 3. After settings update (via callback)
//
// Handler initialization is synchronous and atomic - either all handlers are
// initialized or the function returns an error.
func (init *Initializer) Initialize(existingHAClient *ha.HAClient) (*Handlers, error) {
	logger.Info("Initializing handlers")

	// Close existing HA client if it exists
	if existingHAClient != nil {
		logger.Info("Closing existing Home Assistant connection")
		existingHAClient.Close()
	}

	// Reload config
	runtimeCfg, err := config.LoadRuntimeConfig(init.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Load HA token from database
	ctx := context.Background()
	settings, err := init.Database.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load Home Assistant token from database: %w", err)
	}
	haToken := settings.HAToken

	// Build list of servers with their best connections
	servers, plexServerID := buildServerList(runtimeCfg.PlexServers)

	// Initialize handlers
	h := &Handlers{
		Media:       handlers.NewMediaHandler(init.SessionStore, init.Database, servers, init.DevMode),
		MediaDetail: handlers.NewMediaDetailHandler(init.SessionStore, init.Database, servers, init.DevMode, runtimeCfg.AppleTVs),
		Mappings:    handlers.NewMappingsHandler(init.SessionStore, init.Database, servers, init.DevMode),
		Playback:    handlers.NewPlaybackHandler(init.Database, plexServerID),
		Play:        handlers.NewPlayHandler(init.SessionStore, init.Database, servers, runtimeCfg.AppleTVs, runtimeCfg.HomeAssistant.URL, haToken, init.DevMode),
	}

	// Initialize Home Assistant WebSocket client
	h.HAClient = ha.NewHAClient(runtimeCfg.HomeAssistant.URL, haToken)
	if err := h.HAClient.Connect(); err != nil {
		logger.Warn("Failed to connect to Home Assistant", "error", err)
	}

	// Initialize Home Assistant REST client
	haRest := ha.NewRestClient(runtimeCfg.HomeAssistant.URL, haToken, init.DevMode)

	// Initialize playback service
	playbackService := services.NewPlaybackService(init.Database, haRest)

	// Initialize pairing handler
	h.Pairing = handlers.NewPairingHandler(
		init.SessionStore,
		init.Database,
		h.HAClient,
		playbackService,
		init.ConfigPath,
		init.DevMode,
	)

	// Initialize status handler
	h.Status = handlers.NewStatusHandler(h.HAClient, init.ConfigPath, init.Database)

	// Sanity check: ensure all handlers were initialized
	if h.Media == nil || h.MediaDetail == nil || h.Mappings == nil || h.Playback == nil ||
		h.Play == nil || h.Pairing == nil || h.Status == nil {
		return nil, fmt.Errorf("handler initialization incomplete - this is a programming error")
	}

	logger.Info("All handlers initialized successfully")
	return h, nil
}

// buildServerList processes Plex servers from config and builds the server list.
// Returns: servers, plexServerID (first server's ID)
func buildServerList(plexServers []config.PlexServer) ([]handlers.ServerInfo, string) {
	var servers []handlers.ServerInfo
	var plexServerID string

	for _, srv := range plexServers {
		// Skip shared servers (not owned by the user)
		// Shared servers cause 401 Unauthorized errors with direct connection URLs
		if srv.Owner == "Shared" || srv.Owner == "" {
			logger.Info("Skipping shared/unowned server", "server", srv.Name, "owner", srv.Owner)
			continue
		}

		// Collect all connection URLs, preferring non-docker addresses
		var urls []string
		var dockerURLs []string
		for _, conn := range srv.Connections {
			// Separate docker internal addresses (172.17.0.x) from regular addresses
			if strings.Contains(conn.URI, "172-17-0-") {
				dockerURLs = append(dockerURLs, conn.URI)
			} else {
				urls = append(urls, conn.URI)
			}
		}

		// If no non-docker URLs, use docker URLs as fallback
		if len(urls) == 0 && len(dockerURLs) > 0 {
			logger.Warn("Only docker internal addresses available for server, using as fallback", "server", srv.Name)
			urls = dockerURLs
		}

		if len(urls) > 0 {
			servers = append(servers, handlers.ServerInfo{
				ID:   srv.ID,
				Name: srv.Name,
				URLs: urls,
			})

			// Use first server's ID for playback handler
			if plexServerID == "" {
				plexServerID = srv.ID
			}
		}
	}

	return servers, plexServerID
}
