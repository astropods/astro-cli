package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/postman/astro/apps/astro-cli/ui"
	"github.com/spf13/cobra"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Launch interactive TUI mode",
	Long:  `Launch the Astro CLI in interactive mode with a beautiful terminal user interface.`,
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running interactive mode: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}
