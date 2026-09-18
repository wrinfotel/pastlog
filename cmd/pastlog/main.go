// Command pastlog searches the full history of AI coding-agent sessions.
package main

import (
	"os"

	"github.com/wrinfotel/pastlog/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
