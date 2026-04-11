package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/gin-gonic/gin"
)

func setupCLIRouter(downloadBaseURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			DownloadBaseURL: downloadBaseURL,
		},
	}
	r := gin.New()
	r.GET("/install", CLIInstallScript(cfg))
	r.GET("/download/:name", CLIDownload(cfg))
	return r
}

func doGet(r *gin.Engine, path, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if host != "" {
		req.Host = host
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// --- CLIInstallScript ---

func TestCLIInstallScript_ContainsDownloadBase(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://cdn.example.com") {
		t.Error("script does not contain the configured DOWNLOAD_BASE")
	}
}

func TestCLIInstallScript_ContentType(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %q", ct)
	}
}

func TestCLIInstallScript_ProductionHost_UsesAstPrefix(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, `PREFIX="ast"`) {
		t.Error("expected PREFIX to be ast for production host")
	}
	if strings.Contains(body, `PREFIX="ast-preview"`) {
		t.Error("expected no ast-preview prefix for production host")
	}
}

func TestCLIInstallScript_PreviewHost_UsesPreviewPrefix(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "preview.astropod.ai")

	body := rec.Body.String()
	if !strings.Contains(body, `PREFIX="ast-preview"`) {
		t.Errorf("expected PREFIX to be ast-preview for preview host, got body:\n%s", body)
	}
}

func TestCLIInstallScript_CleanupMatchesSemverAndVPrefixed(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, `[0-9]*`) {
		t.Error("cleanup glob should match bare semver (e.g. 0.1.2)")
	}
	if !strings.Contains(body, `v[0-9]*`) {
		t.Error("cleanup glob should match v-prefixed semver (e.g. v0.1.2)")
	}
}

func TestCLIInstallScript_FailsOnMissingChecksum(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, `No checksum found for`) {
		t.Error("script should fail loudly when no checksum is found for the binary")
	}
}

func TestCLIInstallScript_PathInjection_WritesShellRC(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, `.zshrc`) {
		t.Error("script should reference .zshrc for PATH injection")
	}
	if !strings.Contains(body, `.bashrc`) {
		t.Error("script should reference .bashrc for PATH injection")
	}
	if !strings.Contains(body, `.profile`) {
		t.Error("script should reference .profile as fallback for PATH injection")
	}
	if !strings.Contains(body, `export PATH="$HOME/.ast/bin:$PATH"`) {
		t.Error("script should write export PATH line into shell config")
	}
}

func TestCLIInstallScript_PostInstallVerification(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, "--version") {
		t.Error("script should run --version to verify the binary after install")
	}
	if !strings.Contains(body, "is ready") {
		t.Error("script should print a ready message on successful post-install verification")
	}
}

func TestCLIInstallScript_ExistingInstallDetection(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/install", "astropods.ai")

	body := rec.Body.String()
	if !strings.Contains(body, "EXISTING_VERSION") {
		t.Error("script should detect an existing install before downloading")
	}
	if !strings.Contains(body, "Upgrading") {
		t.Error("script should print Upgrading message when an existing install is found")
	}
}

// --- CLIDownload ---

func TestCLIDownload_AllowedName_Redirects(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/download/ast-darwin-arm64", "")

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "https://cdn.example.com/ast-darwin-arm64" {
		t.Errorf("unexpected redirect location: %q", loc)
	}
}

func TestCLIDownload_UnknownName_Returns404(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/download/ast-windows-amd64", "")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestCLIDownload_PathTraversal_Returns400(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/download/../etc/passwd", "")

	if rec.Code == http.StatusMovedPermanently {
		t.Error("path traversal attempt should not result in a redirect")
	}
}

func TestCLIDownload_EmptyBaseURL_Returns503(t *testing.T) {
	r := setupCLIRouter("")
	rec := doGet(r, "/download/ast-darwin-arm64", "")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when DownloadBaseURL is empty, got %d", rec.Code)
	}
}
