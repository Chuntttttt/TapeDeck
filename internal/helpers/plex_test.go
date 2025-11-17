package helpers

import (
	"context"
	"errors"
	"testing"
)

// mockClient is a simple mock for testing
type mockClient struct{}

func TestTryServerURLs_Success(t *testing.T) {
	server := ServerInfo{
		ID:   "server1",
		Name: "Test Server",
		URLs: []string{"http://localhost:32400", "http://192.168.1.100:32400"},
	}

	called := 0
	operation := func(client mockClient) (string, error) {
		called++
		if called == 1 {
			return "", errors.New("first URL failed")
		}
		return "success", nil
	}

	mockFactory := func(url, serverID, authToken string, devMode bool) mockClient {
		return mockClient{}
	}

	result, err := TryServerURLs(context.Background(), server, "token", false, operation, mockFactory)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got '%s'", result)
	}
	if called != 2 {
		t.Errorf("Expected 2 calls, got %d", called)
	}
}

func TestTryServerURLs_AllFail(t *testing.T) {
	server := ServerInfo{
		ID:   "server1",
		Name: "Test Server",
		URLs: []string{"http://localhost:32400", "http://192.168.1.100:32400"},
	}

	operation := func(client mockClient) (string, error) {
		return "", errors.New("connection failed")
	}

	mockFactory := func(url, serverID, authToken string, devMode bool) mockClient {
		return mockClient{}
	}

	_, err := TryServerURLs(context.Background(), server, "token", false, operation, mockFactory)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Error() != "connection failed" {
		t.Errorf("Expected 'connection failed', got '%s'", err.Error())
	}
}

func TestTryServerURLs_ContextTimeout(t *testing.T) {
	server := ServerInfo{
		ID:   "server1",
		Name: "Test Server",
		URLs: []string{"http://localhost:32400"},
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	operation := func(client mockClient) (string, error) {
		return "success", nil
	}

	mockFactory := func(url, serverID, authToken string, devMode bool) mockClient {
		return mockClient{}
	}

	_, err := TryServerURLs(ctx, server, "token", false, operation, mockFactory)

	if err == nil {
		t.Fatal("Expected context canceled error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}
