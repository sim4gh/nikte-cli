package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/olekukonko/tablewriter"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	linkTTL         string
	linkPermanent   bool
	linkDeleteForce bool
	linkQR          bool
)

func addLinkCommands() {
	linkCmd := &cobra.Command{
		Use:   "link <url>",
		Short: "Shorten a URL",
		Long: `Shorten a long URL into a share.nikte.co short link.

The short URL is printed and copied to your clipboard. Links expire after
48 hours by default; use --ttl to change it or --permanent to keep it forever.

Examples:
  nk link https://example.com/very/long/path   Shorten a URL
  nk link example.com/foo                       https:// is added if missing
  nk link https://example.com --ttl 7d          Custom expiration
  nk link https://example.com --permanent       Never expire
  nk link ls                                    List your short links
  nk link d a3                                  Delete a short link`,
		Args: cobra.MaximumNArgs(1),
		RunE: runLinkCreate,
	}
	linkCmd.Flags().StringVar(&linkTTL, "ttl", "", "Expiration: 30s, 60m, 24h, 7d (default 48h)")
	linkCmd.Flags().BoolVarP(&linkPermanent, "permanent", "p", false, "Never expire")
	linkCmd.Flags().BoolVar(&linkQR, "qr", false, "Print a scannable QR code of the short URL")

	lsCmd := &cobra.Command{
		Use:     "ls",
		Short:   "List your short links",
		Aliases: []string{"list"},
		Args:    cobra.NoArgs,
		RunE:    runLinkList,
	}

	deleteCmd := &cobra.Command{
		Use:     "d <code>",
		Short:   "Delete a short link",
		Aliases: []string{"delete"},
		Args:    cobra.ExactArgs(1),
		RunE:    runLinkDelete,
	}
	deleteCmd.Flags().BoolVarP(&linkDeleteForce, "force", "f", false, "Skip confirmation")

	linkCmd.AddCommand(lsCmd, deleteCmd)
	rootCmd.AddCommand(linkCmd)
}

// normalizeURL prepends https:// when the input has no http(s) scheme.
func normalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "https://" + s
	}
	return s
}

func runLinkCreate(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	url := normalizeURL(args[0])

	body := map[string]interface{}{"url": url}
	if linkPermanent {
		body["ttl"] = "permanent"
	} else if linkTTL != "" {
		body["ttl"] = linkTTL
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Shortening URL..."
	s.Start()

	resp, err := api.Post("/links", body)
	s.Stop()
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("failed to shorten URL: %s", resp.GetString("message"))
	}

	shortURL := resp.GetString("shortUrl")
	fmt.Printf("%s -> %s\n", url, shortURL)
	if expiresAt := resp.GetInt("expiresAt"); expiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiry(int64(expiresAt)))
	} else {
		fmt.Println("Expires: never (permanent)")
	}
	copyToClipboard(shortURL, "Short URL")
	if linkQR {
		printQR(shortURL)
	}
	return nil
}

func runLinkList(cmd *cobra.Command, args []string) error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Loading links..."
	s.Start()

	resp, err := api.Get("/links")
	s.Stop()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to list links: %s", resp.GetString("message"))
	}

	var result struct {
		Links []struct {
			Code      string `json:"code"`
			URL       string `json:"url"`
			ShortURL  string `json:"shortUrl"`
			ExpiresAt *int64 `json:"expiresAt"`
		} `json:"links"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return err
	}

	if len(result.Links) == 0 {
		fmt.Println("No links.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Code", "Short URL", "Destination", "Expires"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, l := range result.Links {
		expiry := "never"
		if l.ExpiresAt != nil {
			expiry = util.FormatExpiry(*l.ExpiresAt)
		}
		table.Append([]string{l.Code, l.ShortURL, truncateURL(l.URL, 50), expiry})
	}
	table.Render()
	return nil
}

func runLinkDelete(cmd *cobra.Command, args []string) error {
	code := args[0]

	if !linkDeleteForce {
		fmt.Printf("Are you sure you want to delete link %q? [y/N]: ", code)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Deleting link..."
	s.Start()

	resp, err := api.Delete("/links/" + code)
	s.Stop()
	if err != nil {
		return err
	}

	switch resp.StatusCode {
	case 204, 200:
		fmt.Printf("Link %q deleted.\n", code)
		return nil
	case 404:
		return fmt.Errorf("no link found with code %q", code)
	default:
		return fmt.Errorf("failed to delete link: %s", resp.GetString("message"))
	}
}

// truncateURL shortens a URL for table display.
func truncateURL(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
