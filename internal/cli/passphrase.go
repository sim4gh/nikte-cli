package cli

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// promptPassphrase reads a hidden passphrase from the terminal. When confirm is
// true it asks a second time and verifies the two entries match.
func promptPassphrase(confirm bool) (string, error) {
	fmt.Fprint(os.Stderr, "Passphrase: ")
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read passphrase: %w", err)
	}
	if len(pass) == 0 {
		return "", fmt.Errorf("passphrase cannot be empty")
	}
	if confirm {
		fmt.Fprint(os.Stderr, "Confirm passphrase: ")
		again, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("failed to read passphrase: %w", err)
		}
		if string(pass) != string(again) {
			return "", fmt.Errorf("passphrases do not match")
		}
	}
	return string(pass), nil
}

// resolvePassphrase returns the encryption passphrase from, in order: the
// --enc-pass flag, the NIKTE_PASSPHRASE environment variable, or an interactive
// hidden prompt. confirm only applies to the interactive prompt.
func resolvePassphrase(flagVal string, confirm bool) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if env := os.Getenv("NIKTE_PASSPHRASE"); env != "" {
		return env, nil
	}
	return promptPassphrase(confirm)
}
