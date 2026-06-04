package main

import (
	"github.com/sim4gh/nikte-cli/internal/cli"
	"github.com/sim4gh/nikte-cli/internal/config"
)

func main() {
	// Load config on startup
	config.Load()

	// Execute CLI
	cli.Execute()
}
