//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/crypto"
)

// accessShare performs an unauthenticated public GET of a share (as a real visitor
// would, with a non-bot User-Agent so the view is counted) and returns the status.
func accessShare(t *testing.T, shareID string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, api.DefaultBaseURL+"/share/"+shareID, nil)
	if err != nil {
		t.Fatalf("build share request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nikte-integration/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("share access failed: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// createPublicShare shares a short publicly (optionally with maxViews) and returns the shareId.
func createPublicShare(t *testing.T, shortID string, maxViews int) string {
	t.Helper()
	body := map[string]interface{}{"isPublic": true}
	if maxViews > 0 {
		body["maxViews"] = maxViews
	}
	resp, err := api.Post("/shorts/"+shortID+"/share", body)
	if err != nil {
		t.Fatalf("create share failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("create share: expected 200/201, got %d (%s)", resp.StatusCode, string(resp.Body))
	}
	shareID := resp.GetString("shareId")
	if shareID == "" {
		t.Fatal("share has no shareId")
	}
	return shareID
}

// TestEncryptedShortRoundTrip verifies the server only ever stores ciphertext and
// that it round-trips: the CLI encrypts, the backend stores it verbatim, and the
// CLI can decrypt the retrieved blob.
func TestEncryptedShortRoundTrip(t *testing.T) {
	ensureAuth(t)
	plaintext := fmt.Sprintf("integration secret %d", time.Now().UnixNano())
	pass := "integration-passphrase"

	blob, err := crypto.EncryptText(pass, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !crypto.IsEncryptedText(blob) {
		t.Fatal("encrypted blob not recognized")
	}

	resp, err := api.Post("/shorts", map[string]interface{}{"content": blob, "ttl": 300})
	if err != nil {
		t.Fatalf("create encrypted short: %v", err)
	}
	assertStatus(t, resp, 201)
	id := resp.GetString("shortId")
	t.Cleanup(func() { api.Delete("/shorts/" + id) })

	got, err := api.Get("/shorts/" + id)
	if err != nil {
		t.Fatalf("get short: %v", err)
	}
	assertStatus(t, got, 200)

	stored := got.GetString("content")
	if stored != blob {
		t.Fatalf("server did not store ciphertext verbatim:\n stored: %q\n want:   %q", stored, blob)
	}
	dec, err := crypto.DecryptText(pass, stored)
	if err != nil {
		t.Fatalf("decrypt retrieved blob: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("round-trip mismatch: got %q want %q", dec, plaintext)
	}
}

// TestBurnAfterRead verifies a maxViews=1 share serves once, then burns: the link
// 404s and the underlying short is destroyed.
func TestBurnAfterRead(t *testing.T) {
	ensureAuth(t)
	id := createTestShort(t, fmt.Sprintf("burn me %d", time.Now().UnixNano()))
	shareID := createPublicShare(t, id, 1)

	if code := accessShare(t, shareID); code != 200 {
		t.Fatalf("first public access: expected 200, got %d", code)
	}
	// Allow the burn (delete) to settle.
	time.Sleep(1500 * time.Millisecond)

	if code := accessShare(t, shareID); code != 404 {
		t.Fatalf("second public access: expected 404 (burned), got %d", code)
	}
	// Underlying short must be gone.
	resp, err := api.Get("/shorts/" + id)
	if err != nil {
		t.Fatalf("get burned short: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("underlying short should be deleted after burn, got %d", resp.StatusCode)
	}
}

// TestShareViewAnalytics verifies public views increment viewCount, surfaced by GET /shares.
func TestShareViewAnalytics(t *testing.T) {
	ensureAuth(t)
	id := createTestShort(t, fmt.Sprintf("analytics %d", time.Now().UnixNano()))
	shareID := createPublicShare(t, id, 5)
	t.Cleanup(func() { api.Delete("/shorts/" + id) })

	accessShare(t, shareID)
	accessShare(t, shareID)
	time.Sleep(1 * time.Second)

	resp, err := api.Get("/shares")
	if err != nil {
		t.Fatalf("list shares: %v", err)
	}
	assertStatus(t, resp, 200)

	var result struct {
		Shares []struct {
			ShareID   string `json:"shareId"`
			ViewCount int    `json:"viewCount"`
			MaxViews  *int   `json:"maxViews"`
		} `json:"shares"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		t.Fatalf("unmarshal shares: %v", err)
	}

	var found bool
	for _, s := range result.Shares {
		if s.ShareID == shareID {
			found = true
			if s.ViewCount != 2 {
				t.Fatalf("expected viewCount 2, got %d", s.ViewCount)
			}
			if s.MaxViews == nil || *s.MaxViews != 5 {
				t.Fatalf("expected maxViews 5, got %v", s.MaxViews)
			}
		}
	}
	if !found {
		t.Fatalf("share %s not found in GET /shares", shareID)
	}
}
