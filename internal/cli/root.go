package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time
var Version = "0.8.1"

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "nk",
	Short: "nikte CLI - Ephemeral content management",
	Long: `nikte CLI - Ephemeral content management

A fast CLI tool for managing ephemeral content with automatic TTL-based deletion.
Upload text, files, and screenshots with optional sharing capabilities.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = Version

	// Add all subcommands
	addAuthCommands()
	addHealthCommand()
	addConfigCommand()
	addAddCommand()
	addGetCommand()
	addListCommand()
	addDeleteCommand()
	addExtendCommand()
	addShareCommand()
	addRecCommand()
	addTrustYouCommand()
	addShortcutCommands()
	addWaCommands()
	addLinkCommands()

	// Custom root help with tree structure and inline aliases
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			printRootHelp()
		} else {
			defaultHelp(cmd, args)
		}
	})
}

func printRootHelp() {
	fmt.Print(`nikte CLI - Ephemeral content management

A fast CLI tool for managing ephemeral content with automatic TTL-based deletion.
Upload text, files, and screenshots with optional sharing capabilities.

Usage:
  nk [command]

Commands:
  a, add [input]              Add from clipboard, screenshot, file, or text
    ├ c                       Quick clipboard shortcut
    ├ sc                      Quick screenshot shortcut
    │
    │ Flags:
    │   --permanent           Keep forever (no expiration)
    │   --ttl <duration>      Custom TTL (e.g., 1h, 7d, 30d)
    │   --public, -p          Create public share link on add
    │   --password <pass>     Password-protected share on add
    │   --encrypt, -e         Encrypt client-side (zero-knowledge)
    │   --title <text>        Social preview title (with --public)
    │   --desc <text>         Social preview description
    │
    │ Examples:
    │   nk a                 Add from clipboard
    │   nk a doc.pdf         Upload file (default: 24h TTL)
    │   nk a doc.pdf --permanent --public
    │                         Upload permanently + get share URL
    └   nk a "hello" -p     Add text + share publicly

  auth                        Authentication commands
  config [subcommand]         Manage configuration
  d, delete <id>              Delete item by ID
  extend <id>                 Extend TTL or make item permanent
  g, get <id>                 Get/download item by ID
  health                      Check system health status
  ls, list                    List all items
    └ -i, --interactive       Navigable list (arrows, copy, delete)
  rec                         Record screen to GIF, MP4, or MOV
  sh, share <id>              Share item (Pro only)
    ├ ls                      List your shares with view counts
    ├ --qr                    Print a scannable QR of the share URL
    ├ --max-views <n>         Burn-after-read: delete link after N views
    └ p <id>                  Quick public share shortcut
  trustyou                    Create a link for browser file uploads
  wa                          WhatsApp messaging commands
    ├ link                    Link WhatsApp (scan QR code)
    ├ send <number> [msg]     Send a WhatsApp message
    │   └ --item <id>         Forward a nikte item by ID
    ├ ls                      Show incoming messages (live)
    ├ status                  Check link status
    └ unlink                  Unlink WhatsApp

Flags:
  -h, --help      help for nk
  -v, --version   version for nk

Use "nk [command] --help" for more information about a command.
`)
}

// exitWithError prints an error message and exits
func exitWithError(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}
