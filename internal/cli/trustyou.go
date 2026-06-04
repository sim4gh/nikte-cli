package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	trustTTL     string
	trustMax     int
	trustMaxSize string
)

func addTrustYouCommand() {
	trustYouCmd := &cobra.Command{
		Use:   "trustyou",
		Short: "Create a trust token for unauthenticated uploads",
		Long: `Create a trust token that allows others to upload files to your account

Examples:
  nk trustyou                 Create token (1 upload, 24h, 150MB max)
    ├ --max 5                  Allow up to 5 uploads
    ├ --ttl 7d                 Token valid for 7 days
    ├ --max-size 10MB          Limit file size to 10MB
    └ --max 10 --ttl 1h        10 uploads, expires in 1 hour`,
		RunE: runTrustYou,
	}

	trustYouCmd.Flags().StringVar(&trustTTL, "ttl", "24h", "Token expiration (e.g., 1h, 7d)")
	trustYouCmd.Flags().IntVar(&trustMax, "max", 1, "Maximum number of uploads allowed")
	trustYouCmd.Flags().StringVar(&trustMaxSize, "max-size", "150MB", "Maximum file size per upload (e.g., 10MB, 1GB)")

	rootCmd.AddCommand(trustYouCmd)
}

func runTrustYou(cmd *cobra.Command, args []string) error {
	// Parse max file size
	maxFileSize, err := parseSize(trustMaxSize)
	if err != nil {
		return fmt.Errorf("invalid --max-size value %q: %w", trustMaxSize, err)
	}

	// Parse TTL to seconds
	ttlSeconds, err := util.ParseTTL(trustTTL)
	if err != nil {
		return fmt.Errorf("invalid --ttl value %q: %w", trustTTL, err)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Creating trust token..."
	s.Start()

	body := map[string]interface{}{
		"ttl":         ttlSeconds,
		"maxUploads":  trustMax,
		"maxFileSize": maxFileSize,
	}

	resp, err := api.Post("/trust", body)
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
		return fmt.Errorf("failed to create trust token: %s", msg)
	}

	var result struct {
		TrustToken  string `json:"trustToken"`
		UploadURL   string `json:"uploadUrl"`
		MaxUploads  int    `json:"maxUploads"`
		MaxFileSize int64  `json:"maxFileSize"`
		ExpiresAt   int64  `json:"expiresAt"`
		CreatedAt   string `json:"createdAt"`
	}

	if err := resp.Unmarshal(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("Trust token created!")
	fmt.Println()
	fmt.Printf("Token:        %s\n", result.TrustToken)
	fmt.Printf("Upload URL:   %s\n", result.UploadURL)
	fmt.Printf("Max uploads:  %d\n", result.MaxUploads)
	fmt.Printf("Max file size: %s\n", util.FormatBytes(result.MaxFileSize))
	fmt.Printf("Expires:      %s\n", util.FormatExpiryTime(result.ExpiresAt))

	// Copy upload URL to clipboard
	if result.UploadURL != "" {
		if err := clipboard.WriteAll(result.UploadURL); err == nil {
			fmt.Println("\n(Upload URL copied to clipboard)")
		}
	}

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
