package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	serverAddr string
)

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
	Short: "🐝 queen – Astro admin CLI & TUI toolkit",
	Run: func(cmd *cobra.Command, args []string) {
		bee := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render(beeArt)
		title := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render("queen 🐝")
		subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Astro admin CLI & TUI toolkit")

		fmt.Println(bee)
		fmt.Printf("  %s  %s\n\n", title, subtitle)
		fmt.Println("  Available commands:")
		fmt.Println()

		for _, c := range cmd.Commands() {
			if c.Hidden || !c.IsAvailableCommand() {
				continue
			}
			name := lipgloss.NewStyle().Foreground(lipgloss.Color("79")).Bold(true).Render(c.Name())
			fmt.Printf("    %-28s %s\n", name, c.Short)
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
