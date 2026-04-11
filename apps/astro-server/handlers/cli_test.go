package handlers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/gin-gonic/gin"
)

// ---- fake CDN ----

// fakeCDN is an httptest.Server that mimics the CLI download CDN, serving
// VERSION, a platform binary, and checksums.txt. Its state can be mutated
// between script runs to exercise upgrade and error scenarios.
type fakeCDN struct {
	mu          sync.RWMutex
	version     string
	binary      []byte
	binaryName  string
	badChecksum bool // serve a wrong hash in checksums.txt
	noChecksum  bool // omit the binary's entry from checksums.txt
	Server      *httptest.Server
}

func newFakeCDN(t *testing.T, version string, binary []byte) *fakeCDN {
	t.Helper()
	name := fmt.Sprintf("ast-%s-%s", runtime.GOOS, runtime.GOARCH)
	f := &fakeCDN{version: version, binary: binary, binaryName: name}

	mux := http.NewServeMux()
	mux.HandleFunc("/VERSION", func(w http.ResponseWriter, r *http.Request) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		fmt.Fprint(w, f.version)
	})
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		w.Write(f.binary)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		sum := fmt.Sprintf("%x", sha256.Sum256(f.binary))
		if f.badChecksum {
			sum = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		}
		if f.noChecksum {
			// Return a checksums file that doesn't include this binary's entry.
			fmt.Fprintf(w, "%s  some-other-binary\n", sum)
			return
		}
		fmt.Fprintf(w, "%s  %s\n", sum, name)
	})

	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeCDN) update(version string, binary []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version = version
	f.binary = binary
}

// fakeBinary returns a shell script that echoes "ast/<version> (abc1234) BETA"
// on --version, matching the real binary's output format.
func fakeBinary(version string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho \"ast/%s (abc1234) BETA\"\n", version))
}

// ---- install server ----

// newInstallServer wires CLIInstallScript and CLIDownload onto a live
// httptest.Server backed by the given CDN URL.
func newInstallServer(t *testing.T, cdnURL string) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{DownloadBaseURL: cdnURL},
	}
	r := gin.New()
	r.GET("/install", CLIInstallScript(cfg))
	r.GET("/download/:name", CLIDownload(cfg))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// ---- script runner ----

// runInstallScript fetches /install from srvURL, writes the script to a temp
// file, and runs it with sh. HOME is set to homeDir and SHELL to /bin/zsh so
// the script installs into an isolated directory and writes to .zshrc.
// Returns combined stdout+stderr and any exit error.
func runInstallScript(t *testing.T, srvURL, homeDir string) (string, error) {
	t.Helper()

	resp, err := http.Get(srvURL + "/install")
	if err != nil {
		t.Fatalf("fetch /install: %v", err)
	}
	defer resp.Body.Close()
	scriptBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}

	scriptFile := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(scriptFile, scriptBytes, 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	// Inherit the full test environment (keeps curl, shasum, etc. on PATH)
	// but override HOME and SHELL for isolation.
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HOME=") && !strings.HasPrefix(e, "SHELL=") {
			env = append(env, e)
		}
	}
	env = append(env, "HOME="+homeDir, "SHELL=/bin/zsh")

	cmd := exec.Command("sh", scriptFile)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ---- CLIInstallScript E2E tests ----

func TestInstallScript_FreshInstall(t *testing.T) {
	cdn := newFakeCDN(t, "0.1.0", fakeBinary("0.1.0"))
	srv := newInstallServer(t, cdn.Server.URL)
	homeDir := t.TempDir()

	out, err := runInstallScript(t, srv.URL, homeDir)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Downloading") {
		t.Errorf("expected 'Downloading' in output:\n%s", out)
	}
	if !strings.Contains(out, "Checksum OK") {
		t.Errorf("expected 'Checksum OK' in output:\n%s", out)
	}
	if !strings.Contains(out, "is ready") {
		t.Errorf("expected 'is ready' in output:\n%s", out)
	}

	binDir := filepath.Join(homeDir, ".ast", "bin")
	if _, err := os.Stat(filepath.Join(binDir, "ast-0.1.0")); err != nil {
		t.Errorf("versioned binary not found: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "ast")); err != nil {
		t.Errorf("symlink not found: %v", err)
	}

	rc, err := os.ReadFile(filepath.Join(homeDir, ".zshrc"))
	if err != nil {
		t.Fatalf(".zshrc not created: %v", err)
	}
	if !strings.Contains(string(rc), ".ast/bin") {
		t.Errorf(".zshrc missing .ast/bin:\n%s", rc)
	}
}

