package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version, commit, and binaryName are set at build time via ldflags.
var (
	version    = "dev"
	commit     = ""
	binaryName = "ast"
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
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = fullVersion()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringP("file", "f", "astroai.yml", "Path to astroai.yml spec file")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
}
