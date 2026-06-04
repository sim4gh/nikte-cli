package cli

import (
	"fmt"

	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/spf13/cobra"
)

func addHealthCommand() {
	rootCmd.AddCommand(healthCmd)
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check system health status",
	RunE:  runHealth,
}

func runHealth(cmd *cobra.Command, args []string) error {
	resp, err := api.GetNoAuth("/health")
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	var health struct {
		Status    string `json:"status"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}

	if err := resp.Unmarshal(&health); err != nil {
		return fmt.Errorf("failed to parse health response: %w", err)
	}

	fmt.Printf("Status: %s\n", health.Status)
	fmt.Printf("Message: %s\n", health.Message)
	fmt.Printf("Timestamp: %s\n", health.Timestamp)

	return nil
}
