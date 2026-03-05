package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	serverAddr string
)

// WebFS is set by main.go from the embedded filesystem.
var WebFS embed.FS

// OpenAPIJSON is set by main.go from the embedded OpenAPI spec.
var OpenAPIJSON []byte

const beeArt = `
        \     /
     \  .\---./  /
      \/ o   o \/
      ( _  ^  _ )
       |/ \Y/ \|
       ()  |  ()
        |  |  |
       _|  |  |_
      (___/ \___)
`

var rootCmd = &cobra.Command{
	Use:   "queen",
	Short: "queen - Astro admin toolkit",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(beeArt)
		fmt.Println("  queen - Astro admin toolkit")
		fmt.Println()
		fmt.Println("  Available commands:")
		fmt.Println()

		for _, c := range cmd.Commands() {
			if c.Hidden || !c.IsAvailableCommand() {
				continue
			}
			fmt.Printf("    %-20s %s\n", c.Name(), c.Short)
		}

		fmt.Println()
		fmt.Println("  Use \"queen <command> --help\" for more information about a command.")
		fmt.Println()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default ~/.astro-queen/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "gRPC server address (overrides config)")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
