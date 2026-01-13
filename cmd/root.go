package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "astro",
	Short: "Astro CLI - A tool for running astro commands",
	Long: `Astro CLI is a command-line interface for interacting with the Astro platform.

Run 'astro interactive' to launch the interactive TUI mode, or just run 'astro' without arguments.`,
	// No Run function - will show help by default
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags can be added here
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.astro.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
}
