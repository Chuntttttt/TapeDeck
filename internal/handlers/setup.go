package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/ha"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/Chuntttttt/tapedeck/templates/pages"
	"github.com/gorilla/sessions"
)

// SetupHandler handles the first-time setup wizard
type SetupHandler struct {
	sessionStore    *sessions.CookieStore
	configPath      string
	plexAuth        *plex.AuthClient
	db              *db.DB
	devMode         bool
	onSetupComplete func() error // Callback to initialize handlers after setup
}

// SetupState tracks the wizard progress in the session
type SetupState struct {
	Step        int                 `json:"step"`
	PlexServers []config.PlexServer `json:"plex_servers"`
	HAConfig    config.HAConfig     `json:"ha_config"`
	AppleTVs    []config.AppleTV    `json:"apple_tvs"`
	SelectedTVs map[string]bool     `json:"selected_tvs"` // entity_id -> selected
}

// NewSetupHandler creates a new setup handler
func NewSetupHandler(store *sessions.CookieStore, configPath string, plexAuth *plex.AuthClient, database *db.DB, devMode bool, onSetupComplete func() error) *SetupHandler {
	return &SetupHandler{
		sessionStore:    store,
		configPath:      configPath,
		plexAuth:        plexAuth,
		db:              database,
		devMode:         devMode,
		onSetupComplete: onSetupComplete,
	}
}

// getSetupState retrieves setup state from session
func (h *SetupHandler) getSetupState(r *http.Request) (*SetupState, error) {
	session, _ := h.sessionStore.Get(r, middleware.SessionName)

	stateData, ok := session.Values["setup_state"].(string)
	if !ok {
		// Initialize new state
		return &SetupState{
			Step:        1,
			SelectedTVs: make(map[string]bool),
		}, nil
	}

	var state SetupState
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal setup state: %w", err)
	}

	return &state, nil
}

// saveSetupState saves setup state to session
func (h *SetupHandler) saveSetupState(w http.ResponseWriter, r *http.Request, state *SetupState) error {
	session, _ := h.sessionStore.Get(r, middleware.SessionName)

	stateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal setup state: %w", err)
	}

	session.Values["setup_state"] = string(stateData)
	return session.Save(r, w)
}

// clearSetupState removes setup state from session
func (h *SetupHandler) clearSetupState(w http.ResponseWriter, r *http.Request) error {
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	delete(session.Values, "setup_state")
	return session.Save(r, w)
}

// Step1Welcome handles GET /setup - Welcome page
func (h *SetupHandler) Step1Welcome(w http.ResponseWriter, r *http.Request) {
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to load setup wizard", http.StatusInternalServerError)
		return
	}

	state.Step = 1
	if err := h.saveSetupState(w, r, state); err != nil {
		log.Printf("Failed to save setup state: %v", err)
	}

	// Render using templ template
	if err := pages.SetupWelcome().Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// Step2Plex handles GET /setup/step/2 - Plex authentication
func (h *SetupHandler) Step2Plex(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessionStore.Get(r, middleware.SessionName)

	// Check if user is already authenticated with Plex
	userID, ok := middleware.GetUserID(session)
	if !ok {
		// Show Plex login page
		h.renderPlexLogin(w, r)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v (clearing invalid session)", err)
		// User doesn't exist (maybe database was reset) - clear session and show login
		middleware.ClearSession(session)
		_ = session.Save(r, w)
		h.renderPlexLogin(w, r)
		return
	}

	// User is authenticated, fetch servers
	servers, err := h.plexAuth.GetServers(user.PlexAuthToken)
	if err != nil {
		log.Printf("Failed to get Plex servers: %v", err)
		h.renderPlexError(w, r, "Failed to fetch Plex servers. Please try again.")
		return
	}

	if len(servers) == 0 {
		h.renderPlexError(w, r, "No Plex servers found. Make sure you have access to at least one Plex Media Server.")
		return
	}

	// Show server selection page
	h.renderServerSelection(w, r, servers)
}

