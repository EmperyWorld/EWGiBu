// Command iacpack is the CLI entrypoint for the IaC Packager.
package main

import (
	"os"

	"iacpack/internal/adapters/cli"
)

func main() {
	os.Exit(cli.Execute())
}
