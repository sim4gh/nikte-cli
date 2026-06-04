package cli

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/sim4gh/nikte-cli/internal/config"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var configForce bool

func addConfigCommand() {
	configCmd := &cobra.Command{
		Use:   "config [subcommand] [args...]",
		Short: "Manage configuration",
		Long: `Manage configuration

Subcommands:
  (none)              Show all config values
  get <key>           Get a specific value
  set <key> <value>   Set a value
  path                Show config file location
  reset               Clear all config

Allowed keys to set: baseurl, default_ttl, quiet
Protected keys (read-only): id_token, access_token, refresh_token, logged_in_at

Examples:
  nk config                   Show all config
    ├ get baseurl              Get baseurl value
    ├ set default_ttl 7d       Set default TTL
    ├ set quiet true           Enable quiet mode
    ├ path                     Show config file path
    ├ reset                    Reset all config
    └ reset --force            Reset without confirmation`,
		RunE: runConfig,
	}

	configCmd.Flags().BoolVarP(&configForce, "force", "f", false, "Skip confirmation for reset")

	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return showAllConfig()
	}

	subcommand := args[0]

	switch subcommand {
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("please specify a key to get. Usage: nk config get <key>")
		}
		return getConfigValue(args[1])

	case "set":
		if len(args) < 3 {
			return fmt.Errorf("please specify a key and value to set. Usage: nk config set <key> <value>")
		}
		return setConfigValue(args[1], args[2])

	case "path":
		return showConfigPath()

	case "reset":
		return resetConfig()

	default:
		return fmt.Errorf("unknown subcommand %q. Available subcommands: get, set, path, reset", subcommand)
	}
}

func showAllConfig() error {
	cfg := config.Get()

	fmt.Println("\nConfiguration:")
	fmt.Println(strings.Repeat("-", 50))

	if cfg == nil {
		fmt.Println("No configuration values set.")
		fmt.Println()
		return nil
	}

	// Show values in order
	showConfigLine("baseurl", cfg.BaseURL, false)
	showConfigLine("default_ttl", cfg.DefaultTTL, false)
	showConfigLine("quiet", fmt.Sprintf("%v", cfg.Quiet), false)
	showConfigLine("logged_in_at", cfg.LoggedInAt, true)
	showConfigLine("id_token", cfg.IDToken, true)
	showConfigLine("access_token", cfg.AccessToken, true)
	showConfigLine("refresh_token", cfg.RefreshToken, true)

	fmt.Println()
	return nil
}

func showConfigLine(key, value string, protected bool) {
	displayValue := value
	if value == "" {
		displayValue = "(not set)"
	} else if protected && len(value) > 8 {
		displayValue = util.MaskToken(value)
	}

	suffix := ""
	if protected {
		suffix = " (protected)"
	}

	fmt.Printf("  %s: %s%s\n", key, displayValue, suffix)
}

func getConfigValue(key string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("key %q is not set", key)
	}

	var value string
	switch key {
	case "baseurl":
		value = cfg.BaseURL
	case "default_ttl":
		value = cfg.DefaultTTL
	case "quiet":
		value = fmt.Sprintf("%v", cfg.Quiet)
	case "logged_in_at":
		value = cfg.LoggedInAt
	case "id_token":
		value = util.MaskToken(cfg.IDToken)
	case "access_token":
		value = util.MaskToken(cfg.AccessToken)
	case "refresh_token":
		value = util.MaskToken(cfg.RefreshToken)
	default:
		return fmt.Errorf("unknown key %q", key)
	}

	if value == "" {
		fmt.Printf("Key %q is not set.\n", key)
	} else {
		fmt.Println(value)
	}
	return nil
}

func setConfigValue(key, value string) error {
	// Check if key is protected
	if config.IsProtectedKey(key) {
		return fmt.Errorf("%q is a protected key and cannot be modified manually. Protected keys: %s",
			key, strings.Join(config.ProtectedKeys, ", "))
	}

	// Check if key is allowed
	if !config.IsAllowedKey(key) {
		return fmt.Errorf("%q is not a valid configuration key. Allowed keys: %s",
			key, strings.Join(config.AllowedKeys, ", "))
	}

	// Validate specific keys
	switch key {
	case "baseurl":
		if _, err := url.Parse(value); err != nil {
			return fmt.Errorf("%q is not a valid URL", value)
		}
	case "quiet":
		if value != "true" && value != "false" {
			return fmt.Errorf("\"quiet\" must be \"true\" or \"false\"")
		}
	case "default_ttl":
		if !util.IsValidTTL(value) {
			return fmt.Errorf("\"default_ttl\" must be in format like \"30s\", \"60m\", \"24h\", or \"7d\"")
		}
	}

	if err := config.Set(key, value); err != nil {
		return err
	}

	fmt.Printf("Set %q to %q\n", key, value)
	return nil
}

func showConfigPath() error {
	fmt.Println(config.Path())
	return nil
}

func resetConfig() error {
	if !configForce {
		fmt.Print("Are you sure you want to reset all configuration? This will log you out. [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Reset cancelled.")
			return nil
		}
	}

	if err := config.Clear(); err != nil {
		return err
	}

	fmt.Println("Configuration reset. All values have been cleared.")
	return nil
}
