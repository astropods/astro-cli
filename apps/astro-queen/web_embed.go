package main

import "embed"

//go:embed all:web/dist
var webFS embed.FS
