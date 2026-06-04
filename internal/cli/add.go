package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/atotto/clipboard"
	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/platform"
	"github.com/sim4gh/nikte-cli/internal/upload"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	addPermanent  bool
	addTTL        string
	addPublic     bool
	addPassword   string
	addTitle      string
	addDesc       string
	addWindow     bool
	addFullscreen bool
	addWatch      string
	addQR         bool
	addMaxViews   int
)

const (
	maxTextSizeBytes = 360 * 1024              // 360KB for text
	maxFileSizeBytes = 10 * 1024 * 1024 * 1024 // 10GB for files
	defaultTTL       = "24h"
)

func addAddCommand() {
	addCmd := &cobra.Command{
		Use:   "a [input]",
		Short: "Add item from clipboard, screenshot, file, or text",
		Long: `Add item from clipboard, screenshot, file, or text

Examples:
  nk a                        Add from clipboard (text or image)
    ├ sc                       Take screenshot (macOS)
    ├ sc --watch               Continuous screenshot mode
    ├ sc --watch 5             Auto-capture every 5 seconds
    ├ document.pdf             Add file from path
    ├ "Hello world"            Add text content
    ├ --permanent              Add with no expiration
    └ photo.jpg --public       Add and share`,
		Aliases: []string{"add"},
		RunE:    runAdd,
	}

	addCmd.Flags().BoolVar(&addPermanent, "permanent", false, "Keep forever (default: 24h TTL)")
	addCmd.Flags().StringVar(&addTTL, "ttl", defaultTTL, "Custom TTL (e.g., 1h, 7d)")
	addCmd.Flags().BoolVarP(&addPublic, "public", "p", false, "Create public share on add (Pro)")
	addCmd.Flags().StringVar(&addPassword, "password", "", "Password-protected share (Pro)")
	addCmd.Flags().StringVar(&addTitle, "title", "", "Share title for social previews (with --public)")
	addCmd.Flags().StringVar(&addDesc, "desc", "", "Share description for social previews (with --public)")
	addCmd.Flags().BoolVarP(&addWindow, "window", "w", false, "Capture specific window (for screenshot)")
	addCmd.Flags().BoolVarP(&addFullscreen, "fullscreen", "f", false, "Capture full screen (for screenshot)")
	addCmd.Flags().StringVar(&addWatch, "watch", "", "Continuous screenshot mode (optional: interval in seconds)")
	addCmd.Flags().BoolVar(&addQR, "qr", false, "Print a scannable QR code of the share URL (with --public/--password)")
	addCmd.Flags().IntVar(&addMaxViews, "max-views", 0, "Burn-after-read: delete the share after N views (with --public/--password)")

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	var input string
	if len(args) > 0 {
		input = args[0]
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

	// Case 1: Screenshot command "nk a sc"
	if input == "sc" {
		return handleScreenshot(s)
	}

	// Case 2: File path provided
	if input != "" {
		if fileInfo, err := os.Stat(input); err == nil && !fileInfo.IsDir() {
			return handleFileUpload(input, s)
		}
	}

	// Case 3: Direct text content provided
	if input != "" {
		return handleTextContent(input, s)
	}

	// Case 4: No input - read from clipboard
	return handleClipboard(s)
}

func handleScreenshot(s *spinner.Spinner) error {
	if !platform.IsScreenshotSupported() {
		return fmt.Errorf("screenshot capture is only supported on macOS")
	}

	// Check for watch mode
	if addWatch != "" {
		return handleWatchMode(s)
	}

	fmt.Println("Select area for screenshot...")
	imageData, err := platform.CaptureScreenshot(addWindow, addFullscreen)
	if err != nil {
		return err
	}
	if imageData == nil {
		fmt.Println("Screenshot cancelled")
		return nil
	}

	s.Suffix = " Uploading screenshot..."
	s.Start()
	return uploadImage(imageData, s, "screenshot")
}

func handleWatchMode(s *spinner.Spinner) error {
	// Simplified watch mode - just capture once for now
	fmt.Println("Watch mode not yet implemented in Go version")
	return nil
}

func handleFileUpload(filePath string, s *spinner.Spinner) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("cannot upload empty file")
	}

	if fileInfo.Size() > maxFileSizeBytes {
		return fmt.Errorf("file too large. Maximum size is 10GB, file is %s", util.FormatBytes(fileInfo.Size()))
	}

	filename := filepath.Base(filePath)
	contentType := upload.GetMimeType(filePath)
	ttlSeconds := calculateTTL(true)

	fmt.Printf("File: %s\n", filename)
	fmt.Printf("Size: %s\n", util.FormatBytes(fileInfo.Size()))
	fmt.Printf("Type: %s\n", contentType)

	s.Suffix = " Reading file..."
	s.Start()

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Println("File read successfully")

	// Initialize multipart upload
	s.Suffix = " Initializing upload..."
	s.Start()

	initBody := map[string]interface{}{
		"filename":    filename,
		"contentType": contentType,
		"fileSize":    fileInfo.Size(),
	}
	if addPermanent {
		initBody["ttl"] = "permanent"
	} else if ttlSeconds > 0 {
		initBody["ttl"] = fmt.Sprintf("%ds", ttlSeconds)
	}

	resp, err := api.Post("/shorts/file/init", initBody)
	if err != nil {
		s.Stop()
		return err
	}

	if resp.StatusCode != 201 {
		s.Stop()
		return fmt.Errorf("failed to initialize upload: %s", resp.GetString("message"))
	}

	var initResp struct {
		ShortID       string `json:"shortId"`
		PresignedUrls []struct {
			PartNumber int    `json:"partNumber"`
			URL        string `json:"url"`
		} `json:"presignedUrls"`
		PartSize  int   `json:"partSize"`
		ExpiresAt int64 `json:"expiresAt"`
	}
	if err := resp.Unmarshal(&initResp); err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Printf("Upload initialized (ID: %s)\n", initResp.ShortID)

	// Convert presigned URLs to upload.PresignedURL type
	presignedUrls := make([]upload.PresignedURL, len(initResp.PresignedUrls))
	for i, pu := range initResp.PresignedUrls {
		presignedUrls[i] = upload.PresignedURL{
			PartNumber: pu.PartNumber,
			URL:        pu.URL,
		}
	}

	// Upload parts
	totalParts := len(presignedUrls)
	s.Suffix = fmt.Sprintf(" Uploading 0/%d parts...", totalParts)
	s.Start()

	completedParts, err := upload.UploadParts(presignedUrls, fileData, initResp.PartSize, func(completed, total int, completedBytes, totalBytes int64) {
		progress := util.CreateProgressBar(completedBytes, totalBytes, 30)
		s.Suffix = fmt.Sprintf(" Uploading %d/%d parts... %s %s/%s", completed, total, progress, util.FormatBytes(completedBytes), util.FormatBytes(totalBytes))
	})
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Printf("Uploaded %d parts\n", totalParts)

	// Complete multipart upload
	s.Suffix = " Finalizing upload..."
	s.Start()

	completeResp, err := api.Post("/shorts/file/complete", map[string]interface{}{
		"shortId": initResp.ShortID,
		"parts":   completedParts,
	})
	if err != nil {
		s.Stop()
		return err
	}

	if completeResp.StatusCode != 200 {
		s.Stop()
		return fmt.Errorf("failed to complete upload: %s", completeResp.GetString("message"))
	}

	s.Stop()
	fmt.Println("Upload complete!")
	fmt.Println()
	fmt.Printf("ID: %s\n", initResp.ShortID)
	if initResp.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(initResp.ExpiresAt))
	} else {
		fmt.Println("Expires: never (permanent)")
	}

	// Copy ID to clipboard
	copyToClipboard(initResp.ShortID, "ID")

	// Handle sharing if requested
	if addPublic || addPassword != "" {
		return createShare(initResp.ShortID, "short")
	}

	return nil
}

