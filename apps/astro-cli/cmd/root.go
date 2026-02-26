package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version, commit, binaryName, and downloadBaseURL are set at build time via ldflags.
var (
	version         = "dev"
	commit          = ""
	binaryName      = "ast"
	downloadBaseURL = "" // e.g. https://download.astromode.ai
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
	Long: `Astro CLI is a tool for building, pushing, and developing AI agents.

It reads an astroai.yml specification file that declares:
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
	rootCmd.PersistentFlags().StringP("file", "f", "astroai.yml", "Path to astroai.yml spec file")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
}

// specFilePath returns the value of the --file persistent flag.
// It returns an error rather than silently producing an empty path if the flag
// is ever missing (e.g. due to a refactor or typo in the flag name).
func specFilePath(cmd *cobra.Command) (string, error) {
	return cmd.Root().PersistentFlags().GetString("file")
}
