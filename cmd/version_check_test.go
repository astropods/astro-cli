package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureStderr redirects os.Stderr to a pipe for the duration of f, then
// returns whatever was written.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	f()

	_ = w.Close()
	var buf strings.Builder
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		buf.Write(b[:n])
		if err != nil {
			break
		}
	}
	return buf.String()
}

// setupVersionCheckTest wires overrides for cache dir and server URL, and
// restores them when the test ends.
func setupVersionCheckTest(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	dir := t.TempDir()
	orig := versionCacheDir
	origURL := versionCheckServerURL
	origVersion := version

	versionCacheDir = dir
	if srv != nil {
		versionCheckServerURL = srv.URL
	}

	t.Cleanup(func() {
		versionCacheDir = orig
		versionCheckServerURL = origURL
		version = origVersion
	})
	return dir
}

// --- cache I/O ---

func TestSaveAndLoadVersionCache(t *testing.T) {
	dir := t.TempDir()
	orig := versionCacheDir
	versionCacheDir = dir
	t.Cleanup(func() { versionCacheDir = orig })

	want := &versionCache{
		LastChecked:   time.Now().Truncate(time.Second),
		LatestVersion: "1.2.3",
	}
	saveVersionCache(want)

	got := loadVersionCache()
	if got == nil {
		t.Fatal("loadVersionCache returned nil after save")
	}
	if got.LatestVersion != want.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, want.LatestVersion)
	}
	if !got.LastChecked.Equal(want.LastChecked) {
		t.Errorf("LastChecked = %v, want %v", got.LastChecked, want.LastChecked)
	}
}

func TestLoadVersionCache_Missing(t *testing.T) {
	dir := t.TempDir()
	orig := versionCacheDir
	versionCacheDir = dir
	t.Cleanup(func() { versionCacheDir = orig })

	if got := loadVersionCache(); got != nil {
		t.Errorf("expected nil for missing cache, got %+v", got)
	}
}

func TestLoadVersionCache_Malformed(t *testing.T) {
	dir := t.TempDir()
	orig := versionCacheDir
	versionCacheDir = dir
	t.Cleanup(func() { versionCacheDir = orig })

	_ = os.WriteFile(filepath.Join(dir, "version-check.json"), []byte("not-json{{{"), 0o600)

	if got := loadVersionCache(); got != nil {
		t.Errorf("expected nil for malformed cache, got %+v", got)
	}
}

// --- fetchLatestVersion ---

func TestFetchLatestVersion_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.Header().Set("X-Cli-Version", "1.9.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.9.0" {
		t.Errorf("got %q, want %q", got, "1.9.0")
	}
}

func TestFetchLatestVersion_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)

	_, err := fetchLatestVersion()
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestFetchLatestVersion_NoVersionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No X-Cli-Version header
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty version, got %q", got)
	}
}

// --- notifyIfUpdateAvailable ---

func TestNotifyIfUpdateAvailable_DevBuild(t *testing.T) {
	setupVersionCheckTest(t, nil)
	version = "dev"

	out := captureStderr(t, notifyIfUpdateAvailable)
	if out != "" {
		t.Errorf("expected no output for dev build, got %q", out)
	}
}

func TestNotifyIfUpdateAvailable_AlreadyUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cli-Version", "1.0.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)
	version = "1.0.0"

	out := captureStderr(t, notifyIfUpdateAvailable)
	if out != "" {
		t.Errorf("expected no output when already up to date, got %q", out)
	}
}

func TestNotifyIfUpdateAvailable_UpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cli-Version", "2.0.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)
	version = "1.0.0"

	out := captureStderr(t, notifyIfUpdateAvailable)
	if !strings.Contains(out, "2.0.0") {
		t.Errorf("expected output to mention new version 2.0.0, got %q", out)
	}
	if !strings.Contains(out, "upgrade") {
		t.Errorf("expected output to mention 'upgrade' command, got %q", out)
	}
}

func TestNotifyIfUpdateAvailable_UsesCacheWhenFresh(t *testing.T) {
	// Server should not be called when cache is fresh.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("X-Cli-Version", "9.9.9")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)
	version = "1.0.0"

	// Pre-populate a fresh cache with a known version.
	fresh := &versionCache{
		LastChecked:   time.Now(),
		LatestVersion: "1.5.0",
	}
	saveVersionCache(fresh)

	out := captureStderr(t, notifyIfUpdateAvailable)

	if called {
		t.Error("expected no HTTP request when cache is fresh")
	}
	if !strings.Contains(out, "1.5.0") {
		t.Errorf("expected output to show cached version 1.5.0, got %q", out)
	}
}

func TestNotifyIfUpdateAvailable_RefreshesStalCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cli-Version", "3.0.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)
	version = "1.0.0"

	// Write a stale cache with an outdated version entry.
	stale := &versionCache{
		LastChecked:   time.Now().Add(-48 * time.Hour),
		LatestVersion: "2.0.0",
	}
	saveVersionCache(stale)

	out := captureStderr(t, notifyIfUpdateAvailable)

	// Should have fetched from server and shown the new version.
	if !strings.Contains(out, "3.0.0") {
		t.Errorf("expected output to show refreshed version 3.0.0, got %q", out)
	}

	// Cache should have been updated on disk.
	updated := loadVersionCache()
	if updated == nil || updated.LatestVersion != "3.0.0" {
		t.Errorf("expected cache to be updated to 3.0.0, got %+v", updated)
	}
}

func TestNotifyIfUpdateAvailable_SilentOnNetworkError(t *testing.T) {
	// Point at a server that immediately closes connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close to simulate network failure
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	setupVersionCheckTest(t, srv)
	version = "1.0.0"
	// No cache — first run, network failure.

	out := captureStderr(t, notifyIfUpdateAvailable)
	if out != "" {
		t.Errorf("expected no output on network error with no cache, got %q", out)
	}
}

// --- cache file permissions ---

func TestSaveVersionCache_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	orig := versionCacheDir
	versionCacheDir = dir
	t.Cleanup(func() { versionCacheDir = orig })

	saveVersionCache(&versionCache{LastChecked: time.Now(), LatestVersion: "1.0.0"})

	info, err := os.Stat(filepath.Join(dir, "version-check.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

// --- JSON round-trip sanity ---

func TestVersionCacheJSON(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	c := versionCache{LastChecked: ts, LatestVersion: "0.5.1"}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got versionCache
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.LatestVersion != c.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, c.LatestVersion)
	}
	if !got.LastChecked.Equal(c.LastChecked) {
		t.Errorf("LastChecked = %v, want %v", got.LastChecked, c.LastChecked)
	}
}
