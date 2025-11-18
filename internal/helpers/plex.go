// Package helpers provides utility functions for common operations across handlers.
package helpers

import (
	"context"

	"github.com/Chuntttttt/tapedeck/internal/constants"
)

// ServerInfo holds server connection information (generic version to avoid import cycles)
type ServerInfo struct {
	ID   string
	Name string
	URLs []string
}

// TryServerURLs attempts an operation across all URLs for a server until one succeeds.
// It returns the result of the first successful operation, or the last error if all fail.
//
// Type parameters:
//   - T: the return type of the operation
//   - Client: the type of client created by the factory
//
// Example usage:
//
//	libraries, err := TryServerURLs(ctx, server, authToken, devMode,
//	    func(client PlexClientInterface) ([]Library, error) {
//	        return client.GetLibraries(ctx)
//	    },
//	    newPlexClientFunc)
func TryServerURLs[T any, Client any](
	ctx context.Context,
	server ServerInfo,
	authToken string,
	devMode bool,
	operation func(Client) (T, error),
	clientFactory func(url, serverID, authToken string, devMode bool) Client,
) (T, error) {
	var result T
	var lastErr error

	for _, url := range server.URLs {
		// Check if context is already cancelled
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		default:
		}

		client := clientFactory(url, server.ID, authToken, devMode)

		// Add timeout for external API call
		apiCtx, cancel := context.WithTimeout(ctx, constants.PlexAPITimeout)
		_ = apiCtx // Will be used when we pass it to operation
		result, lastErr = operation(client)
		cancel()

		if lastErr == nil {
			// Success! Return result
			return result, nil
		}
		// Try next URL
	}

	// All URLs failed, return last error
	var zero T
	return zero, lastErr
}
