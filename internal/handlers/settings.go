package handlers

import (
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/Chuntttttt/tapedeck/internal/config"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/gorilla/sessions"
)

// SettingsHandler handles settings/admin requests
type SettingsHandler struct {
	sessionStore   *sessions.CookieStore
	configPath     string
	reloadHandlers func() error
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(store *sessions.CookieStore, configPath string, reloadHandlers func() error) *SettingsHandler {
	return &SettingsHandler{
		sessionStore:   store,
		configPath:     configPath,
		reloadHandlers: reloadHandlers,
	}
}

// Settings handles GET /settings
func (h *SettingsHandler) Settings(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Load current config
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Render HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Settings - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 1200px; margin: 0 auto; padding: 20px; padding-top: 80px; }
        .header { margin-bottom: 30px; }
        .section { background: white; padding: 25px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 25px; }
        .section h2 { margin-top: 0; color: #333; border-bottom: 2px solid #e5a00d; padding-bottom: 10px; }
        .server-list { margin-top: 15px; }
        .server-item { display: flex; align-items: center; padding: 12px; border: 1px solid #ddd; border-radius: 4px; margin-bottom: 10px; background: #f9f9f9; }
        .server-item input[type="checkbox"] { margin-right: 12px; width: 20px; height: 20px; }
        .server-name { font-weight: bold; margin-right: 10px; }
        .server-owner { color: #666; font-size: 14px; margin-left: auto; }
        .server-owner.shared { color: #f59e0b; }
        .server-disabled { opacity: 0.5; }
        .btn { padding: 10px 20px; font-size: 16px; background: #e5a00d; color: white; border: none; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn:hover { background: #cc8f0a; }
        .btn-secondary { background: #6b7280; }
        .btn-secondary:hover { background: #4b5563; }
        .info-box { background: #fef3c7; border-left: 4px solid #f59e0b; padding: 12px 16px; margin: 15px 0; border-radius: 4px; }
        .info-box p { margin: 0; color: #92400e; }
        .actions { margin-top: 20px; display: flex; gap: 10px; }
        .form-group { margin-bottom: 20px; }
        .form-group label { display: block; margin-bottom: 5px; font-weight: bold; }
        .form-group input[type="url"], .form-group input[type="password"] { padding: 10px; width: 100%%; font-size: 16px; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box; }
        .form-group input[type="url"]:focus, .form-group input[type="password"]:focus { border-color: #e5a00d; outline: none; }
    </style>
</head>
<body>
%s
%s
    <div class="header">
        <h1>⚙️ Settings</h1>
    </div>

    <form method="post" action="/settings/servers">
        <div class="section">
            <h2>Plex Servers</h2>
            <div class="server-list">
`, NavigationHTML(), ConnectionBannerHTML())

	if len(runtimeCfg.PlexServers) == 0 {
		_, _ = fmt.Fprint(w, `                <p>No Plex servers configured.</p>`)
	} else {
		for _, srv := range runtimeCfg.PlexServers {
			disabled := ""
			disabledClass := ""
			if srv.Owner == "Shared" {
				disabled = " disabled"
				disabledClass = " server-disabled"
			}

			ownerClass := ""
			ownerLabel := srv.Owner
			if srv.Owner == "Shared" {
				ownerClass = " shared"
				ownerLabel = "Shared (not supported)"
			}

			_, _ = fmt.Fprintf(w, `
                <div class="server-item%s">
                    <input type="checkbox" name="servers" value="%s" checked%s>
                    <span class="server-name">%s</span>
                    <span class="server-owner%s">%s</span>
                </div>
`, disabledClass, html.EscapeString(srv.ID), disabled, html.EscapeString(srv.Name), ownerClass, html.EscapeString(ownerLabel))
		}
	}

	_, _ = fmt.Fprintf(w, `
            </div>
            <div class="info-box">
                <p><strong>Note:</strong> Shared servers are currently not supported due to Plex API limitations. Uncheck servers you don't want to use.</p>
            </div>
        </div>

        <div class="section">
            <h2>Home Assistant</h2>
            <div class="form-group">
                <label for="ha_url">URL</label>
                <input type="url" id="ha_url" name="ha_url" value="%s" placeholder="http://homeassistant.local:8123">
            </div>
            <div class="form-group">
                <label for="ha_token">Long-Lived Access Token</label>
                <input type="password" id="ha_token" name="ha_token" value="%s" placeholder="Enter token...">
            </div>
            <div class="form-group">
                <button type="button" id="testHABtn" class="btn btn-secondary">Test Connection</button>
                <span id="haStatus" style="margin-left: 15px;">Click to test</span>
            </div>
        </div>

        <div class="section">
            <h2>Apple TVs</h2>
`, html.EscapeString(runtimeCfg.HomeAssistant.URL), html.EscapeString(runtimeCfg.HomeAssistant.Token))

	if len(runtimeCfg.AppleTVs) == 0 {
		_, _ = fmt.Fprint(w, `            <p>No Apple TVs configured.</p>`)
	} else {
		for _, tv := range runtimeCfg.AppleTVs {
			defaultBadge := ""
			if tv.Default {
				defaultBadge = " <span style=\"background: #22c55e; color: white; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: bold;\">DEFAULT</span>"
			}
			_, _ = fmt.Fprintf(w, `            <p>• %s%s</p>
`, html.EscapeString(tv.Name), defaultBadge)
		}
	}

	_, _ = fmt.Fprintf(w, `
        </div>

        <div class="actions">
            <button type="submit" class="btn">Save Changes</button>
            <a href="/setup" class="btn btn-secondary">Re-run Setup Wizard</a>
            <a href="/metrics" class="btn btn-secondary" target="_blank">View Metrics</a>
        </div>
    </form>

    <script>
        // Test HA connection
        document.getElementById('testHABtn').addEventListener('click', function() {
            const statusEl = document.getElementById('haStatus');
            statusEl.textContent = 'Testing...';
            statusEl.style.color = '#666';

            const url = document.getElementById('ha_url').value;
            const token = document.getElementById('ha_token').value;

            if (!url || !token) {
                statusEl.textContent = '✗ Please fill in URL and token';
                statusEl.style.color = '#ef4444';
                return;
            }

            fetch('/setup/ha/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: 'ha_url=' + encodeURIComponent(url) + '&ha_token=' + encodeURIComponent(token)
            })
            .then(res => res.json())
            .then(data => {
                if (data.success) {
                    statusEl.textContent = '✓ Connection successful';
                    statusEl.style.color = '#22c55e';
                } else {
                    statusEl.textContent = '✗ ' + (data.error || 'Connection failed');
                    statusEl.style.color = '#ef4444';
                }
            })
            .catch(err => {
                console.error('Test failed:', err);
                statusEl.textContent = '✗ Test failed';
                statusEl.style.color = '#ef4444';
            });
        });

        // Check current HA connection status on load
        fetch('/api/status/ha')
            .then(res => res.json())
            .then(data => {
                const statusEl = document.getElementById('haStatus');
                if (data.connected) {
                    statusEl.textContent = '✓ Currently connected';
                    statusEl.style.color = '#22c55e';
                } else {
                    statusEl.textContent = '✗ Currently disconnected';
                    statusEl.style.color = '#ef4444';
                }
            })
            .catch(err => {
                console.error('Failed to check HA status:', err);
            });
    </script>
%s
</body>
</html>`, ConnectionBannerScript())
}

// SaveServers handles POST /settings/servers
func (h *SettingsHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	session, _ := h.sessionStore.Get(r, middleware.SessionName)
	_, ok := middleware.GetUserID(session)
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get selected server IDs
	selectedIDs := r.Form["servers"]
	selectedMap := make(map[string]bool)
	for _, id := range selectedIDs {
		selectedMap[id] = true
	}

	// Load current config
	runtimeCfg, err := config.LoadRuntimeConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Update Home Assistant settings
	haURL := r.FormValue("ha_url")
	haToken := r.FormValue("ha_token")
	haChanged := false
	if haURL != "" && haToken != "" {
		if haURL != runtimeCfg.HomeAssistant.URL || haToken != runtimeCfg.HomeAssistant.Token {
			runtimeCfg.HomeAssistant.URL = haURL
			runtimeCfg.HomeAssistant.Token = haToken
			haChanged = true
			log.Printf("Updated Home Assistant settings: %s", haURL)
		}
	}

	// Filter servers to only include selected ones
	var filteredServers []config.PlexServer
	serversChanged := false
	for _, srv := range runtimeCfg.PlexServers {
		if selectedMap[srv.ID] {
			filteredServers = append(filteredServers, srv)
		} else {
			log.Printf("Removing server '%s' from configuration", srv.Name)
			serversChanged = true
		}
	}

	runtimeCfg.PlexServers = filteredServers

	// Save updated config
	if err := runtimeCfg.Save(h.configPath); err != nil {
		log.Printf("Failed to save configuration: %v", err)
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	log.Printf("Configuration updated: %d server(s) enabled", len(filteredServers))

	// Reload handlers if HA settings or servers changed
	if (haChanged || serversChanged) && h.reloadHandlers != nil {
		log.Println("Reloading handlers with new configuration...")
		if err := h.reloadHandlers(); err != nil {
			log.Printf("Warning: Failed to reload handlers: %v", err)
			// Don't fail the request - config was saved successfully
		}
	}

	// Redirect back to settings
	http.Redirect(w, r, "/settings?saved=true", http.StatusSeeOther)
}
