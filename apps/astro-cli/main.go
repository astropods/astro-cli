package main

import (
	"github.com/astropods/astro/apps/astro-cli/cmd"
)

func main() {
	defer cmd.CloseDockerClient() //nolint:errcheck
	cmd.Execute()
}