func (h *SetupHandler) renderPlexLogin(w http.ResponseWriter, r *http.Request) {
	if err := pages.SetupPlexLogin().Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (h *SetupHandler) renderPlexError(w http.ResponseWriter, r *http.Request, errorMsg string) {
	if err := pages.SetupPlexError(errorMsg).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (h *SetupHandler) renderServerSelection(w http.ResponseWriter, r *http.Request, servers []config.PlexServer) {
	if err := pages.SetupServerSelection(servers).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// SavePlexServers handles POST /setup/plex/servers
func (h *SetupHandler) SavePlexServers(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	userID, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get user from database to retrieve auth token
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	// Get all servers
	allServers, err := h.plexAuth.GetServers(user.PlexAuthToken)
	if err != nil {
		log.Printf("Failed to get servers: %v", err)
		http.Error(w, "Failed to fetch servers", http.StatusInternalServerError)
		return
	}

	// Get selected server IDs
	selectedIDs := r.Form["server_ids"]
	if len(selectedIDs) == 0 {
		http.Error(w, "Please select at least one server", http.StatusBadRequest)
		return
	}

	// Filter to selected servers
	var selectedServers []config.PlexServer
	for _, server := range allServers {
		for _, selectedID := range selectedIDs {
			if server.ID == selectedID {
				selectedServers = append(selectedServers, server)
				break
			}
		}
	}

	// Save to setup state
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	state.PlexServers = selectedServers
	state.Step = 3

	if err := h.saveSetupState(w, r, state); err != nil {
		log.Printf("Failed to save setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/setup/ha", http.StatusFound)
}

// Step3HomeAssistant handles GET /setup/step/3 - Home Assistant configuration
func (h *SetupHandler) Step3HomeAssistant(w http.ResponseWriter, r *http.Request) {
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to load setup wizard", http.StatusInternalServerError)
		return
	}

	// Pre-fill with existing values if any
	haURL := state.HAConfig.URL
	haToken := state.HAConfig.Token

	if err := pages.SetupHomeAssistant(haURL, haToken).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// TestHomeAssistant handles POST /setup/ha/test
func (h *SetupHandler) TestHomeAssistant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HAURL   string `json:"ha_url"`
		HAToken string `json:"ha_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"success":false,"error":"Invalid request"}`)
		return
	}

	// Try to connect to HA
	haClient := ha.NewRestClient(req.HAURL, req.HAToken, h.devMode)

	// Test connection by getting states
	_, err := haClient.GetStates()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"success":false,"error":"Connection failed: %s"}`, html.EscapeString(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"success":true}`)
}

// SaveHomeAssistant handles POST /setup/ha/save
func (h *SetupHandler) SaveHomeAssistant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	haURL := r.FormValue("ha_url")
	haToken := r.FormValue("ha_token")

	if haURL == "" || haToken == "" {
		http.Error(w, "URL and token are required", http.StatusBadRequest)
		return
	}

	// Save to setup state
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	state.HAConfig = config.HAConfig{
		URL:   haURL,
		Token: haToken,
	}
	state.Step = 4

	if err := h.saveSetupState(w, r, state); err != nil {
		log.Printf("Failed to save setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/setup/appletv", http.StatusFound)
}

// Step4AppleTVs handles GET /setup/step/4 - Apple TV selection
func (h *SetupHandler) Step4AppleTVs(w http.ResponseWriter, r *http.Request) {
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to load setup wizard", http.StatusInternalServerError)
		return
	}

	// Get media players from HA
	haClient := ha.NewRestClient(state.HAConfig.URL, state.HAConfig.Token, h.devMode)
	mediaPlayers, err := haClient.GetMediaPlayers()

	if err != nil {
		log.Printf("Failed to get media players: %v", err)
		h.renderAppleTVError(w, r, "Failed to fetch media players from Home Assistant")
		return
	}

	if len(mediaPlayers) == 0 {
		h.renderAppleTVEmpty(w, r)
		return
	}

	h.renderAppleTVSelection(w, r, mediaPlayers, state)
}

