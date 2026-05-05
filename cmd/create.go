package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
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

// ollamaModelList is a curated list of popular models from the Ollama library.
// TODO (in the far future): Automate me!
var ollamaModelList = []string{
	"llama3.3:70b",
	"llama3.2:3b",
	"llama3.2:1b",
	"llama3.1:8b",
	"llama3.1:70b",
	"deepseek-r1:7b",
	"deepseek-r1:14b",
	"deepseek-r1:70b",
	"gemma3:4b",
	"gemma3:12b",
	"gemma3:27b",
	"mistral:7b",
	"mistral-small:24b",
	"qwen3:8b",
	"qwen3:14b",
	"qwen3:32b",
	"qwen2.5:7b",
	"qwen2.5:14b",
	"qwen2.5-coder:7b",
	"phi4:14b",
	"phi4-mini:3.8b",
	"codellama:7b",
	"codellama:13b",
	"nomic-embed-text",
	"mxbai-embed-large",
}

// parseModelFlag parses and validates a --model flag value.
// Accepted: "anthropic", "openai", "ollama", or "ollama/<model>" where <model> must be in the known list.
func parseModelFlag(s string) (provider, model string, err error) {
	if s == "" {
		return
	}
	parts := strings.SplitN(s, "/", 2)
	provider = parts[0]
	switch provider {
	case "anthropic", "openai":
		// valid, no specific model
	case "ollama":
		if len(parts) == 2 {
			model = parts[1]
			if !slices.Contains(ollamaModelList, model) {
				return "", "", fmt.Errorf("unknown ollama model %q\n\nKnown models:\n  %s",
					model, strings.Join(ollamaModelList, "\n  "))
			}
		}
	default:
		return "", "", fmt.Errorf("unknown model provider %q; supported: anthropic, openai, ollama[/<model>]", provider)
	}
	return
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
	cmd.Flags().StringP("model", "m", "", "LLM provider: anthropic, openai, or ollama[/<model>] (e.g. ollama/llama3.3:70b)")
	_ = cmd.RegisterFlagCompletionFunc("model", func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if modelPart, ok := strings.CutPrefix(toComplete, "ollama/"); ok {
			completions := make([]cobra.Completion, 0, len(ollamaModelList))
			if namePrefix, tagPrefix, hasColon := strings.Cut(modelPart, ":"); hasColon {
				// Colon is a word separator in zsh/bash. When the user has typed
				// "ollama/<name>:" the shell replaces only the fragment after ":".
				// Return just the tag suffix so it doesn't get doubled.
				for _, m := range ollamaModelList {
					if name, tag, ok := strings.Cut(m, ":"); ok && name == namePrefix && strings.HasPrefix(tag, tagPrefix) {
						completions = append(completions, cobra.CompletionWithDesc(tag, ""))
					}
				}
			} else {
				for _, m := range ollamaModelList {
					completions = append(completions, cobra.CompletionWithDesc("ollama/"+m, ""))
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		}
		return []cobra.Completion{
			cobra.CompletionWithDesc("anthropic", "Anthropic API"),
			cobra.CompletionWithDesc("openai", "OpenAI API"),
			cobra.CompletionWithDesc("ollama", "self-hosted (use ollama/<model> to pin a specific model)"),
		}, cobra.ShellCompDirectiveNoFileComp
	})
}

func initExamples(cmd string) string {
	return fmt.Sprintf(`  %[1]s my-agent
  %[1]s my-agent --model anthropic
  %[1]s my-agent --model ollama/llama3.3:70b
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
	if _, _, err := parseModelFlag(model); err != nil {
		return err
	}

	printBanner()
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

	printSuccess(name, targetDir)
	printCodingPrompt(targetDir, config, yes)
	return nil
}

func applyModelOverride(config *scaffold.ScaffoldConfig, modelOverride string) {
	provider, model, _ := parseModelFlag(modelOverride) // already validated
	switch provider {
	case "anthropic", "openai":
		if !slices.Contains(config.Integrations, provider) {
			config.Integrations = append(config.Integrations, provider)
		}
	case "ollama":
		config.ModelProvider = "ollama"
		config.Model = model
	}
}

func printSuccess(name, targetDir string) {
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
	addStep(buildinfo.BinaryName+" project configure", "set your API keys (anthropic / openai)")
	addStep(buildinfo.BinaryName+" project start", "start your agent locally")

	lines = append(lines, "")
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

func printCodingPrompt(targetDir string, config scaffold.ScaffoldConfig, skipPrompt bool) {
	// Verify the project was actually created before prompting.
	if _, err := os.Stat(filepath.Join(targetDir, "astropods.yml")); os.IsNotExist(err) {
		return
	}

	dim := lipgloss.NewStyle().Faint(true)

	var goal string
	if !skipPrompt {
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("What should " + config.Name + " do?").
				Description("Describe the agent logic and we'll build a prompt for your coding agent.").
				Value(&goal),
		)).WithTheme(cliHuhTheme())
		if err := form.Run(); err != nil && !errors.Is(err, huh.ErrUserAborted) {
			return
		}
		goal = strings.TrimSpace(goal)
	}

	prompt := buildCodingPrompt(config.Name, goal)

	fmt.Println()
	fmt.Println(dim.Render("Paste this into Claude or another coding agent to get started:"))
	fmt.Println()
	fmt.Println(prompt)
	fmt.Println()
}
