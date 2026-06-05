package cli

import (
	"fmt"
	"os"

	"github.com/mdp/qrterminal/v3"
)

// printQR renders a compact half-block QR code for the given URL to stdout,
// preceded by a blank line. It is a no-op for an empty string so callers can
// invoke it unconditionally after producing a share/short URL.
func printQR(url string) {
	if url == "" {
		return
	}
	fmt.Println("\nScan to open:")
	qrterminal.GenerateHalfBlock(url, qrterminal.L, os.Stdout)
}
