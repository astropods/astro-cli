package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	projectconfig "github.com/astropods/astro/apps/astro-cli/internal/config"
	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
	"github.com/astropods/astro/apps/astro-cli/internal/tui/create"
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
	Use:   "create [name]",
	Short: "Create a new Astro agent project",
	Long: `Create a new Astro agent project with scaffolded files.

The create command generates a new agent project with the specified language:
- astropods.yml specification file
- agent source files for your agent logic
- ingestion source files for data pipelines
- Dockerfile for the runtime

If no name is provided, you will be prompted for one interactively.

Supported languages: ts (TypeScript/Bun)
Supported templates: mastra (default)

Example:
  ast create
  ast create my-agent
  ast create my-agent --yes
  ast create my-agent --template mastra
  ast create my-agent --lang ts
  ast create my-agent --path /path/to/projects
  ast create my-agent --force`,
	Args: cobra.MaximumNArgs(1),
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
	var name string
	if len(args) > 0 {
		name = args[0]
	}

	// Validate language
	if !supportedLangs[langFlag] {
		return fmt.Errorf("unsupported language: %s (supported: ts)", langFlag)
	}

	// Validate template
	if !supportedTemplates[templateFlag] {
		return fmt.Errorf("unsupported template: %s (supported: mastra)", templateFlag)
	}

	// If name was provided as arg, validate it upfront
	if name != "" {
		if err := scaffold.ValidateName(name); err != nil {
			return fmt.Errorf("invalid name: %w", err)
		}
	}

	// Get config
	var config scaffold.ScaffoldConfig
	if yesFlag {
		if name == "" {
			return fmt.Errorf("agent name is required with --yes flag")
		}
		printBanner()
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
		name = config.Name
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

	// Generate files
	if err := scaffold.GenerateFiles(targetDir, config, langFlag, templateFlag); err != nil {
		_ = os.RemoveAll(targetDir)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	// Save any API keys collected during the interactive form
	if vars := config.CollectEnvVars(); len(vars) > 0 {
		if absDir, err := filepath.Abs(targetDir); err == nil {
			_ = projectconfig.MergeProjectVars(binaryName, absDir, config.Name, vars)
		}
	}

	printSuccess(name, targetDir, yesFlag)
	return nil
}

func printSuccess(name, targetDir string, usedDefaults bool) {
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	cmd := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	step := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	heading := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))

	var lines []string
	lines = append(lines, heading.Render("✓ Created "+name))
	lines = append(lines, "")
	lines = append(lines, bold.Render("Next steps"))
	lines = append(lines, "")

	n := 1
	addStep := func(command, desc string) {
		lines = append(lines, fmt.Sprintf("  %s  %s   %s", step.Render(fmt.Sprintf("%d", n)), cmd.Render(command), dim.Render(desc)))
		n++
	}

	addStep("cd "+targetDir, "enter the project directory")
	if usedDefaults {
		addStep("ast configure", "set your API keys")
	}
	addStep("ast dev", "start your agent locally")

	lines = append(lines, "")
	lines = append(lines, dim.Render("Tip: run ")+cmd.Render("ast explain")+dim.Render(" for a plain-English breakdown of your spec."))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
}
