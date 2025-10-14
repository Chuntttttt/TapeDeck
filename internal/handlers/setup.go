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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Connect Home Assistant - TapeDeck Setup</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 40px 20px; background: #f5f5f5; }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; font-weight: bold; }
        input[type="text"], input[type="password"] { padding: 10px; width: 100%%; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; margin-right: 10px; }
        .btn:hover { background: #cc8f0a; }
        .btn-secondary { background: #666; }
        .btn-secondary:hover { background: #555; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .progress { display: flex; gap: 10px; margin-bottom: 30px; }
        .progress-step { flex: 1; height: 4px; background: #ddd; border-radius: 2px; }
        .progress-step.active { background: #e5a00d; }
        .help-text { font-size: 14px; color: #666; margin-top: 5px; }
        .help-link { color: #e5a00d; }
        .status { padding: 15px; border-radius: 4px; margin: 20px 0; display: none; }
        .status.success { background: #dcfce7; border: 2px solid #22c55e; color: #166534; }
        .status.error { background: #fee2e2; border: 2px solid #ef4444; color: #991b1b; }
        .loading { display: inline-block; width: 16px; height: 16px; border: 2px solid #f3f3f3; border-top: 2px solid #e5a00d; border-radius: 50%%; animation: spin 1s linear infinite; margin-left: 10px; }
        @keyframes spin { 0%% { transform: rotate(0deg); } 100%% { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="container">
        <div class="progress">
            <div class="progress-step active"></div>
            <div class="progress-step active"></div>
            <div class="progress-step active"></div>
            <div class="progress-step"></div>
        </div>

        <a href="/setup/plex" class="back-link">← Back</a>
        <h1>Connect Home Assistant</h1>
        <p>Home Assistant controls playback on your Apple TV.</p>

        <div id="status" class="status"></div>

        <form method="post" action="/setup/ha/save" id="haForm">
            <div class="form-group">
                <label for="ha_url">Home Assistant URL *</label>
                <input type="text" id="ha_url" name="ha_url" value="%s" required placeholder="http://homeassistant.local:8123">
                <div class="help-text">Your Home Assistant instance URL</div>
            </div>

            <div class="form-group">
                <label for="ha_token">Long-Lived Access Token *</label>
                <input type="password" id="ha_token" name="ha_token" value="%s" required placeholder="Enter your HA token">
                <div class="help-text">
                    Create one in Home Assistant Profile settings
                    <a href="https://www.home-assistant.io/docs/authentication/#your-account-profile" target="_blank" class="help-link">How to create a token →</a>
                </div>
            </div>

            <button type="button" class="btn btn-secondary" id="testBtn">Test Connection</button>
            <button type="submit" class="btn" id="continueBtn" disabled>Continue</button>
        </form>
    </div>

    <script>
        const testBtn = document.getElementById('testBtn');
        const continueBtn = document.getElementById('continueBtn');
        const statusDiv = document.getElementById('status');
        const haURL = document.getElementById('ha_url');
        const haToken = document.getElementById('ha_token');

        testBtn.addEventListener('click', async function() {
            const url = haURL.value.trim();
            const token = haToken.value.trim();

            if (!url || !token) {
                showStatus('error', 'Please enter both URL and token');
                return;
            }

            testBtn.disabled = true;
            testBtn.innerHTML = 'Testing...<span class="loading"></span>';

            try {
                const response = await fetch('/setup/ha/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ha_url: url, ha_token: token })
                });

                const data = await response.json();

                if (response.ok && data.success) {
                    showStatus('success', '✓ Connected to Home Assistant successfully!');
                    continueBtn.disabled = false;
                } else {
                    showStatus('error', '✗ ' + (data.error || 'Connection failed'));
                    continueBtn.disabled = true;
                }
            } catch (error) {
                showStatus('error', '✗ Connection failed: ' + error.message);
                continueBtn.disabled = true;
            } finally {
                testBtn.disabled = false;
                testBtn.textContent = 'Test Connection';
            }
        });

        function showStatus(type, message) {
            statusDiv.className = 'status ' + type;
            statusDiv.textContent = message;
            statusDiv.style.display = 'block';
        }

        // If values already populated, enable continue button
        if (haURL.value && haToken.value) {
            continueBtn.disabled = false;
        }
    </script>
</body>
</html>`, html.EscapeString(haURL), html.EscapeString(haToken))
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

func (h *SetupHandler) renderAppleTVSelection(w http.ResponseWriter, _ *http.Request, mediaPlayers []ha.Entity, state *SetupState) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Select Apple TVs - TapeDeck Setup</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 40px 20px; background: #f5f5f5; }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; margin-right: 10px; }
        .btn:hover { background: #cc8f0a; }
        .btn-secondary { background: #666; }
        .btn-secondary:hover { background: #555; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .progress { display: flex; gap: 10px; margin-bottom: 30px; }
        .progress-step { flex: 1; height: 4px; background: #ddd; border-radius: 2px; }
        .progress-step.active { background: #e5a00d; }
        .tv-list { margin: 20px 0; }
        .tv-item { display: flex; align-items: flex-start; padding: 15px; border: 2px solid #ddd; border-radius: 4px; margin-bottom: 10px; cursor: pointer; }
        .tv-item:hover { border-color: #e5a00d; background: #fffbf0; }
        .tv-item input[type="checkbox"] { margin: 3px 12px 0 0; flex-shrink: 0; width: 18px; height: 18px; cursor: pointer; }
        .tv-item > div { flex: 1; }
        .tv-name { font-weight: bold; margin-bottom: 5px; }
        .tv-meta { font-size: 14px; color: #666; }
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

        <a href="/setup/ha" class="back-link">← Back</a>
        <h1>Select Apple TVs</h1>
        <p>We found %d media player(s) in Home Assistant. Select which ones you'd like to use with TapeDeck.</p>

        <form method="post" action="/setup/appletv/save">
            <div class="tv-list">`, len(mediaPlayers))

	for _, player := range mediaPlayers {
		friendlyName := player.GetFriendlyName()
		checked := ""
		if len(mediaPlayers) == 1 || state.SelectedTVs[player.EntityID] {
			checked = "checked"
		}

		_, _ = fmt.Fprintf(w, `
                <label class="tv-item">
                    <input type="checkbox" name="tv_entities" value="%s" %s>
                    <div>
                        <div class="tv-name">%s</div>
                        <div class="tv-meta">%s - %s</div>
                    </div>
                </label>`,
			html.EscapeString(player.EntityID),
			checked,
			html.EscapeString(friendlyName),
			html.EscapeString(player.EntityID),
			html.EscapeString(player.State))
	}

	_, _ = fmt.Fprint(w, `
            </div>

            <button type="submit" class="btn">Continue</button>
            <a href="/setup/complete" class="btn btn-secondary">Skip for Now</a>
        </form>
    </div>
</body>
</html>`)
}

func (h *SetupHandler) renderAppleTVEmpty(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>No Apple TVs Found - TapeDeck Setup</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 40px 20px; background: #f5f5f5; }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn:hover { background: #cc8f0a; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .progress { display: flex; gap: 10px; margin-bottom: 30px; }
        .progress-step { flex: 1; height: 4px; background: #ddd; border-radius: 2px; }
        .progress-step.active { background: #e5a00d; }
        .info { background: #fef3c7; border: 2px solid #f59e0b; padding: 15px; border-radius: 4px; color: #92400e; margin: 20px 0; }
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

        <a href="/setup/ha" class="back-link">← Back</a>
        <h1>No Apple TVs Found</h1>
        <div class="info">No media players were found in Home Assistant. You can add them later in settings.</div>
        <p>Make sure your Apple TV devices are configured in Home Assistant before continuing.</p>

        <a href="/setup/complete" class="btn">Skip for Now</a>
    </div>
</body>
</html>`)
}

func (h *SetupHandler) renderAppleTVError(w http.ResponseWriter, _ *http.Request, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Error - TapeDeck Setup</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 40px 20px; background: #f5f5f5; }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-top: 0; }
        .error { background: #fee2e2; border: 2px solid #ef4444; padding: 15px; border-radius: 4px; color: #991b1b; margin: 20px 0; }
        .btn { padding: 12px 24px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; margin-right: 10px; }
        .btn:hover { background: #cc8f0a; }
        .back-link { color: #e5a00d; text-decoration: none; margin-bottom: 20px; display: inline-block; }
        .progress { display: flex; gap: 10px; margin-bottom: 30px; }
        .progress-step { flex: 1; height: 4px; background: #ddd; border-radius: 2px; }
        .progress-step.active { background: #e5a00d; }
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

        <a href="/setup/ha" class="back-link">← Back</a>
        <h1>❌ Error</h1>
        <div class="error">%s</div>

        <a href="/setup/appletv" class="btn">Try Again</a>
        <a href="/setup/complete" class="btn">Skip for Now</a>
    </div>
</body>
</html>`, html.EscapeString(errorMsg))
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
