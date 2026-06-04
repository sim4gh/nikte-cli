//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// GetClipboardImage extracts image from clipboard (macOS only)
func GetClipboardImage() ([]byte, error) {
	// Check if pngpaste is installed
	_, err := exec.LookPath("pngpaste")
	if err != nil {
		return nil, fmt.Errorf("pngpaste is not installed. Install with: brew install pngpaste")
	}

	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("nk-clipboard-%d.png", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	cmd := exec.Command("pngpaste", tempFile)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	imageData, err := os.ReadFile(tempFile)
	if err != nil {
		return nil, err
	}

	if len(imageData) == 0 {
		return nil, nil
	}

	return imageData, nil
}
