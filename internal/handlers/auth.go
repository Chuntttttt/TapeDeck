// Package handlers provides HTTP request handlers for TapeDeck.
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/db"
	"github.com/Chuntttttt/tapedeck/internal/middleware"
	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/Chuntttttt/tapedeck/internal/plex"
	"github.com/gorilla/sessions"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	sessionStore *sessions.CookieStore
	plexAuth     *plex.AuthClient
	db           *db.DB
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(store *sessions.CookieStore, plexAuth *plex.AuthClient, database *db.DB) *AuthHandler {
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

	// Request a PIN from Plex
	pin, err := h.plexAuth.RequestPIN()
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

	// Get Plex auth URL
	// Note: Plex requires a forwardUrl but won't actually redirect to localhost
	// We use polling instead to detect authorization
	callbackURL := fmt.Sprintf("%s://%s/auth/callback", scheme(r), r.Host)
	authURL := h.plexAuth.GetAuthURL(pin.Code, callbackURL)

	// Render login page with auth URL
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
        .pin-code { font-size: 24px; font-weight: bold; margin: 20px 0; letter-spacing: 4px; }
        .status { color: #666; margin: 20px 0; }
        .success { color: #2d5016; }
    </style>
</head>
<body>
    <h1>🎬 TapeDeck</h1>
    <p>Connect your Plex account to get started</p>
    <div class="pin-code">%s</div>
    <a href="#" class="button" id="loginBtn" onclick="openPlexAuth(); return false;">Login with Plex</a>
    <p><small>Or visit <a href="https://plex.tv/link">plex.tv/link</a> and enter the code above</small></p>
    <div class="status" id="status">Click the button above to authorize with Plex</div>
    <script>
        const authUrl = '%s';
        const pinId = %d;

        function openPlexAuth() {
            const width = 600;
            const height = 700;
            const left = (screen.width / 2) - (width / 2);
            const top = (screen.height / 2) - (height / 2);

            const popup = window.open(
                authUrl,
                'PlexAuth',
                'width=' + width + ',height=' + height + ',top=' + top + ',left=' + left
            );

            document.getElementById('status').textContent = 'Waiting for authorization in popup...';

            // Check if popup was blocked
            if (!popup || popup.closed || typeof popup.closed == 'undefined') {
                document.getElementById('status').textContent = 'Popup blocked! Please allow popups and try again.';
                return;
            }
        }

        // Listen for messages from Plex popup
        let plexAuthToken = null;

        window.addEventListener('message', function(event) {
            // Verify message is from Plex
            if (event.origin !== 'https://app.plex.tv') {
                return;
            }

            console.log('Received message from Plex:', event.data);

            // Capture auth token from Plex messages
            if (event.data && event.data.type === 'SET_VALUE_ON_WINDOW') {
                if (event.data.name === 'PLEX_USER_UUID') {
                    plexAuthToken = event.data.value;
                    console.log('Captured Plex auth token:', plexAuthToken);
                }
            }

            // Check for successful auth
            if (event.data && event.data.type === 'PUSH_GOOGLE_TAG_MANAGER_DATA') {
                const data = event.data.data || {};
                if (data.event === 'SignInSuccess') {
                    document.getElementById('status').textContent = '✓ Authorized! Completing sign in...';
                    document.getElementById('status').className = 'status success';

                    if (!plexAuthToken) {
                        document.getElementById('status').textContent = 'Error: No auth token received from Plex';
                        return;
                    }

                    // Submit auth token to callback endpoint
                    fetch('/auth/callback', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                        },
                        credentials: 'same-origin',
                        body: JSON.stringify({
                            authToken: plexAuthToken
                        })
                    }).then(function(response) {
                        if (response.ok) {
                            window.location.href = '/';
                        } else {
                            response.text().then(function(text) {
                                document.getElementById('status').textContent = 'Error: ' + text;
                            });
                        }
                    }).catch(function(err) {
                        document.getElementById('status').textContent = 'Error completing sign in';
                        console.error(err);
                    });
                }
            }
        }, false);
    </script>
</body>
</html>`, pin.Code, authURL, pin.ID)
}

// PollStatus handles the GET /auth/poll-status endpoint for JavaScript polling
func (h *AuthHandler) PollStatus(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	// Get PIN ID from session
	pinIDVal, ok := session.Values["plex_pin_id"]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	pinID, ok := pinIDVal.(int)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Check PIN status
	check, err := h.plexAuth.CheckPIN(pinID)
	if err != nil {
		// Don't log 429 rate limit errors - they're expected with polling
		if err.Error() != "unexpected status code: 429" {
			log.Printf("Failed to check PIN: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Debug logging to see what Plex returns
	log.Printf("DEBUG: CheckPIN response - ID: %d, Code: %s, AuthToken: %s", check.ID, check.Code, check.AuthToken)

	// If not authorized yet, return false
	if check.AuthToken == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
		return
	}

	// Authorized! Create/update user and set session
	authToken := check.AuthToken
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

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"authorized": true})
}

// Callback handles both GET and POST /auth/callback endpoint after Plex authorization
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// GET request is from Plex redirect in popup - show success page
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Authentication Successful</title>
    <style>
        body { font-family: sans-serif; max-width: 600px; margin: 50px auto; text-align: center; }
        .success { color: #2d5016; font-size: 24px; margin: 20px 0; }
    </style>
</head>
<body>
    <h1>🎬 TapeDeck</h1>
    <div class="success">✓ Authentication Successful!</div>
    <p>You can close this window and return to TapeDeck.</p>
    <script>
        // Try to close the popup
        setTimeout(function() {
            window.close();
        }, 1000);
    </script>
</body>
</html>`)
		return
	}

	// POST request is from our JavaScript with auth token
	session := getOrCreateSession(h.sessionStore, r)

	// Parse auth token from request body
	var req struct {
		AuthToken string `json:"authToken"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("JSON decode error: %v", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	authToken := req.AuthToken
	if authToken == "" {
		log.Printf("Auth token is empty after decode")
		http.Error(w, "No auth token provided", http.StatusBadRequest)
		return
	}

	log.Printf("Received auth token from Plex: %s", authToken)

	// Get user info from Plex using the auth token
	// For now, we'll use a placeholder - in a full implementation,
	// you'd call Plex API to get user details
	plexUserID := "plex-user-" + authToken[:10]
	plexUsername := "PlexUser"

	// Get or create user in database
	user, err := h.db.GetUserByPlexUserID(plexUserID)
	if err != nil {
		// User doesn't exist, create new one
		user = models.NewUser(plexUsername, plexUserID, authToken)
		userID, err := h.db.CreateUser(user)
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			http.Error(w, "Failed to create user account", http.StatusInternalServerError)
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
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	// Return JSON success (JavaScript will handle navigation)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// Logout handles the POST /auth/logout endpoint
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session := getOrCreateSession(h.sessionStore, r)

	middleware.ClearSession(session)

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session: %v", err)
	}

	http.Redirect(w, r, "/auth/login", http.StatusFound)
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
