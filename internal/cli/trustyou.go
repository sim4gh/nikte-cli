package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	trustTTL      string
	trustMax      int
	trustMaxSize  string
	trustPassword string
)

func addTrustYouCommand() {
	trustYouCmd := &cobra.Command{
		Use:   "trustyou",
		Short: "Create a link people use to upload files to you from the browser",
		Long: `Create a request-link that anyone can open in their browser to upload files to you.
No CLI needed on the recipient's side — they just visit the URL.

Examples:
  nk trustyou                          Create link (1 upload, 24h, 5GB max)
    ├ --max 5                           Allow up to 5 uploads
    ├ --ttl 7d                          Link valid for 7 days
    ├ --max-size 200MB                  Limit file size to 200MB
    ├ --password secret                 Require a password to upload
    └ --max 10 --ttl 7d --max-size 1GB  10 uploads, 7-day link, 1GB per file`,
		RunE: runTrustYou,
	}

	trustYouCmd.Flags().StringVar(&trustTTL, "ttl", "24h", "Link expiration (e.g., 1h, 7d; max 30d)")
	trustYouCmd.Flags().IntVar(&trustMax, "max", 1, "Maximum number of uploads allowed")
	trustYouCmd.Flags().StringVar(&trustMaxSize, "max-size", "5GB", "Maximum file size per upload (e.g., 10MB, 1GB, 5GB)")
	trustYouCmd.Flags().StringVar(&trustPassword, "password", "", "Optional password required to upload")

	rootCmd.AddCommand(trustYouCmd)
}

func runTrustYou(cmd *cobra.Command, args []string) error {
	// Parse max file size
	maxFileSize, err := parseSize(trustMaxSize)
	if err != nil {
		return fmt.Errorf("invalid --max-size value %q: %w", trustMaxSize, err)
	}

	// Parse TTL to seconds, then convert to hours (backend expects expiresInHours).
	// The backend caps at 720h (30 days); we enforce the same ceiling.
	ttlSeconds, err := util.ParseTTL(trustTTL)
	if err != nil {
		return fmt.Errorf("invalid --ttl value %q: %w", trustTTL, err)
	}
	expiresInHours := ttlSeconds / 3600
	if expiresInHours < 1 {
		expiresInHours = 1
	}
	if expiresInHours > 720 {
		expiresInHours = 720
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Creating upload link..."
	s.Start()

	body := map[string]interface{}{
		"maxFileSize":    maxFileSize,
		"maxSubmissions": trustMax,
		"expiresInHours": expiresInHours,
	}
	if trustPassword != "" {
		body["password"] = trustPassword
	}

	resp, err := api.Post("/request-links", body)
	s.Stop()

	if err != nil {
		return err
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		msg := resp.GetString("message")
		if msg == "" {
			msg = resp.GetString("error")
		}
		if msg == "" {
			msg = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		}
		return fmt.Errorf("failed to create upload link: %s", msg)
	}

	uploadURL := resp.GetString("url")

	fmt.Println("Upload link created!")
	fmt.Println()
	fmt.Printf("Upload link:  %s\n", uploadURL)
	fmt.Printf("Max uploads:  %d\n", trustMax)
	fmt.Printf("Max file size: %s\n", util.FormatBytes(maxFileSize))
	fmt.Printf("Expires:      in %dh\n", expiresInHours)
	if trustPassword != "" {
		fmt.Println("Password:     set")
	}

	copyToClipboard(uploadURL, "Upload link")

	return nil
}

// parseSize parses a human-readable size string (e.g., "10MB", "1GB", "500KB") to bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	upper := strings.ToUpper(s)

	// Try suffixes from longest to shortest to avoid ambiguity
	type suffix struct {
		label      string
		multiplier int64
	}
	suffixes := []suffix{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}

	for _, sfx := range suffixes {
		if strings.HasSuffix(upper, sfx.label) {
			numStr := strings.TrimSpace(s[:len(s)-len(sfx.label)])
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number %q in size %q", numStr, s)
			}
			if num <= 0 {
				return 0, fmt.Errorf("size must be greater than 0")
			}
			return int64(num * float64(sfx.multiplier)), nil
		}
	}

	return 0, fmt.Errorf("invalid size format %q, use KB, MB, or GB (e.g., 10MB, 1GB)", s)
}
