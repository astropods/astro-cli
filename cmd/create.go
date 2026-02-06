package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
	"github.com/postman/astro/apps/astro-cli/internal/tui/create"
)

var (
	yesFlag  bool
	pathFlag string
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
  ast create my-agent --yes
  ast create my-agent --path /path/to/projects`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Accept defaults (non-interactive)")
	createCmd.Flags().StringVarP(&pathFlag, "path", "p", "", "Parent directory where the project will be created")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate name
	if err := scaffold.ValidateName(name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	// Determine target directory
	targetDir := name
	if pathFlag != "" {
		// Create the parent directory if it doesn't exist
		if err := os.MkdirAll(pathFlag, 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
		targetDir = filepath.Join(pathFlag, name)
	}

	// Validate directory doesn't exist
	if err := scaffold.ValidateDirectory(targetDir); err != nil {
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
	if err := scaffold.GenerateFiles(targetDir, config); err != nil {
		os.RemoveAll(targetDir)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	printSuccess(name, targetDir)
	return nil
}

func printSuccess(name, targetDir string) {
	fmt.Printf("\n%s✓%s Created agent %s%s%s\n\n", colorGreen, colorReset, colorBold, name, colorReset)
	fmt.Println("Next steps:")
	// Show cd command if created in a different directory
	if targetDir != name {
		fmt.Printf("  %s→%s cd %s\n", colorCyan, colorReset, targetDir)
	}
	fmt.Printf("  %s→%s cp .env.example .env\n", colorCyan, colorReset)
	fmt.Printf("  %s→%s ast dev\n", colorCyan, colorReset)
	fmt.Println()
}
