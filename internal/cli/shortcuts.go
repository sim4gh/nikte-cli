package cli

import (
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/platform"
	"github.com/spf13/cobra"
)

func addShortcutCommands() {
	// nk c - Quick clipboard add
	cCmd := &cobra.Command{
		Use:   "c",
		Short: "Quick add from clipboard (alias for \"nk a\")",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			return handleClipboard(s)
		},
	}

	cCmd.Flags().BoolVar(&addPermanent, "permanent", false, "Keep forever (default: 24h TTL)")
	cCmd.Flags().StringVar(&addTTL, "ttl", defaultTTL, "Custom TTL (e.g., 1h, 7d)")
	cCmd.Flags().BoolVarP(&addPublic, "public", "p", false, "Create public share on add (Pro)")
	cCmd.Flags().StringVar(&addPassword, "password", "", "Password-protected share (Pro)")
	cCmd.Flags().BoolVar(&addQR, "qr", false, "Print a scannable QR code of the share URL (with --public/--password)")
	cCmd.Flags().IntVar(&addMaxViews, "max-views", 0, "Burn-after-read: delete the share after N views (with --public/--password)")

	rootCmd.AddCommand(cCmd)

	// nk sc - Quick screenshot
	scCmd := &cobra.Command{
		Use:   "sc",
		Short: "Quick screenshot (alias for \"nk a sc\")",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			return handleScreenshot(s)
		},
	}

	scCmd.Flags().BoolVar(&addPermanent, "permanent", false, "Keep forever (default: 24h TTL)")
	scCmd.Flags().StringVar(&addTTL, "ttl", defaultTTL, "Custom TTL (e.g., 1h, 7d)")
	scCmd.Flags().BoolVarP(&addPublic, "public", "p", false, "Create public share on add (Pro)")
	scCmd.Flags().StringVar(&addPassword, "password", "", "Password-protected share (Pro)")
	scCmd.Flags().BoolVarP(&addWindow, "window", "w", false, "Capture specific window")
	scCmd.Flags().BoolVarP(&addFullscreen, "fullscreen", "f", false, "Capture full screen")
	scCmd.Flags().StringVar(&addWatch, "watch", "", "Continuous capture mode (optional: interval in seconds)")
	scCmd.Flags().BoolVar(&addQR, "qr", false, "Print a scannable QR code of the share URL (with --public/--password)")
	scCmd.Flags().IntVar(&addMaxViews, "max-views", 0, "Burn-after-read: delete the share after N views (with --public/--password)")

	rootCmd.AddCommand(scCmd)

	// nk p <id> - Quick public share
	pCmd := &cobra.Command{
		Use:   "p <id>",
		Short: "Quick public share (alias for \"nk sh <id> --public\")",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sharePublic = true
			return runShare(cmd, args)
		},
	}

	pCmd.Flags().StringVar(&shareExpires, "expires", "", "Share expiration (default: 24h, e.g., 7d)")
	pCmd.Flags().StringVar(&shareTitle, "title", "", "Share title for social previews")
	pCmd.Flags().StringVar(&shareDesc, "desc", "", "Share description for social previews")
	pCmd.Flags().BoolVar(&shareQR, "qr", false, "Print a scannable QR code of the share URL")
	pCmd.Flags().IntVar(&shareMaxViews, "max-views", 0, "Burn-after-read: delete the link after N views")

	rootCmd.AddCommand(pCmd)
}

func handleScreenshotShortcut(s *spinner.Spinner) error {
	if !platform.IsScreenshotSupported() {
		return errScreenshotNotSupported
	}

	return handleScreenshot(s)
}

var errScreenshotNotSupported = &screenshotError{msg: "screenshot capture is only supported on macOS"}

type screenshotError struct {
	msg string
}

func (e *screenshotError) Error() string {
	return e.msg
}
