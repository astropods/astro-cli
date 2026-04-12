package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/gin-gonic/gin"
)

// allowedCLINames is the set of valid download filenames (prevents path traversal).
var allowedCLINames = map[string]bool{
	"ast-darwin-amd64":         true,
	"ast-darwin-arm64":         true,
	"ast-linux-amd64":          true,
	"ast-linux-arm64":          true,
	"ast-preview-darwin-amd64": true,
	"ast-preview-darwin-arm64": true,
	"ast-preview-linux-amd64":  true,
	"ast-preview-linux-arm64":  true,
	"checksums.txt":            true,
}

// RegisterCLIRoutes registers the CLI install and download routes on router.
// Called from main.go and reused in tests so both exercise the same wiring.
func RegisterCLIRoutes(router *gin.Engine, cfg *config.Config) {
	router.GET("/install", CLIInstallScript(cfg))
	if cfg.Server.DownloadBaseURL != "" {
		router.GET("/download/:name", CLIDownload(cfg))
		router.HEAD("/download/:name", CLIDownload(cfg))
	}
}

// CLIDownload redirects to the configured DownloadBaseURL for backward
// compatibility with CLI versions that resolve binaries via the server.
func CLIDownload(cfg *config.Config) gin.HandlerFunc {
	baseURL := strings.TrimRight(cfg.Server.DownloadBaseURL, "/")
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if !allowedCLINames[name] {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s/%s", baseURL, name))
	}
}

// CLIInstallScript returns a shell script that detects the platform and
// installs the CLI binary directly from the configured download CDN.
// Usage: curl -fsSL <host>/install | sh
func CLIInstallScript(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.Request.Host
		downloadBase := strings.TrimRight(cfg.Server.DownloadBaseURL, "/")

		prefix := "ast"
		// this is the preview domain
		if strings.Contains(host, "astropod.ai") {
			prefix = "ast-preview"
		}

		script := fmt.Sprintf(`#!/bin/sh
set -e

DOWNLOAD_BASE="%s"
PREFIX="%s"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

INSTALL_DIR="${HOME}/.ast/bin"
mkdir -p "$INSTALL_DIR"

VERSION="$(curl -fsSL "${DOWNLOAD_BASE}/VERSION" | tr -d '[:space:]')"
if [ -z "$VERSION" ]; then
  echo "Failed to fetch version from ${DOWNLOAD_BASE}/VERSION" >&2
  exit 1
fi

# Detect existing install
EXISTING_VERSION=""
if [ -x "$INSTALL_DIR/${PREFIX}" ]; then
  EXISTING_VERSION="$("$INSTALL_DIR/${PREFIX}" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)"
fi

BINARY_NAME="${PREFIX}-${OS}-${ARCH}"
URL="${DOWNLOAD_BASE}/${BINARY_NAME}"

if [ -n "$EXISTING_VERSION" ]; then
  echo "Upgrading ${PREFIX} from ${EXISTING_VERSION} to ${VERSION} for ${OS}/${ARCH}..."
else
  echo "Downloading ${PREFIX} ${VERSION} for ${OS}/${ARCH}..."
fi
curl -fsSL "$URL" -o "$INSTALL_DIR/${PREFIX}.tmp"

echo "Verifying checksum..."
CHECKSUMS="$(curl -fsSL "${DOWNLOAD_BASE}/checksums.txt")"
EXPECTED="$(echo "$CHECKSUMS" | awk -v name="$BINARY_NAME" '$2 == name {print $1}')"
if [ -z "$EXPECTED" ]; then
  rm -f "$INSTALL_DIR/${PREFIX}.tmp"
  echo "No checksum found for $BINARY_NAME" >&2
  exit 1
fi
# Use shasum on macOS, sha256sum on Linux
if command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$INSTALL_DIR/${PREFIX}.tmp" | awk '{print $1}')"
else
  ACTUAL="$(sha256sum "$INSTALL_DIR/${PREFIX}.tmp" | awk '{print $1}')"
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
  rm -f "$INSTALL_DIR/${PREFIX}.tmp"
  echo "Checksum verification failed!" >&2
  echo "  Expected: $EXPECTED" >&2
  echo "  Got:      $ACTUAL" >&2
  exit 1
fi
echo "Checksum OK"

chmod +x "$INSTALL_DIR/${PREFIX}.tmp"
mv "$INSTALL_DIR/${PREFIX}.tmp" "$INSTALL_DIR/${PREFIX}-${VERSION}"
ln -sf "${PREFIX}-${VERSION}" "$INSTALL_DIR/${PREFIX}"

# Remove old versioned binaries (keep only the new one)
for f in "$INSTALL_DIR/${PREFIX}"-*; do
  [ -e "$f" ] || continue
  case "$(basename "$f")" in
    "${PREFIX}-${VERSION}") ;;
    "${PREFIX}"-[0-9]*|"${PREFIX}"-v[0-9]*) rm -f "$f" ;;
  esac
done

echo "Installed ${PREFIX} ${VERSION} to ${INSTALL_DIR}"

# Inject INSTALL_DIR into shell config if not already on PATH
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    SHELL_RC=""
    if [ -n "$ZSH_VERSION" ] || [ "$(basename "$SHELL")" = "zsh" ]; then
      SHELL_RC="$HOME/.zshrc"
    elif [ -f "$HOME/.bashrc" ]; then
      SHELL_RC="$HOME/.bashrc"
    else
      SHELL_RC="$HOME/.profile"
    fi
    if ! grep -q '\.ast/bin' "$SHELL_RC" 2>/dev/null; then
      echo "Adding ~/.ast/bin to PATH in $SHELL_RC"
      echo '' >> "$SHELL_RC"
      echo '# astropods' >> "$SHELL_RC"
      echo 'export PATH="$HOME/.ast/bin:$PATH"' >> "$SHELL_RC"
      echo "Run: source $SHELL_RC"
    fi
    ;;
esac

# Post-install verification
if "$INSTALL_DIR/${PREFIX}" --version >/dev/null 2>&1; then
  echo "${PREFIX} is ready"
else
  echo "Binary installed but failed to run — check your platform or file an issue" >&2
fi
`, downloadBase, prefix)

		c.Data(200, "text/plain; charset=utf-8", []byte(script))
	}
}
