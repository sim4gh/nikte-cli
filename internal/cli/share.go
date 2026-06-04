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
	sharePublic   bool
	sharePassword string
	shareExpires  string
	shareTitle    string
	shareDesc     string
)

const defaultShareExpiryDays = 1

func addShareCommand() {
	shareCmd := &cobra.Command{
		Use:   "sh <id>",
		Short: "Share item (Pro only)",
		Long: `Share item (Pro only)

Examples:
  nk sh <id>                  Create public share link
    ├ --password x             Password-protected share
    ├ --expires 7d             Share expires in 7 days
    └ --title "My Doc"         Share with title and description

All shares use share.nikte.co/{id}`,
		Aliases: []string{"share"},
		Args:    cobra.ExactArgs(1),
		RunE:    runShare,
	}

	shareCmd.Flags().BoolVarP(&sharePublic, "public", "p", false, "Public share (default)")
	shareCmd.Flags().StringVar(&sharePassword, "password", "", "Password-protected share")
	shareCmd.Flags().StringVar(&shareExpires, "expires", "", "Share expiration (default: 24h, e.g., 7d)")
	shareCmd.Flags().StringVar(&shareTitle, "title", "", "Share title for social previews")
	shareCmd.Flags().StringVar(&shareDesc, "desc", "", "Share description for social previews")

	rootCmd.AddCommand(shareCmd)
}

func runShare(cmd *cobra.Command, args []string) error {
	id := args[0]

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Creating share link..."
	s.Start()

	// Try to share as a file first
	result := shareFile(id)

	// If file not found, try sharing as a short
	if !result.success && result.reason == "not_found" {
		result = shareShort(id)
	}

	s.Stop()

	if result.success {
		displayShareSuccess(result.data)

		// Copy share URL to clipboard
		if result.data.ShareURL != "" {
			if err := clipboard.WriteAll(result.data.ShareURL); err == nil {
				fmt.Println("\n(Share URL copied to clipboard)")
			}
		}
		return nil
	}

	// Handle errors
	switch result.reason {
	case "pro_required":
		return fmt.Errorf(`sharing requires a Pro subscription

To share content:
  1. Upgrade to Pro for sharing capabilities
  2. Use "nk files add <path>" to upload files
  3. Use "nk sh <id>" to create share links`)
	case "not_found":
		return fmt.Errorf("no shareable item found with ID %q. Sharing is available for Pro files and shorts", id)
	default:
		if result.message != "" {
			return fmt.Errorf("%s", result.message)
		}
		return fmt.Errorf("failed to create share (unknown error)")
	}
}

type shareResult struct {
	success bool
	reason  string
	message string
	data    shareData
}

type shareData struct {
	ShareID     string `json:"shareId"`
	ShareURL    string `json:"shareUrl"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
	ExpiresAt   int64  `json:"expiresAt"`
	Password    string `json:"password"`
}

func shareFile(id string) shareResult {
	body := buildShareBody()

	resp, err := api.Post(fmt.Sprintf("/files/%s/share", id), body)
	if err != nil {
		return shareResult{success: false, reason: "error", message: err.Error()}
	}

	if resp.StatusCode == 403 {
		return shareResult{success: false, reason: "pro_required"}
	}

	if resp.StatusCode == 404 {
		return shareResult{success: false, reason: "not_found"}
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		msg := resp.GetString("message")
		if msg == "" {
			msg = resp.GetString("error")
		}
		// For server errors (5xx), include status code for clarity
		if resp.StatusCode >= 500 {
			if msg == "" {
				msg = fmt.Sprintf("server error (status %d)", resp.StatusCode)
			} else {
				msg = fmt.Sprintf("%s (server error %d)", msg, resp.StatusCode)
			}
		} else if msg == "" {
			msg = fmt.Sprintf("status %d: %s", resp.StatusCode, string(resp.Body))
		}
		return shareResult{success: false, reason: "error", message: msg}
	}

	var data shareData
	resp.Unmarshal(&data)

	// Handle URL field variations
	if data.ShareURL == "" && data.URL != "" {
		data.ShareURL = data.URL
	}

	return shareResult{success: true, data: data}
}

func shareShort(id string) shareResult {
	body := buildShareBody()

	resp, err := api.Post(fmt.Sprintf("/shorts/%s/share", id), body)
	if err != nil {
		return shareResult{success: false, reason: "error", message: err.Error()}
	}

	if resp.StatusCode == 403 {
		return shareResult{success: false, reason: "pro_required"}
	}

	if resp.StatusCode == 404 {
		return shareResult{success: false, reason: "not_found"}
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		msg := resp.GetString("message")
		if msg == "" {
			msg = resp.GetString("error")
		}
		// For server errors (5xx), include status code for clarity
		if resp.StatusCode >= 500 {
			if msg == "" {
				msg = fmt.Sprintf("server error (status %d)", resp.StatusCode)
			} else {
				msg = fmt.Sprintf("%s (server error %d)", msg, resp.StatusCode)
			}
		} else if msg == "" {
			msg = fmt.Sprintf("status %d: %s", resp.StatusCode, string(resp.Body))
		}
		return shareResult{success: false, reason: "error", message: msg}
	}

	var data shareData
	resp.Unmarshal(&data)

	// Handle URL field variations
	if data.ShareURL == "" && data.URL != "" {
		data.ShareURL = data.URL
	}

	return shareResult{success: true, data: data}
}

func buildShareBody() map[string]interface{} {
	isPublic := sharePublic || sharePassword == ""
	expiresInDays := parseExpiresToDays(shareExpires)

	body := map[string]interface{}{
		"isPublic": isPublic,
	}

	if sharePassword != "" {
		body["password"] = sharePassword
		body["isPublic"] = false
	}

	if expiresInDays > 0 {
		body["expiresInDays"] = expiresInDays
	}

	if shareTitle != "" {
		body["title"] = shareTitle
	}

	if shareDesc != "" {
		body["description"] = shareDesc
	}

	return body
}

func parseExpiresToDays(expiresStr string) int {
	if expiresStr == "" {
		return defaultShareExpiryDays
	}

	// Handle day format directly
	if strings.HasSuffix(expiresStr, "d") {
		dayStr := strings.TrimSuffix(expiresStr, "d")
		if days, err := strconv.Atoi(dayStr); err == nil {
			return days
		}
	}

	// Convert TTL to days
	seconds, err := util.ParseTTL(expiresStr)
	if err != nil {
		return defaultShareExpiryDays
	}

	days := (seconds + 86400 - 1) / 86400 // Round up to nearest day
	if days < 1 {
		return 1
	}
	return days
}

func displayShareSuccess(share shareData) {
	fmt.Println("Share created!")
	fmt.Println()

	fmt.Printf("Share ID: %s\n", share.ShareID)
	if share.Title != "" {
		fmt.Printf("Title: %s\n", share.Title)
	}
	if share.Description != "" {
		fmt.Printf("Description: %s\n", share.Description)
	}

	shareType := "Public"
	if !share.IsPublic {
		shareType = "Password Protected"
	}
	fmt.Printf("Type: %s\n", shareType)

	if share.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", time.Unix(share.ExpiresAt, 0).Format("Jan 2, 2006 3:04 PM"))
	}

	fmt.Println()
	fmt.Println("Share URL:")
	fmt.Println(share.ShareURL)
}
