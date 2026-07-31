package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

//go:embed docs/agent_instructions.md
var docsAgent string

//go:embed docs/ast.md
var docsHelp string

var docsCmd = &cobra.Command{
	Use:     "docs [category]",
	Aliases: []string{"doc"},
	Short:   "Display Astropods documentation",
	Long: `Display documentation in your terminal.

Categories:
  agent  Agent development guide (LLM, tools, messaging). Default.
  help   CLI help: installation, quick start, commands, spec.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDocs,
}

func init() {
	rootCmd.AddCommand(docsCmd)
}

func runDocs(cmd *cobra.Command, args []string) error {
	category := "agent"
	if len(args) > 0 {
		category = strings.ToLower(strings.TrimSpace(args[0]))
	}

	var content string
	switch category {
	case "agent":
		content = docsAgent
	case "help":
		content = docsHelp
	default:
		return fmt.Errorf("unknown category %q (use agent or help)", category)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	out, err := renderer.Render(content)
	if err != nil {
		return fmt.Errorf("failed to render docs: %w", err)
	}

	_, _ = fmt.Fprint(os.Stdout, out)
	return nil
}
