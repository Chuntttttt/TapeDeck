package plex

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"
)

var integration = flag.Bool("integration", false, "run integration tests")

// TestIntegration_PlexAuth tests the PIN OAuth flow with a real Plex server
// Run with: go test -integration -v ./internal/plex -run TestIntegration_PlexAuth
func TestIntegration_PlexAuth(t *testing.T) {
	if !*integration {
		t.Skip("Skipping integration test. Use -integration flag to run.")
	}

	// Use test client ID (you should use a real one for production)
	clientID := "tapedeck-integration-test"
	productName := "TapeDeck"

	t.Log("Creating auth client...")
	authClient := NewAuthClient("https://plex.tv", clientID, productName, false)

	// Step 1: Request a PIN
	t.Log("Requesting PIN from Plex...")
	pin, err := authClient.RequestPIN()
	if err != nil {
		t.Fatalf("Failed to request PIN: %v", err)
	}

	t.Logf("PIN received: %s (ID: %d, expires in %d seconds)", pin.Code, pin.ID, pin.ExpiresIn)

	// Step 2: Show user the auth URL
	authURL := authClient.GetAuthURL(pin.Code, "")
	t.Logf("\n\nPlease visit this URL to authorize:\n%s\n", authURL)
	t.Log("\nAfter authorizing, waiting for auth token...")

	// Step 3: Poll for authorization (max 5 minutes)
	var authToken string
	maxAttempts := 60 // 5 minutes with 5-second intervals
	for i := 0; i < maxAttempts; i++ {
		time.Sleep(5 * time.Second)

		check, err := authClient.CheckPIN(pin.ID)
		if err != nil {
			t.Logf("Check attempt %d failed: %v", i+1, err)
			continue
		}

		if check.AuthToken != "" {
			authToken = check.AuthToken
			t.Logf("\n✓ Authorization successful! Token received.")
			break
		}

		t.Logf("Attempt %d/%d - waiting for authorization...", i+1, maxAttempts)
	}

	if authToken == "" {
		t.Fatal("Failed to get auth token within timeout period")
	}

	// Optionally print token (be careful with this in production)
	t.Logf("Auth Token (first 20 chars): %s...", authToken[:20])
}

// TestIntegration_PlexLibraries tests library browsing with a real Plex server
// Run with: go test -integration -v ./internal/plex -run TestIntegration_PlexLibraries
func TestIntegration_PlexLibraries(t *testing.T) {
	if !*integration {
		t.Skip("Skipping integration test. Use -integration flag to run.")
	}

	// Get server URL and token from environment
	serverURL := os.Getenv("PLEX_URL")
	authToken := os.Getenv("PLEX_AUTH_TOKEN")

	if serverURL == "" || authToken == "" {
		t.Skip("PLEX_URL and PLEX_AUTH_TOKEN environment variables required for this test")
	}

	t.Logf("Testing against server: %s", serverURL)

	client := NewClient(serverURL, authToken, false)

	// Test 1: Get Libraries
	t.Log("\n--- Testing GetLibraries ---")
	libraries, err := client.GetLibraries()
	if err != nil {
		t.Fatalf("GetLibraries() failed: %v", err)
	}

	t.Logf("Found %d libraries:", len(libraries))
	for i, lib := range libraries {
		t.Logf("  %d. %s (Type: %s, Key: %s)", i+1, lib.Title, lib.Type, lib.Key)
	}

	if len(libraries) == 0 {
		t.Log("No libraries found. This may be normal for a new server.")
		return
	}

	// Test 2: Get Library Contents (first library)
	firstLib := libraries[0]
	t.Logf("\n--- Testing GetLibraryContents for '%s' ---", firstLib.Title)
	items, err := client.GetLibraryContents(firstLib.Key)
	if err != nil {
		t.Fatalf("GetLibraryContents() failed: %v", err)
	}

	t.Logf("Found %d items in '%s':", len(items), firstLib.Title)
	for i, item := range items {
		if i >= 5 {
			t.Logf("  ... and %d more items", len(items)-5)
			break
		}
		year := ""
		if item.Year > 0 {
			year = fmt.Sprintf(" (%d)", item.Year)
		}
		t.Logf("  %d. %s%s [%s]", i+1, item.Title, year, item.Type)
	}

	// Test 3: Search
	if len(items) > 0 {
		// Use first word of first item's title as search term
		searchTerm := items[0].Title
		if len(searchTerm) > 10 {
			searchTerm = searchTerm[:10]
		}

		t.Logf("\n--- Testing Search with query: '%s' ---", searchTerm)
		results, err := client.Search(searchTerm)
		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		t.Logf("Found %d search results:", len(results))
		for i, result := range results {
			if i >= 5 {
				t.Logf("  ... and %d more results", len(results)-5)
				break
			}
			t.Logf("  %d. %s [%s]", i+1, result.Title, result.Type)
		}
	}
}
