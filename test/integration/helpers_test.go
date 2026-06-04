//go:build integration

package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/auth"
	"github.com/sim4gh/nikte-cli/internal/config"
)

func TestMain(m *testing.M) {
	// Initialize config singleton
	if _, err := config.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// CI mode: seed config from environment
	if rt := os.Getenv("OIO_REFRESH_TOKEN"); rt != "" {
		config.Set("refresh_token", rt)
		config.Set("baseurl", "https://auth.yumaverse.com")
	}

	// Check if we have a refresh token at all
	cfg := config.Get()
	if cfg == nil || cfg.RefreshToken == "" {
		fmt.Println("SKIP: no refresh token available (run 'oio auth login' or set OIO_REFRESH_TOKEN)")
		os.Exit(0)
	}

	// Validate auth works by refreshing tokens
	if _, err := auth.RefreshTokens(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh tokens: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// ensureAuth refreshes tokens if needed and skips the test if auth is unavailable.
func ensureAuth(t *testing.T) {
	t.Helper()
	cfg := config.Get()
	if cfg == nil || cfg.RefreshToken == "" {
		t.Skip("no refresh token available")
	}
	if auth.IsTokenExpired(cfg.IDToken) {
		if _, err := auth.RefreshTokens(); err != nil {
			t.Fatalf("failed to refresh tokens: %v", err)
		}
	}
}

// createTestShort creates a short with a 5-minute TTL and registers cleanup to delete it.
// Returns the short ID.
func createTestShort(t *testing.T, content string) string {
	t.Helper()
	ensureAuth(t)

	resp, err := api.Post("/shorts", map[string]interface{}{
		"content": content,
		"ttl":     300, // 5 minutes
	})
	if err != nil {
		t.Fatalf("failed to create test short: %v", err)
	}
	assertStatus(t, resp, 201)

	id := resp.GetString("shortId")
	if id == "" {
		t.Fatal("created short has no shortId")
	}

	t.Cleanup(func() {
		api.Delete("/shorts/" + id)
	})

	return id
}

// assertStatus checks that the response has the expected status code.
func assertStatus(t *testing.T, resp *api.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected status %d, got %d (body: %s)", expected, resp.StatusCode, string(resp.Body))
	}
}
