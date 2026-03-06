package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/fatih/color"
	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

const versionCheckInterval = 24 * time.Hour
const versionCheckTimeout = 2 * time.Second

// overrides for testing
var (
	versionCacheDir         string // if non-empty, used instead of auth.ConfigDir
	versionCheckDownloadURL string // if non-empty, used instead of downloadBaseURL
)

type versionCache struct {
	LastChecked   time.Time `json:"last_checked"`
	LatestVersion string    `json:"latest_version"`
}

func versionCacheFilePath() (string, error) {
	dir := versionCacheDir
	if dir == "" {
		var err error
		dir, err = auth.ConfigDir(binaryName)
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "version-check.json"), nil
}

func loadVersionCache() *versionCache {
	path, err := versionCacheFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil
	}
	var cache versionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return &cache
}

func saveVersionCache(cache *versionCache) {
	path, err := versionCacheFilePath()
	if err != nil {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func fetchLatestVersion() (string, error) {
	base := versionCheckDownloadURL
	if base == "" {
		base = downloadBaseURL
	}
	if base == "" {
		return "", fmt.Errorf("download URL not configured")
	}
	url := strings.TrimRight(base, "/") + "/VERSION"

	client := &http.Client{Timeout: versionCheckTimeout}
	resp, err := client.Get(url) //nolint:gosec
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// isNewerVersion reports whether latest is a strictly higher semver than
// current. Falls back to string inequality when either version cannot be
// parsed.
func isNewerVersion(latest, current string) bool {
	lv, err := semver.NewVersion(latest)
	if err != nil {
		return latest != current
	}
	cv, err := semver.NewVersion(current)
	if err != nil {
		return latest != current
	}
	return lv.GreaterThan(cv)
}

// notifyIfUpdateAvailable checks whether a newer CLI version is available and
// prints an update notice if so. It caches the result for 24 hours to avoid
// making a network request on every command invocation.
//
// This is a no-op for dev builds (version == "dev").
func notifyIfUpdateAvailable() {
	if version == "dev" {
		return
	}

	verbose, _ := rootCmd.PersistentFlags().GetBool("verbose")
	dim := color.New(color.Faint)

	cache := loadVersionCache()

	// Refresh the cache at most once per interval
	if cache == nil || time.Since(cache.LastChecked) > versionCheckInterval {
		if verbose {
			dim.Fprintln(os.Stderr, "  version check: fetching latest version...") //nolint:errcheck,gosec
		}
		latest, err := fetchLatestVersion()
		if err == nil && latest != "" {
			cache = &versionCache{
				LastChecked:   time.Now(),
				LatestVersion: latest,
			}
			saveVersionCache(cache)
			if verbose {
				dim.Fprintf(os.Stderr, "  version check: latest=%s (cached)\n", latest) //nolint:errcheck,gosec
			}
		} else {
			if verbose {
				dim.Fprintf(os.Stderr, "  version check: fetch failed: %v\n", err) //nolint:errcheck,gosec
			}
			if cache == nil {
				return
			}
		}
	} else if verbose {
		dim.Fprintf(os.Stderr, "  version check: using cache (checked %s ago)\n", time.Since(cache.LastChecked).Truncate(time.Second)) //nolint:errcheck,gosec
	}

	if cache.LatestVersion == "" || !isNewerVersion(cache.LatestVersion, version) {
		if verbose {
			dim.Fprintf(os.Stderr, "  version check: up to date (%s)\n", version) //nolint:errcheck,gosec
		}
		return
	}

	fmt.Fprintln(os.Stderr)
	_, _ = color.New(color.FgYellow).Fprintf(os.Stderr, "  Update available: %s → %s\n", version, cache.LatestVersion)
	_, _ = color.New(color.Faint).Fprintf(os.Stderr, "  Run `%s upgrade` to update.\n\n", binaryName)
}
