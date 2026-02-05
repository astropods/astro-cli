package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
	"github.com/postman/astro/apps/astro-cli/internal/tui/create"
)

var (
	yesFlag bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new Astro agent project",
	Long: `Create a new Astro agent project with scaffolded files.

The create command generates a new agent project with TypeScript and Bun:
- astro.yml specification file
- Dockerfile for Bun runtime
- agent/index.ts for your agent logic
- ingestion/index.ts for data pipelines

Example:
  ast create my-agent
  ast create my-agent --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Accept defaults (non-interactive)")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate name
	if err := scaffold.ValidateName(name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	// Validate directory doesn't exist
	if err := scaffold.ValidateDirectory(name); err != nil {
		return err
	}

	// Get config
	var config scaffold.ScaffoldConfig
	if yesFlag {
		config = scaffold.DefaultConfig(name)
	} else {
		var err error
		config, err = create.Run(name)
		if err != nil {
			return err
		}
	}

	// Generate files
	if err := scaffold.GenerateFiles(name, config); err != nil {
		os.RemoveAll(name)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	// Change into the created directory
	if err := os.Chdir(name); err != nil {
		return fmt.Errorf("failed to cd into %s: %w", name, err)
	}

	printSuccess(name)
	return nil
}

func printSuccess(name string) {
	fmt.Printf("\n%s✓%s Created agent %s%s%s\n\n", colorGreen, colorReset, colorBold, name, colorReset)
	fmt.Println("Next steps:")
	fmt.Printf("  %s→%s cp .env.example .env\n", colorCyan, colorReset)
	fmt.Printf("  %s→%s ast dev\n", colorCyan, colorReset)
	fmt.Println()
}
