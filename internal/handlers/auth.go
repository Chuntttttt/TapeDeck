// Package handlers provides HTTP request handlers for TapeDeck.
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

var debugLog *os.File

func init() {
	var err error
	debugLog, err = os.OpenFile("/tmp/tapedeck-auth-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to open debug log: %v", err)
	}
}

func logDebug(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	log.Print(msg)
	if debugLog != nil {
		debugLog.WriteString(time.Now().Format("2006/01/02 15:04:05") + " " + msg + "\n")
		debugLog.Sync()
	}
}

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	sessionStore *sessions.CookieStore
	plexAuth     plex.AuthClientInterface
	db           *db.DB
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(store *sessions.CookieStore, plexAuth plex.AuthClientInterface, database *db.DB) *AuthHandler {
	return &AuthHandler{
		sessionStore: store,
		plexAuth:     plexAuth,
		db:           database,
	}
}

// Login handles the GET /auth/login endpoint
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Check if already authenticated
	session := getOrCreateSession(h.sessionStore, r)
	if _, ok := middleware.GetUserID(session); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Check if we already have a PIN in the session (avoid generating multiple PINs)
	var pin *plex.PINResponse
	if existingPinID, ok := session.Values["plex_pin_id"].(int); ok {
		if existingPinCode, ok := session.Values["plex_pin_code"].(string); ok {
			// Reuse existing PIN from session
			pin = &plex.PINResponse{
				ID:   existingPinID,
				Code: existingPinCode,
			}
			log.Printf("DEBUG Login: Reusing existing PIN ID=%d, Code='%s'", pin.ID, pin.Code)
		}
	}

	// If no existing PIN, request a new one from Plex
	if pin == nil {
		var err error
		pin, err = h.plexAuth.RequestPIN()
		if err != nil {
			log.Printf("Failed to request PIN: %v", err)
			http.Error(w, "Failed to initiate Plex authentication", http.StatusInternalServerError)
			return
		}

		// Store PIN ID in session for callback
		session.Values["plex_pin_id"] = pin.ID
		session.Values["plex_pin_code"] = pin.Code
		if err := session.Save(r, w); err != nil {
			log.Printf("Failed to save session: %v", err)
			http.Error(w, "Session error", http.StatusInternalServerError)
			return
		}

		log.Printf("DEBUG Login: Created NEW PIN ID=%d, Code='%s', stored in session", pin.ID, pin.Code)
	}

	// Render login page with polling
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Login - TapeDeck</title>
    <style>
        body { font-family: sans-serif; max-width: 600px; margin: 50px auto; text-align: center; }
        .button { display: inline-block; padding: 15px 30px; background: #e5a00d; color: white;
                  text-decoration: none; border-radius: 5px; font-weight: bold; margin: 20px 0; }
        .button:hover { background: #cc8f0a; }
        .pin-code { font-size: 48px; font-weight: bold; margin: 30px 0; letter-spacing: 8px;
                    color: #e5a00d; }
        .status { color: #666; margin: 20px 0; font-size: 14px; }
        .success { color: #2d5016; }
        .error { color: #cc0000; }
    </style>
    <script>
        // NOTE: We use polling instead of the recommended forwardUrl redirect flow
        // because Plex broke OAuth for 3rd party apps in v4.152.0 (Sept 2025)
        // See: https://forums.plex.tv/t/plex-oauth-authenticate-with-plex-broken-after-plex-web-update-v4-152-0/931098
        // TODO: Switch back to forwardUrl when Plex fixes their OAuth implementation

        let pollInterval = 5000; // 5 seconds to avoid rate limiting
        let polling = true;

        function showRetryButton() {
            const status = document.getElementById('status');
            status.textContent = '';
            status.className = 'error';

            const retryBtn = document.createElement('button');
            retryBtn.textContent = 'Resume Checking';
            retryBtn.className = 'button';
            retryBtn.style.display = 'inline-block';
            retryBtn.style.margin = '10px 0';
            retryBtn.onclick = function() {
                status.textContent = 'Waiting for authorization...';
                status.className = 'status';
                retryBtn.remove();
                polling = true;
                pollInterval = 5000;
                setTimeout(checkAuth, 5000);
            };

            status.appendChild(document.createTextNode('Rate limited by Plex. Wait a moment and then: '));
            status.appendChild(document.createElement('br'));
            status.appendChild(retryBtn);
        }

        async function checkAuth() {
            if (!polling) return;

            console.log('[TapeDeck] Polling /auth/poll-status...');
            try {
                const response = await fetch('/auth/poll-status', {
                    credentials: 'same-origin'
                });

                console.log('[TapeDeck] Poll response status:', response.status);

                if (response.status === 429) {
                    // Rate limited - stop polling and show retry button
                    console.log('[TapeDeck] Rate limited (429) - stopping poll');
                    polling = false;
                    showRetryButton();
                    return;
                }

                if (response.ok) {
                    const data = await response.json();
                    console.log('[TapeDeck] Poll response data:', data);
                    if (data.authorized) {
                        console.log('[TapeDeck] ✅ AUTHORIZED! Redirecting to home...');
                        polling = false;
                        document.getElementById('status').textContent = 'Success! Redirecting...';
                        document.getElementById('status').className = 'success';
                        window.location.href = '/';
                        return;
                    } else {
                        console.log('[TapeDeck] Not authorized yet, will poll again in', pollInterval, 'ms');
                    }
                }

                // Continue polling
                setTimeout(checkAuth, pollInterval);
            } catch (e) {
                // On error, stop and show retry
                console.error('[TapeDeck] Poll error:', e);
                polling = false;
                showRetryButton();
            }
        }

        // Start polling after 5 seconds
        setTimeout(checkAuth, 5000);
    </script>
</head>
<body>
    <h1>🎬 TapeDeck</h1>
    <p>To connect your Plex account:</p>
    <ol style="text-align: left; max-width: 400px; margin: 20px auto;">
        <li>Click the button below to open plex.tv/link</li>
        <li>Enter this PIN code on that page</li>
        <li>This page will automatically detect when you're authorized</li>
    </ol>
    <div class="pin-code">%s</div>
    <a href="https://plex.tv/link" class="button" target="_blank">Open Plex Link Page</a>
    <p id="status" class="status">Waiting for authorization...</p>
</body>
</html>`, pin.Code)
}

// PollStatus handles the GET /auth/poll-status endpoint for JavaScript polling
//
// NOTE: This polling approach is a workaround because Plex broke OAuth for 3rd party apps
// in v4.152.0 (Sept 2025). The recommended forwardUrl redirect flow no longer works.
// We poll every 5 seconds to check if the user has authorized the PIN on plex.tv/link.
//
// See: https://forums.plex.tv/t/plex-oauth-authenticate-with-plex-broken-after-plex-web-update-v4-152-0/931098
// TODO: Switch back to forwardUrl redirect flow when Plex fixes their OAuth implementation
func (h *AuthHandler) PollStatus(w http.ResponseWriter, r *http.Request) {
	logDebug("DEBUG PollStatus: Received poll request from %s", r.RemoteAddr)

	session := getOrCreateSession(h.sessionStore, r)

	// Get PIN ID from session
	pinIDVal, ok := session.Values["plex_pin_id"]
	if !ok {
		logDebug("DEBUG PollStatus: No plex_pin_id in session")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	pinID, ok := pinIDVal.(int)
	if !ok {
		logDebug("DEBUG PollStatus: plex_pin_id type assertion failed, got type %T", pinIDVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	logDebug("DEBUG PollStatus: Checking PIN ID=%d", pinID)

	// Check PIN status to get auth token
	check, err := h.plexAuth.CheckPIN(pinID)
	if err != nil {
		// Don't log 429 rate limit errors - they're expected with polling
		if err.Error() != "unexpected status code: 429" {
			log.Printf("Failed to check PIN: %v", err)
		}
		// Return 429 status so client can back off
		if err.Error() == "unexpected status code: 429" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	authToken := check.AuthToken
	if authToken == "" {
		logDebug("DEBUG PollStatus: AuthToken still empty for PIN ID=%d, Code=%s", pinID, check.Code)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Token is ready! Create/update user and set session
	logDebug("✅ SUCCESS: Received auth token from Plex via polling! Token='%s' (len=%d)", authToken, len(authToken))
	plexUserID := "plex-user-" + authToken[:10]
	plexUsername := "PlexUser"

	user, err := h.db.GetUserByPlexUserID(plexUserID)
	if err != nil {
		// User doesn't exist, create new one
		user = models.NewUser(plexUsername, plexUserID, authToken)
		userID, err := h.db.CreateUser(user)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
			return
		}
		user.ID = userID
	} else {
		// Update existing user's token
		user.PlexAuthToken = authToken
		user.UpdatedAt = time.Now()
		if err := h.db.UpdateUser(user); err != nil {
			log.Printf("Failed to update user: %v", err)
		}
	}

	// Store user ID in session
	middleware.SetUserID(session, user.ID)
	delete(session.Values, "plex_pin_id")
	delete(session.Values, "plex_pin_code")

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	log.Printf("User authenticated successfully via polling")

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": true})
}

// Logout handles the POST /auth/logout endpoint
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	middleware.ClearSession(session)

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	// Check for redirect parameter
	redirectTo := r.URL.Query().Get("redirect")
	if redirectTo == "" {
		redirectTo = "/auth/login"
	}

	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// getOrCreateSession retrieves or creates a session
func getOrCreateSession(store *sessions.CookieStore, r *http.Request) *sessions.Session {
	session, _ := store.Get(r, middleware.SessionName)
	return session
}

// scheme returns the request scheme (http or https)
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
