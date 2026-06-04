package cli

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	extendTTL       string
	extendPermanent bool
)

func addExtendCommand() {
	extendCmd := &cobra.Command{
		Use:   "extend <id>",
		Short: "Extend TTL or make item permanent",
		Long: `Extend TTL or make item permanent

Examples:
  nk extend <id>              Extend item TTL
    ├ --ttl 7d                 Extend to 7 days from now
    ├ --ttl 24h                Extend to 24 hours from now
    └ --permanent              Make permanent (no expiration)`,
		Args: cobra.ExactArgs(1),
		RunE: runExtend,
	}

	extendCmd.Flags().StringVar(&extendTTL, "ttl", "", "New TTL from now (e.g., 1h, 7d, 30d)")
	extendCmd.Flags().BoolVar(&extendPermanent, "permanent", false, "Remove TTL (make permanent)")

	rootCmd.AddCommand(extendCmd)
}

func runExtend(cmd *cobra.Command, args []string) error {
	id := args[0]

	// Validate options
	if extendTTL == "" && !extendPermanent {
		return fmt.Errorf(`please specify either --ttl <duration> or --permanent

Examples:
  nk extend %s --ttl 7d      # Extend to 7 days from now
  nk extend %s --ttl 24h     # Extend to 24 hours from now
  nk extend %s --permanent   # Make permanent (no expiration)`, id, id, id)
	}

	if extendTTL != "" && extendPermanent {
		return fmt.Errorf("cannot use both --ttl and --permanent together")
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Extending TTL..."
	s.Start()

	// Build request body
	var body map[string]interface{}
	if extendPermanent {
		body = map[string]interface{}{"permanent": true}
	} else {
		body = map[string]interface{}{"ttl": extendTTL}
	}

	resp, err := api.Patch("/shorts/"+id, body)
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()

	// Handle success
	if resp.StatusCode == 200 {
		var result struct {
			ExpiresAt int64 `json:"expiresAt"`
		}
		resp.Unmarshal(&result)

		if extendPermanent {
			fmt.Println("Item is now permanent")
			fmt.Printf("\nItem %q will no longer expire.\n", id)
		} else {
			fmt.Println("TTL extended successfully")
			fmt.Printf("\nItem %q now expires %s\n", id, util.FormatExpiryTime(result.ExpiresAt))
		}
		return nil
	}

	// Handle errors
	switch resp.StatusCode {
	case 404:
		return fmt.Errorf("no item found with ID %q. The item may have already expired or been deleted", id)
	case 403:
		return fmt.Errorf("extending files requires a Pro subscription")
	case 400:
		return fmt.Errorf("invalid TTL format: %s", resp.GetString("message"))
	default:
		return fmt.Errorf("failed to extend TTL: %s", resp.GetString("message"))
	}
}
