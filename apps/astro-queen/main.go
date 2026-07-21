package main

import "github.com/astropods/astro/apps/astro-queen/cmd"

func main() {
	cmd.WebFS = webFS
	cmd.Execute()
}