func handleTextContent(content string, s *spinner.Spinner) error {
	contentBytes := len(content)
	if contentBytes > maxTextSizeBytes {
		return fmt.Errorf("content exceeds maximum size of %dKB (current: %.2fKB)",
			maxTextSizeBytes/1024, float64(contentBytes)/1024)
	}

	s.Suffix = " Creating item..."
	s.Start()

	return uploadTextContent(content, s)
}

func handleClipboard(s *spinner.Spinner) error {
	s.Suffix = " Reading clipboard..."
	s.Start()

	// Check for image first (macOS only)
	if platform.IsScreenshotSupported() {
		if platform.ClipboardHasImage() {
			imageData, err := platform.GetClipboardImage()
			if err == nil && imageData != nil {
				s.Stop()
				fmt.Println("Clipboard image read successfully")
				uploadSpinner := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
				uploadSpinner.Suffix = " Uploading image..."
				uploadSpinner.Start()
				return uploadImage(imageData, uploadSpinner, "clipboard")
			}
		}
	}

	// Try to read text from clipboard
	text, err := clipboard.ReadAll()
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to read clipboard: %w", err)
	}

	if text == "" {
		s.Stop()
		return fmt.Errorf("clipboard is empty. Hint: Copy some text or an image to clipboard first")
	}

	s.Stop()
	fmt.Println("Clipboard content read successfully")

	createSpinner := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	createSpinner.Suffix = " Creating item..."
	createSpinner.Start()

	return uploadTextContent(text, createSpinner)
}

