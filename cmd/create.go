package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
	"github.com/postman/astro/apps/astro-cli/internal/tui/create"
)

var (
	yesFlag      bool
	pathFlag     string
	langFlag     string
	templateFlag string
	forceFlag    bool
)

// Supported languages for project templates
var supportedLangs = map[string]bool{
	"ts": true,
}

// Supported template types
var supportedTemplates = map[string]bool{
	"mastra": true,
}

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new Astro agent project",
	Long: `Create a new Astro agent project with scaffolded files.

The create command generates a new agent project with the specified language:
- astropods.yml specification file
- agent source files for your agent logic
- ingestion source files for data pipelines
- Dockerfile for the runtime

Supported languages: ts (TypeScript/Bun)
Supported templates: mastra (default)

Example:
  ast create my-agent
  ast create my-agent --yes
  ast create my-agent --template mastra
  ast create my-agent --lang ts
  ast create my-agent --path /path/to/projects
  ast create my-agent --force`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Accept defaults (non-interactive)")
	createCmd.Flags().StringVarP(&pathFlag, "path", "p", "", "Parent directory where the project will be created")
	createCmd.Flags().StringVarP(&langFlag, "lang", "l", "ts", "Project language template (ts)")
	createCmd.Flags().StringVarP(&templateFlag, "template", "t", "mastra", "Agent template (mastra)")
	createCmd.Flags().BoolVar(&forceFlag, "force", false, "Recreate in place if directory already exists")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate language
	if !supportedLangs[langFlag] {
		return fmt.Errorf("unsupported language: %s (supported: ts)", langFlag)
	}

	// Validate template
	if !supportedTemplates[templateFlag] {
		return fmt.Errorf("unsupported template: %s (supported: mastra)", templateFlag)
	}

	// Validate name
	if err := scaffold.ValidateName(name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	// Determine target directory
	targetDir := name
	if pathFlag != "" {
		// Create the parent directory if it doesn't exist
		if err := os.MkdirAll(pathFlag, 0755); err != nil { //nolint:gosec
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
		targetDir = filepath.Join(pathFlag, name)
	}

	// Validate directory doesn't exist (or remove it if --force)
	if forceFlag {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	} else if err := scaffold.ValidateDirectory(targetDir); err != nil {
		return err
	}

	// Get config
	var config scaffold.ScaffoldConfig
	if yesFlag {
		config = scaffold.DefaultConfig(name)
	} else {
		var err error
		config, err = create.Run(name)
		if errors.Is(err, create.ErrCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
	}

	// Generate files
	if err := scaffold.GenerateFiles(targetDir, config, langFlag, templateFlag); err != nil {
		_ = os.RemoveAll(targetDir)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	printSuccess(name, targetDir)
	return nil
}

func printSuccess(name, targetDir string) {
	fmt.Printf("\n%s%s✓ Created agent %s%s\n\n", colorGreen, colorBold, name, colorReset)
	fmt.Printf("  %s$ cd %s%s\n\n", colorYellow, targetDir, colorReset)
	fmt.Printf("  %s%-12s%s  captures everything you configured — infrastructure, models, and integrations.\n", colorBold, "astropods.yml", colorReset)
	fmt.Printf("  %s%-12s%s  holds the secrets you provided. Add any missing ones before starting.\n\n", colorBold, ".env", colorReset)
	fmt.Printf("  When ready, run %s%sast dev%s to start your agent locally.\n\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  %sTip:%s run %sast explain%s for a plain-English breakdown of your agent spec.\n\n", colorDim, colorReset, colorCyan, colorReset)
}
