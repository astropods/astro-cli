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

	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the CLI to the latest version",
	RunE:  runUpgrade,
}

var forceUpgrade bool

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().BoolVar(&forceUpgrade, "force", false, "Skip version check and always download")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)
	dim := color.New(color.Faint)

	verbose, _ := cmd.Flags().GetBool("verbose")
	serverURL := auth.DefaultServerURL
	binName := fmt.Sprintf("%s-%s-%s", binaryName, runtime.GOOS, runtime.GOARCH)
	downloadURL := serverURL + "/download/" + binName

	if verbose {
		dim.Printf("  server:  %s\n", serverURL)
		dim.Printf("  binary:  %s\n", binName)
		dim.Printf("  url:     %s\n", downloadURL)
	}

	// Check latest version via HEAD request
	if !forceUpgrade {
		cyan.Print("→ ")
		fmt.Println("Checking for updates...")

		latest, err := checkLatestVersion(downloadURL, verbose)
		if err != nil {
			return fmt.Errorf("failed to check latest version: %w", err)
		}
		if latest != "" && latest == version {
			green.Print("✓ ")
			fmt.Printf("Already up to date (%s)\n", version)
			return nil
		}
	}

	// Resolve the install directory: use the directory of the current binary.
	// This works whether installed via ~/.ast/bin or elsewhere.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}
	installDir := filepath.Dir(execPath)
	symlinkPath := filepath.Join(installDir, binaryName)

	// Download to temp file in the install directory
	tmpFile, err := os.CreateTemp(installDir, ".ast-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	cyan.Print("→ ")
	fmt.Printf("Downloading %s...\n", binName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download failed: server returned %d", resp.StatusCode)
	}

	newVersion := resp.Header.Get("X-Cli-Version")

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write download: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if newVersion != "" {
		// Versioned binary + symlink approach
		versionedName := fmt.Sprintf("%s-%s", binaryName, newVersion)
		versionedPath := filepath.Join(installDir, versionedName)

		// Move temp to versioned path
		if err := os.Rename(tmpPath, versionedPath); err != nil {
			return fmt.Errorf("failed to install binary: %w", err)
		}

		// Remove old symlink and create new one
		os.Remove(symlinkPath)
		if err := os.Symlink(versionedName, symlinkPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}

		// Clean up old versioned binaries
		cleanOldVersions(installDir, binaryName, newVersion)
	} else {
		// No version header — fall back to direct replace
		realPath, err := filepath.EvalSymlinks(execPath)
		if err != nil {
			realPath = execPath
		}
		if err := os.Rename(tmpPath, realPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
	}

	green.Print("✓ ")
	fmt.Print("Upgraded ")
	dim.Print(version)
	fmt.Print(" → ")
	if newVersion != "" {
		green.Println(newVersion)
	} else {
		green.Println("latest")
	}

	return nil
}

// cleanOldVersions removes versioned binaries in dir matching {binaryName}-{semver}
// except for the one matching keepVersion.
func cleanOldVersions(dir, binaryName, keepVersion string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	keep := fmt.Sprintf("%s-%s", binaryName, keepVersion)
	pfx := binaryName + "-"
	for _, e := range entries {
		name := e.Name()
		if name == keep || !strings.HasPrefix(name, pfx) {
			continue
		}
		// Only remove versioned binaries (start with a digit after binaryName-)
		rest := strings.TrimPrefix(name, pfx)
		if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

// checkLatestVersion sends a HEAD request and reads the X-Cli-Version header.
func checkLatestVersion(url string, verbose bool) (string, error) {
	resp, err := http.Head(url)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if verbose {
		dim := color.New(color.Faint)
		dim.Printf("  HEAD %s → %d\n", url, resp.StatusCode)
		if v := resp.Header.Get("X-Cli-Version"); v != "" {
			dim.Printf("  X-Cli-Version: %s\n", v)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return resp.Header.Get("X-Cli-Version"), nil
}
