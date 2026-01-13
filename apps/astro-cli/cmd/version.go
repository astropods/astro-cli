package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	commit  = "dev"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Astro CLI",
	Long:  `Display version information including build commit and date.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Astro CLI v%s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built: %s\n", date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