func uploadTextContent(content string, s *spinner.Spinner) error {
	ttlSeconds := calculateTTL(false)

	body := map[string]interface{}{
		"content": content,
	}
	if addPermanent {
		body["ttl"] = 0
	} else if ttlSeconds > 0 {
		body["ttl"] = ttlSeconds
	}

	resp, err := api.Post("/shorts", body)
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()

	if resp.StatusCode == 201 {
		fmt.Println("Item created successfully")

		var result struct {
			ShortID   string `json:"shortId"`
			ExpiresAt int64  `json:"expiresAt"`
		}
		if err := resp.Unmarshal(&result); err != nil {
			return err
		}

		fmt.Printf("\nID: %s\n", result.ShortID)
		if result.ExpiresAt > 0 {
			fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
		} else {
			fmt.Println("Expires: never (permanent)")
		}

		copyToClipboard(result.ShortID, "ID")

		// Handle sharing if requested
		if addPublic || addPassword != "" {
			return createShare(result.ShortID, "short")
		}

		return nil
	}

	if resp.StatusCode == 413 {
		return fmt.Errorf("content too large: %s", resp.GetString("message"))
	}

	return fmt.Errorf("failed to create item: %s", resp.GetString("message"))
}

func uploadImage(imageData []byte, s *spinner.Spinner, source string) error {
	ttlSeconds := calculateTTL(true)
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	body := map[string]interface{}{
		"contentType": "image/png",
		"data":        base64Data,
	}
	if addPermanent {
		body["ttl"] = "permanent"
	} else if ttlSeconds > 0 {
		body["ttl"] = fmt.Sprintf("%ds", ttlSeconds)
	} else {
		body["ttl"] = "24h"
	}

	resp, err := api.Post("/screenshots", body)
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()

	if resp.StatusCode == 201 {
		fmt.Println("Image uploaded successfully")

		var result struct {
			ScreenshotID string `json:"screenshotId"`
			ExpiresAt    int64  `json:"expiresAt"`
		}
		if err := resp.Unmarshal(&result); err != nil {
			return err
		}

		// Get the download URL
		urlResp, err := api.Get(fmt.Sprintf("/screenshots/%s", result.ScreenshotID))
		if err == nil && urlResp.StatusCode == 200 {
			var urlResult struct {
				DownloadURL string `json:"downloadUrl"`
			}
			if err := urlResp.Unmarshal(&urlResult); err == nil {
				fmt.Printf("\nID: %s\n", result.ScreenshotID)
				fmt.Printf("URL: %s\n", urlResult.DownloadURL)
				if result.ExpiresAt > 0 {
					fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
				}

				copyToClipboard(urlResult.DownloadURL, "URL")
			}
		} else {
			fmt.Printf("\nID: %s\n", result.ScreenshotID)
			if result.ExpiresAt > 0 {
				fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
			}
		}

		return nil
	}

	if resp.StatusCode == 413 {
		return fmt.Errorf("image too large: %s", resp.GetString("message"))
	}

	return fmt.Errorf("failed to upload image: %s", resp.GetString("message"))
}

func calculateTTL(isFile bool) int {
	if addPermanent {
		return 0
	}

	ttlString := addTTL
	if ttlString == "" {
		ttlString = defaultTTL
	}

	ttlSeconds, err := util.ParseTTL(ttlString)
	if err != nil {
		ttlSeconds = 24 * 3600 // Default to 24h
	}

	return ttlSeconds
}

func createShare(itemID, itemType string) error {
	fmt.Println("\nCreating share link...")
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Creating share..."
	s.Start()

	endpoint := fmt.Sprintf("/shorts/%s/share", itemID)
	if itemType == "screenshot" {
		endpoint = fmt.Sprintf("/screenshots/%s/share", itemID)
	}

	body := map[string]interface{}{
		"isPublic": addPublic || addPassword == "",
	}
	if addPassword != "" {
		body["password"] = addPassword
	}
	if addTitle != "" {
		body["title"] = addTitle
	}
	if addDesc != "" {
		body["description"] = addDesc
	}
	if addMaxViews > 0 {
		body["maxViews"] = addMaxViews
	}

	resp, err := api.Post(endpoint, body)
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to create share: %w", err)
	}

	s.Stop()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		fmt.Println("Share link created!")

		var shareResult struct {
			ShareURL string `json:"shareUrl"`
			URL      string `json:"url"`
		}
		if err := resp.Unmarshal(&shareResult); err == nil {
			shareURL := shareResult.ShareURL
			if shareURL == "" {
				shareURL = shareResult.URL
			}
			if shareURL != "" {
				fmt.Printf("\nShare URL: %s\n", shareURL)
				copyToClipboard(shareURL, "Share URL")
				if addQR {
					printQR(shareURL)
				}
			}
		}
		return nil
	}

	return fmt.Errorf("failed to create share: %s", resp.GetString("message"))
}

func copyToClipboard(text, label string) {
	if err := clipboard.WriteAll(text); err == nil {
		fmt.Printf("\n(%s copied to clipboard)\n", label)
	}
}
