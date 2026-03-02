package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/buildinfo"
)

// version, commit, and downloadBaseURL are set at build time via ldflags.
var (
	version         = "dev"
	commit          = ""
	binaryName      = buildinfo.BinaryName
	downloadBaseURL = "" // e.g. https://download.astropods.ai
)

func fullVersion() string {
	if commit != "" {
		return binaryName + "/" + version + " (" + commit + ")"
	}
	return binaryName + "/" + version
}

var rootCmd = &cobra.Command{
	Use:   binaryName,
	Short: "Astro CLI - Build, push, and develop AI agents",
	Long: astroBanner() + `

Build, push, and develop AI agents.

It reads an astropods.yml specification file that declares:
- Self-hosted components (models, knowledge stores, tools)
- Cloud integrations (Anthropic, GitHub, etc.)
- Interfaces (Slack, HTTP API)
- Data ingestion pipelines`,
	Version:       "placeholder",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	// Identify the subcommand before executing so we can skip the version
	// check for commands that manage the CLI itself.
	invoked, _, _ := rootCmd.Find(os.Args[1:])

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if invoked == nil || invoked.Name() != "upgrade" {
		notifyIfUpdateAvailable()
	}
}

func init() {
	rootCmd.Version = fullVersion()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringP("file", "f", "astropods.yml", "/path/to/astropods.yml")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
}

// specFilePath returns the value of the --file persistent flag.
// It returns an error rather than silently producing an empty path if the flag
// is ever missing (e.g. due to a refactor or typo in the flag name).
func specFilePath(cmd *cobra.Command) (string, error) {
	return cmd.Root().PersistentFlags().GetString("file")
}

// SpecFileAliases are filenames checked in order when the user does not pass --file.
var SpecFileAliases = []string{"astropods.yml", "astropods.yaml", "astroai.yml", "astro.yml"}

// resolveSpecPath returns the path to the spec file. If the user passed --file
// explicitly, that path is used (relative paths are joined with workingDir).
// Otherwise, the first existing file from SpecFileAliases in workingDir is returned.
func resolveSpecPath(cmd *cobra.Command, workingDir string) (string, error) {
	specFile, err := specFilePath(cmd)
	if err != nil {
		return "", err
	}
	if cmd.Root().PersistentFlags().Changed("file") {
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
	return "", fmt.Errorf("no spec file found (try: %s)", strings.Join(SpecFileAliases, ", "))
}

// resolveSpecPathFromCwd resolves the spec path using the current working directory.
// It is the convenience wrapper used by commands.
func resolveSpecPathFromCwd(cmd *cobra.Command) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return resolveSpecPath(cmd, workingDir)
}
