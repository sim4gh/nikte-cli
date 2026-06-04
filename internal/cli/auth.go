package cli

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/browser"
	"github.com/sim4gh/nikte-cli/internal/auth"
	"github.com/sim4gh/nikte-cli/internal/config"
	"github.com/spf13/cobra"
)

func addAuthCommands() {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(whoamiCmd)

	rootCmd.AddCommand(authCmd)
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login using device flow authentication",
	RunE:  runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials and logout",
	RunE:  runLogout,
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current user information",
	RunE:  runWhoami,
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Initialize device authorization
	deviceAuth, err := auth.InitiateDeviceAuth()
	if err != nil {
		return fmt.Errorf("failed to initiate device authorization: %w", err)
	}

	// Display verification URL and user code
	fmt.Println("\nTo complete authentication, please visit:")
	fmt.Printf("  %s\n", deviceAuth.VerificationURIComplete)
	fmt.Printf("\nUser Code: %s\n\n", deviceAuth.UserCode)

	// Try to open browser
	_ = browser.OpenURL(deviceAuth.VerificationURIComplete)

	// Start spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Waiting for you to complete the login in the browser..."
	s.Start()

	// Poll for token
	tokenResp, err := auth.PollForToken(deviceAuth.DeviceCode, deviceAuth.Interval)
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Println("Login successful!")

	// Load or create config
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	// Store credentials
	cfg.BaseURL = auth.BaseURL
	cfg.IDToken = tokenResp.IDToken
	cfg.AccessToken = tokenResp.AccessToken
	cfg.RefreshToken = tokenResp.RefreshToken
	cfg.LoggedInAt = time.Now().Format(time.RFC3339)

	if err := config.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Println("\nAuthentication complete! You are now logged in.")
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg := config.Get()
	if cfg == nil || cfg.BaseURL == "" {
		fmt.Println("You are not currently logged in.")
		return nil
	}

	// Clear all stored credentials
	if err := config.Clear(); err != nil {
		return fmt.Errorf("failed to clear credentials: %w", err)
	}

	fmt.Println("Successfully logged out. All credentials have been cleared.")
	return nil
}

func runWhoami(cmd *cobra.Command, args []string) error {
	cfg := config.Get()
	if cfg == nil || cfg.BaseURL == "" || cfg.AccessToken == "" {
		fmt.Println("You are not currently logged in.")
		fmt.Println("Run \"nk auth login\" to authenticate.")
		return nil
	}

	fmt.Println("\nCurrent Authentication Status:")
	fmt.Println("------------------------------")
	fmt.Printf("Base URL: %s\n", cfg.BaseURL)

	// Decode and display ID token payload if available
	if cfg.IDToken != "" {
		payload, err := auth.DecodeJWT(cfg.IDToken)
		if err == nil {
			fmt.Println("\nUser Information:")
			if payload.Sub != "" {
				fmt.Printf("  User ID: %s\n", payload.Sub)
			}
			if payload.Email != "" {
				fmt.Printf("  Email: %s\n", payload.Email)
			}
			if payload.Name != "" {
				fmt.Printf("  Name: %s\n", payload.Name)
			}
			if payload.PreferredUsername != "" {
				fmt.Printf("  Username: %s\n", payload.PreferredUsername)
			}
		}
	}

	// Show session expiration
	if cfg.LoggedInAt != "" {
		loginDate, err := time.Parse(time.RFC3339, cfg.LoggedInAt)
		if err == nil {
			sessionExpiry := loginDate.AddDate(1, 0, 0) // 365 days
			daysRemaining := int(time.Until(sessionExpiry).Hours() / 24)

			fmt.Println("\nSession Information:")
			fmt.Printf("  Logged in: %s\n", loginDate.Local().Format("Jan 2, 2006 3:04 PM"))
			fmt.Printf("  Session expires: %s\n", sessionExpiry.Local().Format("Jan 2, 2006 3:04 PM"))
			if daysRemaining > 0 {
				fmt.Printf("  Status: Valid (%d days remaining)\n", daysRemaining)
			} else {
				fmt.Println("  Status: EXPIRED (please login again)")
			}
		}
	} else {
		fmt.Println("\nSession Information:")
		fmt.Println("  Session expires: ~1 year from login")
		fmt.Println("  (Re-login to see exact expiration date)")
	}

	fmt.Println()
	return nil
}