func TestInstallScript_Upgrade(t *testing.T) {
	cdn := newFakeCDN(t, "0.1.0", fakeBinary("0.1.0"))
	srv := newInstallServer(t, cdn.Server.URL)
	homeDir := t.TempDir()

	if _, err := runInstallScript(t, srv.URL, homeDir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	cdn.update("0.2.0", fakeBinary("0.2.0"))

	out, err := runInstallScript(t, srv.URL, homeDir)
	if err != nil {
		t.Fatalf("upgrade failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Upgrading") {
		t.Errorf("expected 'Upgrading' in output:\n%s", out)
	}
	if !strings.Contains(out, "0.1.0") || !strings.Contains(out, "0.2.0") {
		t.Errorf("expected both version numbers in upgrade message:\n%s", out)
	}

	binDir := filepath.Join(homeDir, ".ast", "bin")
	if _, err := os.Stat(filepath.Join(binDir, "ast-0.1.0")); !os.IsNotExist(err) {
		t.Error("expected ast-0.1.0 to be cleaned up after upgrade")
	}
	if _, err := os.Stat(filepath.Join(binDir, "ast-0.2.0")); err != nil {
		t.Errorf("expected ast-0.2.0 to exist: %v", err)
	}
}

func TestInstallScript_BadChecksum(t *testing.T) {
	cdn := newFakeCDN(t, "0.1.0", fakeBinary("0.1.0"))
	cdn.badChecksum = true
	srv := newInstallServer(t, cdn.Server.URL)
	homeDir := t.TempDir()

	out, err := runInstallScript(t, srv.URL, homeDir)
	if err == nil {
		t.Errorf("expected non-zero exit:\n%s", out)
	}
	if !strings.Contains(out, "Checksum verification failed") {
		t.Errorf("expected checksum error in output:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(homeDir, ".ast", "bin", "ast")); err == nil {
		t.Error("binary should not be installed after checksum failure")
	}
}

func TestInstallScript_MissingChecksum(t *testing.T) {
	cdn := newFakeCDN(t, "0.1.0", fakeBinary("0.1.0"))
	cdn.noChecksum = true
	srv := newInstallServer(t, cdn.Server.URL)
	homeDir := t.TempDir()

	out, err := runInstallScript(t, srv.URL, homeDir)
	if err == nil {
		t.Errorf("expected non-zero exit:\n%s", out)
	}
	if !strings.Contains(out, "No checksum found") {
		t.Errorf("expected 'No checksum found' in output:\n%s", out)
	}
}

func TestInstallScript_PathInjection_NoDuplicate(t *testing.T) {
	cdn := newFakeCDN(t, "0.1.0", fakeBinary("0.1.0"))
	srv := newInstallServer(t, cdn.Server.URL)
	homeDir := t.TempDir()

	if _, err := runInstallScript(t, srv.URL, homeDir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Bump version so the second run proceeds past the download step.
	cdn.update("0.2.0", fakeBinary("0.2.0"))

	if _, err := runInstallScript(t, srv.URL, homeDir); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(homeDir, ".zshrc"))
	if err != nil {
		t.Fatalf(".zshrc not found: %v", err)
	}
	if n := strings.Count(string(content), ".ast/bin"); n != 1 {
		t.Errorf("expected exactly 1 .ast/bin entry in .zshrc, got %d:\n%s", n, content)
	}
}

// ---- CLIDownload tests ----

func setupCLIRouter(downloadBaseURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{DownloadBaseURL: downloadBaseURL},
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

func TestCLIDownload_AllowedName_Redirects(t *testing.T) {
	r := setupCLIRouter("https://cdn.example.com")
	rec := doGet(r, "/download/ast-darwin-arm64", "")

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://cdn.example.com/ast-darwin-arm64" {
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
		t.Error("path traversal should not produce a redirect")
	}
}

func TestCLIDownload_EmptyBaseURL_Returns503(t *testing.T) {
	r := setupCLIRouter("")
	rec := doGet(r, "/download/ast-darwin-arm64", "")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when DownloadBaseURL is empty, got %d", rec.Code)
	}
}
