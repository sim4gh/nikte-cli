//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CaptureScreenshot captures a screenshot (macOS only)
func CaptureScreenshot(window, fullscreen bool) ([]byte, error) {
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("nk-screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	var args []string
	if window {
		args = append(args, "-w")
	} else if fullscreen {
		// No additional args for fullscreen
	} else {
		// Interactive selection (default)
		args = append(args, "-i")
	}
	args = append(args, tempFile)

	cmd := exec.Command("screencapture", args...)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Check if file was created (user might have cancelled)
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		return nil, nil // User cancelled
	}

	imageData, err := os.ReadFile(tempFile)
	if err != nil {
		return nil, err
	}

	if len(imageData) == 0 {
		return nil, nil // User cancelled
	}

	return imageData, nil
}
