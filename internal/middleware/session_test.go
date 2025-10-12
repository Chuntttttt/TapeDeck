package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSessionStore(t *testing.T) {
	secret := []byte("test-secret-key-32-chars-long!!")

	store := NewSessionStore(secret)
	if store == nil {
		t.Fatal("NewSessionStore() returned nil")
	}
}

func TestGetSession(t *testing.T) {
	tests := []struct {
		name        string
		setupCookie bool
		wantErr     bool
	}{
		{
			name:        "new session without cookie",
			setupCookie: false,
			wantErr:     false,
		},
		{
			name:        "existing session with cookie",
			setupCookie: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()

			if tt.setupCookie {
				// Create a session first
				session, err := store.Get(req, SessionName)
				if err != nil {
					t.Fatalf("Failed to get initial session: %v", err)
				}
				session.Values["test"] = "value"
				_ = session.Save(req, w)

				// Add the cookie to the next request
				cookies := w.Result().Cookies()
				for _, cookie := range cookies {
					req.AddCookie(cookie)
				}
			}

			session, err := store.Get(req, SessionName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if session == nil {
				t.Error("Get() returned nil session")
			}
		})
	}
}

func TestSetGetUserID(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Get session
	session, err := store.Get(req, SessionName)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Set user ID
	userID := int64(12345)
	SetUserID(session, userID)

	// Save session
	if err := session.Save(req, w); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Get user ID
	retrievedID, ok := GetUserID(session)
	if !ok {
		t.Fatal("GetUserID() returned ok=false")
	}

	if retrievedID != userID {
		t.Errorf("GetUserID() = %d, want %d", retrievedID, userID)
	}
}

func TestGetUserID_NotSet(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	session, err := store.Get(req, SessionName)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	_, ok := GetUserID(session)
	if ok {
		t.Error("GetUserID() returned ok=true for empty session")
	}
}

func TestClearSession(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Create session with user ID
	session, err := store.Get(req, SessionName)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	SetUserID(session, 12345)
	if err := session.Save(req, w); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Clear session
	ClearSession(session)

	// Verify user ID is gone
	_, ok := GetUserID(session)
	if ok {
		t.Error("GetUserID() returned ok=true after ClearSession()")
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// Setup authenticated session
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	session, _ := store.Get(req, SessionName)
	SetUserID(session, 12345)
	_ = session.Save(req, w)

	// Add cookie to request
	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	// Test middleware
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAuth(store)(next)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !nextCalled {
		t.Error("RequireAuth() did not call next handler for authenticated user")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireAuth_NotAuthenticated(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"))

	// No session setup - unauthenticated
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	})

	handler := RequireAuth(store)(next)
	handler.ServeHTTP(w, req)

	if nextCalled {
		t.Error("RequireAuth() called next handler for unauthenticated user")
	}

	if w.Code != http.StatusFound {
		t.Errorf("Status = %d, want %d (redirect)", w.Code, http.StatusFound)
	}

	location := w.Header().Get("Location")
	if location != "/auth/login" {
		t.Errorf("Redirect location = %s, want /auth/login", location)
	}
}
