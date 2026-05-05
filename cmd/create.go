package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	projectconfig "github.com/astropods/astro/apps/astro-cli/internal/config"
	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	"github.com/astropods/astro/apps/astro-cli/internal/tui/create"
	spec "github.com/astropods/astro/packages/astro-spec"
)

var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new Astropods agent project",
	Long: `Create a new Astropods agent project with scaffolded files.

Generates a new agent project from a template:
- astropods.yml specification file
- agent source files for your agent logic
- ingestion source files for data pipelines
- Dockerfile for the runtime

If no name is provided, you will be prompted for one interactively.

Available templates:
  mastra     TypeScript/Bun agent using Mastra (default)
  langchain  Python agent using LangChain`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCreate,
}

func registerCreateFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP("yes", "y", false, "Accept defaults (non-interactive)")
	cmd.Flags().StringP("path", "p", "", "Parent directory where the project will be created")
	cmd.Flags().StringP("template", "t", "mastra", "Agent template (mastra, langchain)")
	cmd.Flags().Bool("force", false, "Recreate in place if directory already exists")
	cmd.Flags().StringP("model", "m", "", "LLM provider: anthropic, openai, or ollama[/<model>] (e.g. ollama/llama3.3:70b)")
}

func initExamples(cmd string) string {
	return fmt.Sprintf(`  %[1]s
  %[1]s my-agent
  %[1]s my-agent --yes
  %[1]s my-agent --yes --model anthropic
  %[1]s my-agent --yes --model ollama/llama3.3:70b
  %[1]s my-agent --template langchain
  %[1]s my-agent --path /path/to/projects
  %[1]s my-agent --force`, cmd)
}

func init() {
	devCmd.AddCommand(createCmd)
	createCmd.Example = initExamples(buildinfo.BinaryName + " project create")
	registerCreateFlags(createCmd)

	topLevelCreateCmd := &cobra.Command{
		Use:   "create [name]",
		Short: createCmd.Short,
		Long:  createCmd.Long,
		Args:  createCmd.Args,
		RunE:  runCreate,
	}
	topLevelCreateCmd.Example = initExamples(buildinfo.BinaryName + " project create")
	registerCreateFlags(topLevelCreateCmd)
	rootCmd.AddCommand(topLevelCreateCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	yes := flagBool(cmd, "yes")
	path := flagString(cmd, "path")
	template := flagString(cmd, "template")
	force := flagBool(cmd, "force")
	model := flagString(cmd, "model")
	var name string
	if len(args) > 0 {
		name = args[0]
	}

	// Validate template
	if _, ok := scaffold.LangForTemplate(template); !ok {
		available := "  mastra     TypeScript/Bun agent using Mastra\n  langchain  Python agent using LangChain"
		return fmt.Errorf("unknown template: %q\n\nAvailable templates:\n%s", template, available)
	}

	// Validate model flag
	if _, _, err := create.ParseModelFlag(model); err != nil {
		return err
	}

	// If name was provided as arg, validate it upfront
	if name != "" {
		if err := spec.ValidateName(name); err != nil {
			return fmt.Errorf("invalid name: %w", err)
		}
	}

	// Get config
	var config scaffold.ScaffoldConfig
	if yes {
		if name == "" {
			return fmt.Errorf("agent name is required with --yes flag")
		}
		printBanner()
		config = scaffold.DefaultConfig(name)
		applyModelOverride(&config, model)
	} else {
		var err error
		config, err = create.Run(name, model)
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
	if path != "" {
		// Create the parent directory if it doesn't exist
		if err := os.MkdirAll(path, 0755); err != nil { //nolint:gosec
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
		targetDir = filepath.Join(path, name)
	}

	// Validate directory doesn't exist (or remove it if --force)
	if force {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	} else if err := scaffold.ValidateDirectory(targetDir); err != nil {
		return err
	}

	// Generate files
	if err := scaffold.GenerateFiles(targetDir, config, template); err != nil {
		_ = os.RemoveAll(targetDir)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	// Save any API keys collected during the interactive form
	if vars := config.CollectEnvVars(); len(vars) > 0 {
		if absDir, err := filepath.Abs(targetDir); err == nil {
			_ = projectconfig.MergeProjectVars(buildinfo.BinaryName, absDir, config.Name, vars)
		}
	}

	printSuccess(name, targetDir, yes)
	return nil
}

func applyModelOverride(config *scaffold.ScaffoldConfig, modelOverride string) {
	provider, model, _ := create.ParseModelFlag(modelOverride) // already validated
	switch provider {
	case "anthropic", "openai":
		config.Integrations = append(config.Integrations, provider)
	case "ollama":
		config.ModelProvider = "ollama"
		config.Model = model
	}
}

func printSuccess(name, targetDir string, usedDefaults bool) {
	bold := lipgloss.NewStyle().Bold(true)
	boldPrimary := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	dim := lipgloss.NewStyle().Faint(true)

	var lines []string
	lines = append(lines, "✓ "+bold.Render("Created "+name))
	lines = append(lines, "")
	lines = append(lines, bold.Render("Next steps"))
	lines = append(lines, "")

	n := 1
	addStep := func(command, desc string) {
		lines = append(lines, fmt.Sprintf("  %s  %s   %s", bold.Render(fmt.Sprintf("%d", n)), boldPrimary.Render(command), dim.Render(desc)))
		n++
	}

	addStep("cd "+targetDir, "enter the project directory")
	if usedDefaults {
		addStep(buildinfo.BinaryName+" project configure", "set your API keys")
	}
	addStep(buildinfo.BinaryName+" project start", "start your agent locally")

	lines = append(lines, "")
	lines = append(lines, dim.Render("Tip: run ")+boldPrimary.Render(buildinfo.BinaryName+" spec explain")+dim.Render(" for a plain-English breakdown of your spec."))
	lines = append(lines, dim.Render("Ready to ship? ")+boldPrimary.Render(buildinfo.BinaryName+" push "+name)+dim.Render(" then ")+boldPrimary.Render(buildinfo.BinaryName+" deploy "+name)+dim.Render("."))

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
}
