package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/config"
)

// Allowed CLI download names (no path traversal). Mac only for now.
var allowedCLINames = map[string]bool{
	"ast-darwin-amd64":         true,
	"ast-darwin-arm64":         true,
	"ast-preview-darwin-amd64": true,
	"ast-preview-darwin-arm64": true,
	"checksums.txt":            true,
}

// CLIDownload serves CLI binaries from CLIDir. Only allowed filenames are served.
// It also sets the X-Cli-Version header from the VERSION file in CLIDir.
func CLIDownload(cfg *config.Config) gin.HandlerFunc {
	// Read version once at init time
	cliVersion := ""
	if data, err := os.ReadFile(filepath.Join(cfg.Server.CLIDir, "VERSION")); err == nil {
		cliVersion = strings.TrimSpace(string(data))
	}

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
		if cliVersion != "" {
			c.Header("X-Cli-Version", cliVersion)
		}
		path := filepath.Join(cfg.Server.CLIDir, name)
		c.FileAttachment(path, name)
	}
}

// CLIInstallScript returns a shell script that detects the platform and
// installs the CLI binary. Usage: curl -fsSL <host>/install | sh
func CLIInstallScript(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.Request.Host
		scheme := "https"
		if c.Request.TLS == nil && (strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.")) {
			scheme = "http"
		}
		base := fmt.Sprintf("%s://%s", scheme, host)

		prefix := "ast"
		if strings.Contains(host, "astropod.ai") {
			prefix = "ast-preview"
		}

		script := fmt.Sprintf(`#!/bin/sh
set -e

BASE="%s"
PREFIX="%s"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

URL="${BASE}/download/${PREFIX}-${OS}-${ARCH}"
INSTALL_DIR="${HOME}/.ast/bin"
HEADER_FILE="$(mktemp)"

mkdir -p "$INSTALL_DIR"

BINARY_NAME="${PREFIX}-${OS}-${ARCH}"

echo "Downloading ${PREFIX} for ${OS}/${ARCH}..."
curl -fsSL -D "$HEADER_FILE" "$URL" -o "$INSTALL_DIR/${PREFIX}.tmp"

# Verify checksum
echo "Verifying checksum..."
CHECKSUMS="$(curl -fsSL "${BASE}/download/checksums.txt")"
EXPECTED="$(echo "$CHECKSUMS" | grep "$BINARY_NAME" | awk '{print $1}')"
if [ -n "$EXPECTED" ]; then
  # Use shasum on macOS, sha256sum on Linux
  if command -v shasum >/dev/null 2>&1; then
    ACTUAL="$(shasum -a 256 "$INSTALL_DIR/${PREFIX}.tmp" | awk '{print $1}')"
  else
    ACTUAL="$(sha256sum "$INSTALL_DIR/${PREFIX}.tmp" | awk '{print $1}')"
  fi
  if [ "$ACTUAL" != "$EXPECTED" ]; then
    rm -f "$INSTALL_DIR/${PREFIX}.tmp" "$HEADER_FILE"
    echo "Checksum verification failed!" >&2
    echo "  Expected: $EXPECTED" >&2
    echo "  Got:      $ACTUAL" >&2
    exit 1
  fi
  echo "Checksum OK"
fi

chmod +x "$INSTALL_DIR/${PREFIX}.tmp"

# Extract version from response header to create versioned binary
VERSION=$(grep -i '^X-Cli-Version:' "$HEADER_FILE" | tr -d '\r' | awk '{print $2}')
rm -f "$HEADER_FILE"

if [ -n "$VERSION" ]; then
  mv "$INSTALL_DIR/${PREFIX}.tmp" "$INSTALL_DIR/${PREFIX}-${VERSION}"
  ln -sf "${PREFIX}-${VERSION}" "$INSTALL_DIR/${PREFIX}"

  # Remove old versioned binaries (keep only the new one)
  for f in "$INSTALL_DIR/${PREFIX}"-*; do
    case "$(basename "$f")" in
      "${PREFIX}-${VERSION}"|"${PREFIX}-${OS}-"*) ;; # keep new version + platform binaries won't be here but guard anyway
      "${PREFIX}"-[0-9]*) rm -f "$f" ;;
    esac
  done

  echo "Installed ${PREFIX} ${VERSION} to ${INSTALL_DIR}"
else
  mv "$INSTALL_DIR/${PREFIX}.tmp" "$INSTALL_DIR/${PREFIX}"
  echo "Installed ${PREFIX} to ${INSTALL_DIR}"
fi

# Check if INSTALL_DIR is on PATH
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo ""
    echo "Add this to your shell profile:"
    echo "  export PATH=\"\$HOME/.ast/bin:\$PATH\""
    ;;
esac
`, base, prefix)

		c.Data(200, "text/plain; charset=utf-8", []byte(script))
	}
}
