package cmd

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

//go:embed docs_content.md
var docsContent string

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Display Astro agent development documentation",
	Long: `Display comprehensive documentation for developing Astro agents.

This command renders the Astro development guide in your terminal,
covering project structure, the Astro DSL, and best practices.`,
	RunE: runDocs,
}

func init() {
	rootCmd.AddCommand(docsCmd)
}

func runDocs(cmd *cobra.Command, args []string) error {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	out, err := renderer.Render(docsContent)
	if err != nil {
		return fmt.Errorf("failed to render docs: %w", err)
	}

	fmt.Fprint(os.Stdout, out)
	return nil
}
