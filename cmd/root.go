package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/telemetry"
)

func fullVersion() string {
	if buildinfo.Commit != "" {
		return buildinfo.BinaryName + "/" + buildinfo.Version + " (" + buildinfo.Commit + ") BETA"
	}
	return buildinfo.BinaryName + "/" + buildinfo.Version + " BETA"
}

var rootCmd = &cobra.Command{
	Use:   buildinfo.BinaryName,
	Short: "Astropods CLI - Build, push, and develop AI agents",
	Long: astroBanner() + `

Build, push, and develop AI agents.`,
	Version:       "placeholder",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := buildinfo.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Identify the subcommand before executing so we can skip the version
	// check for commands that manage the CLI itself.
	invoked, _, _ := rootCmd.Find(os.Args[1:])

	// Print first-run telemetry notice (once per install)
	if telemetry.EnsureNoticed(buildinfo.BinaryName) {
		fmt.Fprintln(os.Stderr, "Notice: Astropods collects anonymous usage data to improve the CLI.")
		fmt.Fprintln(os.Stderr, "Run `"+buildinfo.BinaryName+" settings update --telemetry off` to opt out.")
		fmt.Fprintln(os.Stderr)
	}

	start := time.Now()
	execErr := rootCmd.Execute()

	// Send telemetry — skip noise commands (help, version, completion, telemetry config)
	cmdName := resolveCommandName(invoked, os.Args[1:])
	if telemetry.IsEnabled(buildinfo.BinaryName) && !isNoiseCommand(cmdName) {
		tc := buildTelemetryClient()
		if tc != nil {
			tc.TrackCommand(cmdName, time.Since(start), execErr)
			tc.Shutdown()
		}
	}

	if execErr != nil {
		fmt.Fprintln(os.Stderr, execErr)
		os.Exit(1)
	}

	if invoked == nil || invoked.Name() != "upgrade" {
		notifyIfUpdateAvailable()
	}
}

// resolveCommandName returns a command name like "deploy" or "configure.set".
// Detects --version / --help flags that Cobra handles on the root command.
func resolveCommandName(cmd *cobra.Command, args []string) string {
	// Detect --version flag (Cobra runs it on root, no subcommand)
	for _, a := range args {
		if a == "--version" || a == "-V" {
			return "version"
		}
		if a == "--help" || a == "-h" {
			return "help"
		}
		if a == "--" {
			break
		}
	}

	if cmd == nil {
		return "unknown"
	}
	var parts []string
	for c := cmd; c != nil && c != rootCmd; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	if len(parts) == 0 {
		return "root"
	}
	return strings.Join(parts, ".")
}

// isNoiseCommand returns true for commands that don't provide useful analytics signal.
func isNoiseCommand(name string) bool {
	switch name {
	case "help", "version", "settings.update":
		return true
	}
	switch name {
	case "settings.bash", "settings.zsh", "settings.fish", "settings.powershell":
		return true
	}
	return false
}

// buildTelemetryClient creates a one-shot telemetry client.
// User identity enrichment happens on the web client — CLI just needs the same UserID.
func buildTelemetryClient() *telemetry.Client {
	userID := ""
	storage := auth.NewStorage(buildinfo.BinaryName)
	if profile, err := storage.GetCurrentProfile(); err == nil && profile.User != nil {
		userID = profile.User.ID
	}
	deviceID := telemetry.GetDeviceID(buildinfo.BinaryName)

	return telemetry.NewClient(userID, deviceID, buildinfo.Version)
}

func init() {
	rootCmd.Version = fullVersion()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
}

// SpecFileAliases are filenames checked in order when the user does not pass --file.
var SpecFileAliases = []string{"astropods.yml", "astropods.yaml", "astroai.yml", "astro.yml"}

// resolveSpecPath returns the path to the spec file. If specFile is non-empty
// it is used directly (relative paths are joined with workingDir). Otherwise
// the first existing file from SpecFileAliases in workingDir is returned.
func resolveSpecPath(specFile, workingDir string) (string, error) {
	if specFile != "" {
		if filepath.IsAbs(specFile) {
			return specFile, nil
		}
		return filepath.Join(workingDir, specFile), nil
	}
	for _, name := range SpecFileAliases {
		path := filepath.Join(workingDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", errNoSpecFile()
}

// resolveSpecPathAndCwd resolves the spec path and also returns the working directory.
func resolveSpecPathAndCwd(specFile string) (specPath, workingDir string, err error) {
	workingDir, err = os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get working directory: %w", err)
	}
	specPath, err = resolveSpecPath(specFile, workingDir)
	return
}

// resolveSpecPathFromCwd resolves the spec path using the current working directory.
func resolveSpecPathFromCwd(specFile string) (string, error) {
	specPath, _, err := resolveSpecPathAndCwd(specFile)
	return specPath, err
}
