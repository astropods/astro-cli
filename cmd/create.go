package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/scaffold"
	"github.com/astropods/astro-cli/internal/theme"
	"github.com/astropods/astro-cli/internal/tui"
	spec "github.com/astropods/astro-spec"
)

func exactValidProjectName(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("this command expected exactly one argument <project name>, but got %d", len(args))
	}
	if err := spec.ValidateName(args[0]); err != nil {
		return fmt.Errorf("project name %q: %w", args[0], err)
	}
	return nil
}

// parseModelFlag parses and validates a --model flag value.
// Accepted: "gateway", "anthropic", or "openai". Empty input is valid (no override).
func parseModelFlag(s string) (provider string, err error) {
	switch s {
	case "", "gateway", "anthropic", "openai":
		return s, nil
	default:
		return "", fmt.Errorf("unknown model provider %q; supported: gateway, anthropic, openai", s)
	}
}

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new Astropods agent project",
	Long: `Create a new Astropods agent project with scaffolded files.

Generates a new agent project from a template:
- astropods.yml specification file
- agent source files for your agent logic
- ingestion source files for data pipelines
- Dockerfile for the runtime

Available templates:
  mastra     TypeScript/Bun agent using Mastra (default)
  langchain  Python agent using LangChain`,
	Args: exactValidProjectName,
	RunE: runCreate,
}

func registerCreateFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP("yes", "y", false, "Accept defaults (non-interactive)")
	cmd.Flags().StringP("path", "p", "", "Parent directory where the project will be created")
	cmd.Flags().StringP("template", "t", "mastra", "Agent template (mastra, langchain)")
	cmd.Flags().Bool("force", false, "Recreate in place if directory already exists")
	cmd.Flags().StringP("model", "m", "", "LLM provider: gateway, anthropic, or openai")
	_ = cmd.RegisterFlagCompletionFunc("model", func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return []cobra.Completion{
			cobra.CompletionWithDesc("gateway", "Astro AI Gateway (managed models, no provider key)"),
			cobra.CompletionWithDesc("anthropic", "Anthropic API"),
			cobra.CompletionWithDesc("openai", "OpenAI API"),
		}, cobra.ShellCompDirectiveNoFileComp
	})
}

func initExamples(cmd string) string {
	return fmt.Sprintf(`  %[1]s my-agent
  %[1]s my-agent --model gateway
  %[1]s my-agent --model anthropic
  %[1]s my-agent --template langchain
  %[1]s my-agent --path /path/to/projects
  %[1]s my-agent --force`, cmd)
}

func init() {
	devCmd.AddCommand(createCmd)
	createCmd.Example = initExamples(buildinfo.BinaryName + " project create")
	registerCreateFlags(createCmd)

	topLevelCreateCmd := &cobra.Command{
		Use:   "create <name>",
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
	name := args[0] // validated by exactValidProjectName

	// Validate template
	if _, ok := scaffold.LangForTemplate(template); !ok {
		available := "  mastra     TypeScript/Bun agent using Mastra\n  langchain  Python agent using LangChain"
		return fmt.Errorf("unknown template: %q\n\nAvailable templates:\n%s", template, available)
	}

	// Validate model flag
	if _, err := parseModelFlag(model); err != nil {
		return err
	}

	printBanner()

	// The provider decides the models block in astropods.yml and the model the
	// generated agent uses, so ask before anything is written. --model skips this,
	// and --yes takes the default rather than blocking a non-interactive run.
	if model == "" && !yes {
		chosen, err := promptModelProvider()
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				printCancelled(cmd.OutOrStdout())
				return nil
			}
			return err
		}
		model = chosen
	}

	config := scaffold.DefaultConfig(name)
	applyModelOverride(&config, model)

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

	printSuccess(name, targetDir, config.AIGateway)
	printCodingPrompt(targetDir, config)
	return nil
}

func applyModelOverride(config *scaffold.ScaffoldConfig, modelOverride string) {
	provider, _ := parseModelFlag(modelOverride) // already validated
	switch provider {
	case "gateway":
		// Gateway supplies managed model access, so the agent needs no provider
		// API key — drop the default anthropic integration that DefaultConfig adds.
		config.AIGateway = true
		config.Integrations = slices.DeleteFunc(config.Integrations, func(s string) bool {
			return s == "anthropic" || s == "openai"
		})
	case "anthropic", "openai":
		// The choice is exclusive: drop the other model provider (DefaultConfig seeds
		// anthropic) so the spec declares one, and `project configure` asks for one
		// key rather than both.
		other := "openai"
		if provider == "openai" {
			other = "anthropic"
		}
		config.Integrations = slices.DeleteFunc(config.Integrations, func(s string) bool {
			return s == other
		})
		if !slices.Contains(config.Integrations, provider) {
			config.Integrations = append(config.Integrations, provider)
		}
	}
}

func printSuccess(name, targetDir string, aiGateway bool) {
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
	if aiGateway {
		// Gateway is account-scoped: no provider keys to set, just authenticate.
		addStep(buildinfo.BinaryName+" login", "authenticate for AI Gateway access")
	} else {
		addStep(buildinfo.BinaryName+" project configure", "set your API keys (anthropic / openai)")
	}
	addStep(buildinfo.BinaryName+" project start", "start your agent locally")

	lines = append(lines, "")
	lines = append(lines, dim.Render("Ready to ship? ")+boldPrimary.Render(buildinfo.BinaryName+" push "+name)+dim.Render(" then ")+boldPrimary.Render(buildinfo.BinaryName+" deploy "+name)+dim.Render("."))

	box := theme.Box(lines)

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
}

// promptModelProvider asks which LLM to wire up. The gateway leads because it
// needs no provider key: the platform injects the credential at deploy.
func promptModelProvider() (string, error) {
	provider := "gateway"
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which model provider should this agent use?").
			Description("Sets the models block in astropods.yml and the model the agent code uses.").
			Options(
				huh.NewOption("Astro AI Gateway  (managed models, no API key)", "gateway"),
				huh.NewOption("OpenAI           (needs OPENAI_API_KEY)", "openai"),
				huh.NewOption("Anthropic        (needs ANTHROPIC_API_KEY)", "anthropic"),
			).
			Value(&provider),
	))
	if err := runForm(form); err != nil {
		return "", err
	}
	return provider, nil
}

// printCodingPrompt emits a prompt to paste into a coding agent. It deliberately
// asks the user nothing: the goal is gathered by that agent, which can ask better
// follow-ups than a single input field.
func printCodingPrompt(targetDir string, config scaffold.ScaffoldConfig) {
	// Verify the project was actually created before printing.
	if _, err := os.Stat(filepath.Join(targetDir, "astropods.yml")); os.IsNotExist(err) {
		return
	}

	dim := lipgloss.NewStyle().Faint(true)
	prompt := buildCodingPrompt(config.Name, "")

	fmt.Println()
	fmt.Println(dim.Render("Paste this into Claude or another coding agent to get started:"))
	fmt.Println()
	fmt.Println(prompt)
	fmt.Println()
}
