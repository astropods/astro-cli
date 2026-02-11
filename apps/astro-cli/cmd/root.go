package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version and commit are set at build time via ldflags.
var (
	version = "dev"
	commit  = ""
)

func fullVersion() string {
	if commit != "" {
		return "ast/" + version + " (" + commit + ")"
	}
	return "ast/" + version
}

var rootCmd = &cobra.Command{
	Use:   "ast",
	Short: "Astro CLI - Build, publish, and develop AI agents",
	Long: `Astro CLI is a tool for building, publishing, and developing AI agents.

It reads an astro.yml specification file that declares:
- Self-hosted components (models, knowledge stores, tools)
- Cloud integrations (Anthropic, GitHub, etc.)
- Interfaces (Slack, HTTP API)
- Data injection pipelines`,
	Version:       "placeholder",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = fullVersion()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringP("file", "f", "astro.yml", "Path to astro.yml spec file")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
}
