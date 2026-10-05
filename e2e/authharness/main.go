//go:build integration

// Command authharness is the CLI with its WorkOS URL read from ASTRO_E2E_WORKOS_URL, for e2e tests.
package main

import (
	"os"

	"github.com/astropods/astro-cli/cmd"
	"github.com/astropods/astro-cli/internal/auth"
)

func main() {
	auth.SetWorkOSBaseURLOverride(os.Getenv("ASTRO_E2E_WORKOS_URL"))
	cmd.Execute()
}
