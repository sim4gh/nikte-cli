package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	getOutput string
	getURL    bool
	getCopy   bool
)

func addGetCommand() {
	getCmd := &cobra.Command{
		Use:   "g <id>",
		Short: "Get/download item by ID",
		Long: `Get/download item by ID

Examples:
  nk g <id>                   Download item to current directory
    ├ --url                    Get download URL only
    ├ --copy                   Copy download URL to clipboard
    └ -o ~/Downloads           Save to specific directory`,
		Aliases: []string{"get"},
		Args:    cobra.ExactArgs(1),
		RunE:    runGet,
	}

	getCmd.Flags().StringVarP(&getOutput, "output", "o", "", "Save to specific directory")
	getCmd.Flags().BoolVar(&getURL, "url", false, "Get URL only (do not download)")
	getCmd.Flags().BoolVarP(&getCopy, "copy", "c", false, "Copy download URL to clipboard (do not download)")

	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	id := args[0]
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Fetching item..."
	s.Start()

	// Try as short first (most common)
	if found, err := getAsShort(id, s); found || err != nil {
		return err
	}

	// Try as screenshot
	s.Suffix = " Trying as screenshot..."
	if found, err := getAsScreenshot(id, s); found || err != nil {
		return err
	}

	// Try as file (Pro)
	s.Suffix = " Trying as file..."
	if found, err := getAsFile(id, s); found || err != nil {
		return err
	}

	// Not found anywhere
	s.Stop()
	return fmt.Errorf("no item found with ID %q. The item may have expired or never existed", id)
}

func getAsShort(id string, s *spinner.Spinner) (bool, error) {
	resp, err := api.Get("/shorts/" + id)
	if err != nil {
		s.Stop()
		return false, err
	}

	if resp.StatusCode != 200 {
		return false, nil
	}

	s.Stop()
	fmt.Println("Item fetched successfully")

	var result struct {
		Type        string `json:"type"`
		Content     string `json:"content"`
		CreatedAt   string `json:"createdAt"`
		ExpiresAt   int64  `json:"expiresAt"`
		Filename    string `json:"filename"`
		FileSize    int64  `json:"fileSize"`
		ContentType string `json:"contentType"`
		DownloadURL string `json:"downloadUrl"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return true, err
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("ID: %s\n", id)
	fmt.Printf("Type: %s\n", capitalize(result.Type))
	if result.CreatedAt != "" {
		fmt.Printf("Created: %s\n", result.CreatedAt)
	}
	if result.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
		fmt.Printf("Expires At: %s\n", time.Unix(result.ExpiresAt, 0).Format(time.RFC3339))
	}
	fmt.Println(strings.Repeat("=", 60))

	// Handle file type
	if result.Type == "file" {
		fmt.Println()
		fmt.Printf("Filename: %s\n", result.Filename)
		fmt.Printf("Size: %s\n", util.FormatBytes(result.FileSize))
		fmt.Printf("Content-Type: %s\n", result.ContentType)
		fmt.Println()

		return true, handleFileDownload(result.DownloadURL, result.Filename)
	}

	// Handle text type
	fmt.Println()
	fmt.Println(result.Content)
	fmt.Println()

	// Copy content to clipboard
	if err := clipboard.WriteAll(result.Content); err == nil {
		fmt.Println("(Content copied to clipboard)")
	}

	return true, nil
}

func getAsScreenshot(id string, s *spinner.Spinner) (bool, error) {
	resp, err := api.Get("/screenshots/" + id)
	if err != nil {
		s.Stop()
		return false, err
	}

	if resp.StatusCode != 200 {
		return false, nil
	}

	s.Stop()
	fmt.Println("Screenshot fetched successfully")

	var result struct {
		DownloadURL string `json:"downloadUrl"`
		ExpiresAt   int64  `json:"expiresAt"`
		ContentType string `json:"contentType"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return true, err
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("ID: %s\n", id)
	fmt.Println("Type: Screenshot")
	if result.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Determine filename
	ext := "png"
	if strings.Contains(result.ContentType, "jpeg") || strings.Contains(result.ContentType, "jpg") {
		ext = "jpg"
	}
	filename := fmt.Sprintf("screenshot-%s.%s", id, ext)

	return true, handleFileDownload(result.DownloadURL, filename)
}

func getAsFile(id string, s *spinner.Spinner) (bool, error) {
	resp, err := api.Get("/files/" + id)
	if err != nil {
		s.Stop()
		return false, err
	}

	if resp.StatusCode != 200 {
		return false, nil
	}

	s.Stop()
	fmt.Println("File fetched successfully")

	var result struct {
		Filename    string `json:"filename"`
		Size        int64  `json:"size"`
		ContentType string `json:"contentType"`
		DownloadURL string `json:"downloadUrl"`
		Description string `json:"description"`
		ExpiresAt   int64  `json:"expiresAt"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return true, err
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("ID: %s\n", id)
	fmt.Println("Type: File (Pro)")
	fmt.Printf("Filename: %s\n", result.Filename)
	fmt.Printf("Size: %s\n", util.FormatBytes(result.Size))
	fmt.Printf("Content-Type: %s\n", result.ContentType)
	if result.Description != "" {
		fmt.Printf("Description: %s\n", result.Description)
	}
	if result.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
	} else {
		fmt.Println("Expires: never (permanent)")
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	return true, handleFileDownload(result.DownloadURL, result.Filename)
}

func handleFileDownload(downloadURL, filename string) error {
	// If --copy flag, copy URL to clipboard and return
	if getCopy {
		if err := clipboard.WriteAll(downloadURL); err != nil {
			fmt.Println("Failed to copy URL to clipboard")
			fmt.Println("Download URL:", downloadURL)
		} else {
			fmt.Println("Download URL copied to clipboard")
		}
		return nil
	}

	// If --url flag, just show URL
	if getURL {
		fmt.Println("Download URL (valid for 1 hour):")
		fmt.Println(downloadURL)
		if err := clipboard.WriteAll(downloadURL); err == nil {
			fmt.Println("\n(URL copied to clipboard)")
		}
		return nil
	}

	// Download the file
	outputPath := filename
	if getOutput != "" {
		outputPath = filepath.Join(getOutput, filename)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Downloading %s...", filename)
	s.Start()

	if err := downloadFile(downloadURL, outputPath); err != nil {
		s.Stop()
		fmt.Println()
		fmt.Println("Download URL (valid for 1 hour):")
		fmt.Println(downloadURL)
		if err := clipboard.WriteAll(downloadURL); err == nil {
			fmt.Println("(Download URL copied to clipboard)")
		}
		return fmt.Errorf("download failed: %w", err)
	}

	s.Stop()
	fmt.Printf("Downloaded: %s\n", outputPath)

	return nil
}

func downloadFile(url, outputPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
