package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the CLI to the latest version",
	Args:  cobra.NoArgs,
	RunE:  runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().Bool("force", false, "Skip version check and always download")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	forceUpgrade := flagBool(cmd, "force")
	green := color.New(color.FgGreen)
	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	verbose, _ := cmd.Flags().GetBool("verbose")

	if buildinfo.BuildType == buildinfo.BuildTypeDev && !forceUpgrade {
		return fmt.Errorf("cannot upgrade a dev build; use --force to override")
	}

	base := strings.TrimRight(buildinfo.DownloadBaseURL, "/")
	if base == "" {
		return fmt.Errorf("upgrade not available: download URL not configured in this build")
	}

	binName := fmt.Sprintf("%s-%s-%s", buildinfo.BinaryName, runtime.GOOS, runtime.GOARCH)
	downloadURL := base + "/" + binName
	versionURL := base + "/VERSION"

	if verbose {
		dim.Printf("  binary:  %s\n", binName)     //nolint:errcheck,gosec
		dim.Printf("  url:     %s\n", downloadURL) //nolint:errcheck,gosec
	}

	// Fetch the latest version from the VERSION file.
	cyan.Print("→ ") //nolint:errcheck,gosec
	fmt.Println("Checking for updates...")

	latestVersion, err := fetchVersionFile(versionURL)
	if err != nil {
		return fmt.Errorf("failed to check latest version: %w", err)
	}

	if !forceUpgrade {
		if latestVersion != "" && latestVersion == buildinfo.Version {
			green.Print("✓ ") //nolint:errcheck,gosec
			fmt.Printf("Already up to date (%s)\n", buildinfo.Version)
			return nil
		}
	}

	// Resolve the install directory: use the directory of the current binary.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}
	installDir := filepath.Dir(execPath)
	symlinkPath := filepath.Join(installDir, buildinfo.BinaryName)

	// Download to temp file in the install directory.
	tmpFile, err := os.CreateTemp(installDir, ".ast-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) //nolint:errcheck,gosec

	cyan.Print("→ ") //nolint:errcheck,gosec
	fmt.Printf("Downloading %s...\n", binName)

	resp, err := http.Get(downloadURL) //nolint:gosec
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: server returned %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write download: %w", err)
	}
	_ = tmpFile.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if latestVersion != "" {
		// Versioned binary + symlink approach
		versionedName := fmt.Sprintf("%s-%s", buildinfo.BinaryName, latestVersion)
		versionedPath := filepath.Join(installDir, versionedName)

		if err := os.Rename(tmpPath, versionedPath); err != nil { //nolint:gosec
			return fmt.Errorf("failed to install binary: %w", err)
		}

		_ = os.Remove(symlinkPath)
		if err := os.Symlink(versionedName, symlinkPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}

		cleanOldVersions(installDir, latestVersion)
	} else {
		// No version info — fall back to direct replace
		realPath, err := filepath.EvalSymlinks(execPath)
		if err != nil {
			realPath = execPath
		}
		if err := os.Rename(tmpPath, realPath); err != nil { //nolint:gosec
			return fmt.Errorf("failed to replace binary: %w", err)
		}
	}

	green.Print("✓ ") //nolint:errcheck,gosec
	fmt.Print("Upgraded ")
	dim.Print(buildinfo.Version) //nolint:errcheck,gosec
	fmt.Print(" → ")
	if latestVersion != "" {
		green.Println(latestVersion) //nolint:errcheck,gosec
	} else {
		green.Println("latest") //nolint:errcheck,gosec
	}

	return nil
}

// fetchVersionFile fetches the VERSION file at the given URL and returns its
// trimmed content.
func fetchVersionFile(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec
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

// cleanOldVersions removes versioned binaries in dir matching {buildinfo.BinaryName}-{semver}
// except for the one matching keepVersion.
func cleanOldVersions(dir, keepVersion string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	keep := fmt.Sprintf("%s-%s", buildinfo.BinaryName, keepVersion)
	pfx := buildinfo.BinaryName + "-"
	for _, e := range entries {
		name := e.Name()
		if name == keep || !strings.HasPrefix(name, pfx) {
			continue
		}
		rest := strings.TrimPrefix(name, pfx)
		if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