func (h *SetupHandler) renderAppleTVSelection(w http.ResponseWriter, r *http.Request, mediaPlayers []ha.Entity, state *SetupState) {
	if err := pages.SetupAppleTVSelection(mediaPlayers, state.SelectedTVs).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (h *SetupHandler) renderAppleTVEmpty(w http.ResponseWriter, r *http.Request) {
	if err := pages.SetupAppleTVEmpty().Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

func (h *SetupHandler) renderAppleTVError(w http.ResponseWriter, r *http.Request, errorMsg string) {
	if err := pages.SetupAppleTVError(errorMsg).Render(r.Context(), w); err != nil {
		log.Printf("Failed to render template: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// SaveAppleTVs handles POST /setup/appletv/save
func (h *SetupHandler) SaveAppleTVs(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	// Get selected entity IDs
	selectedEntities := r.Form["tv_entities"]

	// Fetch media players again to get friendly names
	haClient := ha.NewRestClient(state.HAConfig.URL, state.HAConfig.Token, h.devMode)
	mediaPlayers, err := haClient.GetMediaPlayers()
	if err != nil {
		log.Printf("Failed to get media players: %v", err)
		http.Error(w, "Failed to fetch media players", http.StatusInternalServerError)
		return
	}

	// Build Apple TV list
	var appleTVs []config.AppleTV
	for _, entity := range selectedEntities {
		// Find friendly name
		friendlyName := entity
		for _, player := range mediaPlayers {
			if player.EntityID == entity {
				friendlyName = player.GetFriendlyName()
				break
			}
		}

		appleTVs = append(appleTVs, config.AppleTV{
			Entity:  entity,
			Name:    friendlyName,
			Default: len(appleTVs) == 0, // First one is default
		})
	}

	state.AppleTVs = appleTVs
	state.Step = 5

	if err := h.saveSetupState(w, r, state); err != nil {
		log.Printf("Failed to save setup state: %v", err)
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/setup/complete", http.StatusFound)
}

// Step5Complete handles GET /setup/step/5 - Completion summary
func (h *SetupHandler) Step5Complete(w http.ResponseWriter, r *http.Request) {
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to load setup wizard", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Complete Setup - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 40px 20px; background: #f5f5f5; }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .emoji { font-size: 48px; margin-bottom: 20px; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn:hover { background: #cc8f0a; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .progress { display: flex; gap: 10px; margin-bottom: 30px; }
        .progress-step { flex: 1; height: 4px; background: #ddd; border-radius: 2px; }
        .progress-step.active { background: #e5a00d; }
        .summary { background: #f5f5f5; padding: 20px; border-radius: 4px; margin: 20px 0; }
        .summary-item { padding: 10px 0; border-bottom: 1px solid #ddd; }
        .summary-item:last-child { border-bottom: none; }
        .summary-label { font-weight: bold; margin-bottom: 5px; }
        .summary-value { color: #666; }
        p { line-height: 1.6; color: #555; }
    </style>
</head>
<body>
    <div class="container">
        <div class="progress">
            <div class="progress-step active"></div>
            <div class="progress-step active"></div>
            <div class="progress-step active"></div>
            <div class="progress-step active"></div>
        </div>

        <a href="/setup/appletv" class="back-link">← Back</a>

        <div class="emoji">✓</div>
        <h1>Setup Complete!</h1>
        <p>Review your configuration summary below:</p>

        <div class="summary">
            <div class="summary-item">
                <div class="summary-label">Plex Servers</div>
                <div class="summary-value">%d server(s) connected</div>
            </div>
            <div class="summary-item">
                <div class="summary-label">Home Assistant</div>
                <div class="summary-value">Connected (%s)</div>
            </div>
            <div class="summary-item">
                <div class="summary-label">Apple TVs</div>
                <div class="summary-value">%d device(s) available</div>
            </div>
        </div>

        <p>You're ready to start pairing NFC cards with your media!</p>

        <form method="post" action="/setup/finish">
            <button type="submit" class="btn">Start Using TapeDeck</button>
        </form>
    </div>
</body>
</html>`, len(state.PlexServers), html.EscapeString(state.HAConfig.URL), len(state.AppleTVs))
}

// CompleteSetup handles POST /setup/complete - Finalize and save config
func (h *SetupHandler) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	state, err := h.getSetupState(r)
	if err != nil {
		log.Printf("Failed to get setup state: %v", err)
		http.Error(w, "Failed to load setup wizard", http.StatusInternalServerError)
		return
	}

	// Create runtime config
	runtimeConfig := &config.RuntimeConfig{
		Version:       1,
		PlexServers:   state.PlexServers,
		HomeAssistant: state.HAConfig,
		AppleTVs:      state.AppleTVs,
	}

	// Validate config
	if err := runtimeConfig.Validate(); err != nil {
		log.Printf("Invalid config: %v", err)
		http.Error(w, "Configuration is invalid: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Save to file
	if err := runtimeConfig.Save(h.configPath); err != nil {
		log.Printf("Failed to save config: %v", err)
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Printf("Setup completed successfully! Config saved to %s", h.configPath)

	// Initialize handlers now that config exists
	if h.onSetupComplete != nil {
		if err := h.onSetupComplete(); err != nil {
			log.Printf("Failed to initialize handlers: %v", err)
			http.Error(w, "Setup completed but failed to initialize handlers. Please restart the application.", http.StatusInternalServerError)
			return
		}
		log.Println("Handlers initialized successfully")
	}

	// Clear setup state from session
	if err := h.clearSetupState(w, r); err != nil {
		log.Printf("Failed to clear setup state: %v", err)
	}

	// Redirect to libraries page
	http.Redirect(w, r, "/libraries", http.StatusFound)
}
