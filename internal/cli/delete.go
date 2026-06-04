package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/spf13/cobra"
)

var deleteForce bool

func addDeleteCommand() {
	deleteCmd := &cobra.Command{
		Use:   "d <id>",
		Short: "Delete item by ID",
		Long: `Delete item by ID

Examples:
  nk d <id>                   Delete with confirmation
    └ --force                  Delete without confirmation`,
		Aliases: []string{"delete"},
		Args:    cobra.ExactArgs(1),
		RunE:    runDelete,
	}

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation")

	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	id := args[0]

	// Skip confirmation if --force flag is provided
	if !deleteForce {
		fmt.Printf("Are you sure you want to delete item %q? [y/N]: ", id)

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
	s.Suffix = " Deleting item..."
	s.Start()

	result := tryDelete(id)

	s.Stop()

	if result.success {
		fmt.Println("Item deleted successfully")
		fmt.Printf("\nItem %q has been deleted.\n", id)
		return nil
	}

	// Handle errors
	switch result.error {
	case "not_found":
		return fmt.Errorf("no item found with ID %q. The item may have already expired or been deleted", id)
	case "pro_required":
		return fmt.Errorf("deleting Pro files requires a Pro subscription")
	default:
		return fmt.Errorf("failed to delete item: %s", result.error)
	}
}

type deleteResult struct {
	success bool
	source  string
	error   string
}

func tryDelete(id string) deleteResult {
	// Try as short first (most common)
	resp, err := api.Delete("/shorts/" + id)
	if err == nil && (resp.StatusCode == 204 || resp.StatusCode == 200) {
		return deleteResult{success: true, source: "short"}
	}

	// Try as screenshot
	resp, err = api.Delete("/screenshots/" + id)
	if err == nil && (resp.StatusCode == 204 || resp.StatusCode == 200) {
		return deleteResult{success: true, source: "screenshot"}
	}

	// Try as file (Pro)
	resp, err = api.Delete("/files/" + id)
	if err == nil && (resp.StatusCode == 204 || resp.StatusCode == 200) {
		return deleteResult{success: true, source: "file"}
	}

	// Check if it was a 403 (Pro required)
	if resp != nil && resp.StatusCode == 403 {
		return deleteResult{success: false, error: "pro_required"}
	}

	// Not found in any source
	return deleteResult{success: false, error: "not_found"}
}
