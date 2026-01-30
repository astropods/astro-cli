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
  astro create my-agent
  astro create my-agent --yes`,
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

	printSuccess(name, config)
	return nil
}

func printSuccess(name string, config scaffold.ScaffoldConfig) {
	fmt.Printf("\n✅ Created agent '%s' in ./%s/\n\n", name, name)
	fmt.Println("Files generated:")
	fmt.Println("  - astro.yml")
	fmt.Println("  - Dockerfile")
	if config.Ingestion != "none" {
		fmt.Println("  - Dockerfile.ingestion")
	}
	fmt.Println("  - package.json")
	fmt.Println("  - tsconfig.json")
	fmt.Println("  - .env.example")
	fmt.Println("  - .gitignore")
	fmt.Println("  - .dockerignore")
	fmt.Println("  - agent/index.ts")
	fmt.Println("  - ingestion/index.ts")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. cd %s\n", name)
	fmt.Println("  2. bun install")
	fmt.Println("  3. Edit agent/index.ts with your agent logic")
	fmt.Println("  4. Edit ingestion/index.ts for data pipelines")
	fmt.Println("  5. Copy .env.example to .env and add credentials")
	fmt.Println("  6. Run: astro dev")
	fmt.Println()
}
