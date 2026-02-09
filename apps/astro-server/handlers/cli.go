package handlers

import (
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/config"
)

// Allowed CLI binary names (no path traversal). Mac only for now.
var allowedCLINames = map[string]bool{
	"ast-darwin-amd64": true,
	"ast-darwin-arm64": true,
}

// CLIDownload serves CLI binaries from CLIDir. Only allowed filenames are served.
func CLIDownload(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
			c.AbortWithStatus(400)
			return
		}
		if !allowedCLINames[name] {
			c.AbortWithStatus(404)
			return
		}
		path := filepath.Join(cfg.Server.CLIDir, name)
		c.FileAttachment(path, name)
	}
}
