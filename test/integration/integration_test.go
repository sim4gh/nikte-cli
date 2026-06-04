//go:build integration

package integration

import (
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sim4gh/nikte-cli/internal/api"
)

func TestHealthEndpoint(t *testing.T) {
	resp, err := api.GetNoAuth("/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	assertStatus(t, resp, 200)
}

func TestShortsCRUDLifecycle(t *testing.T) {
	ensureAuth(t)
	content := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())

	var shortID string

	t.Run("Create", func(t *testing.T) {
		resp, err := api.Post("/shorts", map[string]interface{}{
			"content": content,
			"ttl":     300,
		})
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		assertStatus(t, resp, 201)

		shortID = resp.GetString("shortId")
		if shortID == "" {
			t.Fatal("shortId is empty")
		}
		t.Logf("created short: %s", shortID)
	})

	if shortID == "" {
		t.Fatal("create subtest failed, cannot continue")
	}

	// Ensure cleanup even if later subtests fail
	t.Cleanup(func() {
		api.Delete("/shorts/" + shortID)
	})

	t.Run("Get", func(t *testing.T) {
		resp, err := api.Get("/shorts/" + shortID)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		assertStatus(t, resp, 200)

		got := resp.GetString("content")
		if got != content {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := api.Get("/shorts")
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assertStatus(t, resp, 200)

		var result struct {
			Shorts []struct {
				ShortID string `json:"shortId"`
			} `json:"shorts"`
		}
		if err := resp.Unmarshal(&result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		found := false
		for _, s := range result.Shorts {
			if s.ShortID == shortID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("short %s not found in list", shortID)
		}
	})

	t.Run("Extend", func(t *testing.T) {
		resp, err := api.Patch("/shorts/"+shortID, map[string]interface{}{
			"ttl": "5m",
		})
		if err != nil {
			t.Fatalf("extend failed: %v", err)
		}
		assertStatus(t, resp, 200)
	})

	t.Run("MakePermanent", func(t *testing.T) {
		resp, err := api.Patch("/shorts/"+shortID, map[string]interface{}{
			"permanent": true,
		})
		if err != nil {
			t.Fatalf("make permanent failed: %v", err)
		}
		assertStatus(t, resp, 200)
	})

	t.Run("Delete", func(t *testing.T) {
		resp, err := api.Delete("/shorts/" + shortID)
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}
		if resp.StatusCode != 200 && resp.StatusCode != 204 {
			t.Fatalf("expected 200 or 204, got %d", resp.StatusCode)
		}
	})

	t.Run("Verify404", func(t *testing.T) {
		resp, err := api.Get("/shorts/" + shortID)
		if err != nil {
			t.Fatalf("get after delete failed: %v", err)
		}
		assertStatus(t, resp, 404)
	})
}

func TestShortsCreateValidation(t *testing.T) {
	ensureAuth(t)

	resp, err := api.Post("/shorts", map[string]interface{}{
		"content": "",
		"ttl":     300,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for empty content, got %d", resp.StatusCode)
	}
}

func TestShortsGetNotFound(t *testing.T) {
	ensureAuth(t)

	resp, err := api.Get("/shorts/zzzz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

func TestShortsDeleteIdempotent(t *testing.T) {
	ensureAuth(t)

	id := createTestShort(t, fmt.Sprintf("delete-idem-%d", time.Now().UnixNano()))

	// First delete should succeed
	resp, err := api.Delete("/shorts/" + id)
	if err != nil {
		t.Fatalf("first delete failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		t.Fatalf("expected 200 or 204, got %d", resp.StatusCode)
	}

	// Second delete should return 404
	resp, err = api.Delete("/shorts/" + id)
	if err != nil {
		t.Fatalf("second delete failed: %v", err)
	}
	assertStatus(t, resp, 404)
}

func TestScreenshotCRUD(t *testing.T) {
	ensureAuth(t)

	// 1x1 red PNG
	pngBytes := minimalPNG()
	b64Data := base64.StdEncoding.EncodeToString(pngBytes)

	var screenshotID string

	t.Run("Create", func(t *testing.T) {
		resp, err := api.Post("/screenshots", map[string]interface{}{
			"contentType": "image/png",
			"data":        b64Data,
			"ttl":         "5m",
		})
		if err != nil {
			t.Fatalf("create screenshot failed: %v", err)
		}
		assertStatus(t, resp, 201)

		screenshotID = resp.GetString("screenshotId")
		if screenshotID == "" {
			t.Fatal("screenshotId is empty")
		}
		t.Logf("created screenshot: %s", screenshotID)
	})

	if screenshotID == "" {
		t.Fatal("create subtest failed, cannot continue")
	}

	t.Cleanup(func() {
		api.Delete("/screenshots/" + screenshotID)
	})

	t.Run("Get", func(t *testing.T) {
		resp, err := api.Get("/screenshots/" + screenshotID)
		if err != nil {
			t.Fatalf("get screenshot failed: %v", err)
		}
		assertStatus(t, resp, 200)

		url := resp.GetString("downloadUrl")
		if url == "" {
			t.Error("downloadUrl is empty")
		}
	})

	t.Run("List", func(t *testing.T) {
		resp, err := api.Get("/screenshots")
		if err != nil {
			t.Fatalf("list screenshots failed: %v", err)
		}
		assertStatus(t, resp, 200)

		var result struct {
			Screenshots []struct {
				ScreenshotID string `json:"screenshotId"`
			} `json:"screenshots"`
		}
		if err := resp.Unmarshal(&result); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		found := false
		for _, sc := range result.Screenshots {
			if sc.ScreenshotID == screenshotID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("screenshot %s not found in list", screenshotID)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		resp, err := api.Delete("/screenshots/" + screenshotID)
		if err != nil {
			t.Fatalf("delete screenshot failed: %v", err)
		}
		if resp.StatusCode != 200 && resp.StatusCode != 204 {
			t.Fatalf("expected 200 or 204, got %d (body: %s)", resp.StatusCode, string(resp.Body))
		}
	})
}

func TestConcurrentOperations(t *testing.T) {
	ensureAuth(t)

	const n = 3
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content := fmt.Sprintf("concurrent-%d-%d", idx, time.Now().UnixNano())
			resp, err := api.Post("/shorts", map[string]interface{}{
				"content": content,
				"ttl":     300,
			})
			if err != nil {
				errs[idx] = err
				return
			}
			if resp.StatusCode != 201 {
				errs[idx] = fmt.Errorf("expected 201, got %d", resp.StatusCode)
				return
			}
			ids[idx] = resp.GetString("shortId")
		}(i)
	}
	wg.Wait()

	// Cleanup all created shorts
	t.Cleanup(func() {
		for _, id := range ids {
			if id != "" {
				api.Delete("/shorts/" + id)
			}
		}
	})

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d failed: %v", i, errs[i])
		}
		if ids[i] == "" {
			t.Errorf("goroutine %d: no shortId returned", i)
		}
	}

	// Verify all are retrievable
	for i, id := range ids {
		if id == "" {
			continue
		}
		resp, err := api.Get("/shorts/" + id)
		if err != nil {
			t.Errorf("get short %d (%s) failed: %v", i, id, err)
			continue
		}
		assertStatus(t, resp, 200)
	}
}

// minimalPNG returns a valid 1x1 red PNG image (67 bytes).
func minimalPNG() []byte {
	data, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==",
	)
	return data
}
