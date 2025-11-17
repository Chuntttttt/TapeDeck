package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Chuntttttt/tapedeck/internal/models"
)

type mockDB struct {
	user *models.User
	err  error
}

func (m *mockDB) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	return m.user, m.err
}

func (m *mockDB) Close() error { return nil }

func TestWithUser_AddsUserToContext(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	mockDatabase := &mockDB{
		user: &models.User{
			ID:            123,
			PlexUserID:    "plex123",
			PlexUsername:  "testuser",
			PlexAuthToken: "token123",
		},
	}

	// Create a test handler that checks for user in context
	var capturedUser *models.User
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			t.Error("Expected user in context")
			return
		}
		capturedUser = user
		w.WriteHeader(http.StatusOK)
	})

	// Create request with session containing user ID
	req := httptest.NewRequest("GET", "/test", nil)
	session, _ := store.Get(req, SessionName)
	session.Values[UserIDKey] = int64(123)

	rr := httptest.NewRecorder()
	session.Save(req, rr)

	// Copy session cookie to new request
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Cookie", rr.Header().Get("Set-Cookie"))

	// Chain both middlewares: WithUserID then WithUser
	handler := WithUserID(store)(WithUser(store, mockDatabase)(testHandler))

	// Execute middleware chain
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr2.Code)
	}

	if capturedUser == nil {
		t.Fatal("User was not added to context")
	}

	if capturedUser.ID != 123 {
		t.Errorf("Expected user ID 123, got %d", capturedUser.ID)
	}
}

func TestWithUser_NoSessionContinues(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-key-32-chars-long!!"), false)
	mockDatabase := &mockDB{}

	middleware := WithUser(store, mockDatabase)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if ok {
			t.Error("Expected no user in context")
		}
		if user != nil {
			t.Error("Expected nil user")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware(testHandler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}
