package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	cmd.Flags().StringP("description", "d", "", "One sentence on what the agent does, used for its instructions")
	cmd.Flags().Bool("no-git", false, "Skip git repository initialization")
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
  %[1]s my-agent --description "Summarise tech talks"
  %[1]s my-agent --model gateway
  %[1]s my-agent --model anthropic
  %[1]s my-agent --template langchain
  %[1]s my-agent --path /path/to/projects
  %[1]s my-agent --no-git
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
	noGit := flagBool(cmd, "no-git")
	description := normalizeDescription(flagString(cmd, "description"))
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

	// Validate description flag
	if err := validateDescription(description); err != nil {
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

	// Validate directory doesn't exist (--force recreates it in place below)
	if !force {
		if err := scaffold.ValidateDirectory(targetDir); err != nil {
			return err
		}
	}

	// Prompt before any write or removal, so cancelling leaves the disk untouched.
	if description == "" && !yes && interactiveTerminal() {
		answer, err := promptDescription(name)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				printCancelled(cmd.OutOrStdout())
				return nil
			}
			return err
		}
		description = answer
	}
	config.Description = description

	if force {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}

	// Generate files
	if err := scaffold.GenerateFiles(targetDir, config, template); err != nil {
		_ = os.RemoveAll(targetDir)
		return fmt.Errorf("failed to generate files: %w", err)
	}

	w := cmd.OutOrStdout()
	printSuccess(w, name, targetDir, config.AIGateway)
	if !noGit {
		initGitRepo(cmd.Context(), w, targetDir)
	}
	printCodingPrompt(w, targetDir, config)
	return nil
}

// normalizeDescription collapses whitespace and terminates the sentence, so one
// description reads the same in the agent card, the docs, and the agent's prompt.
// The period is dropped rather than pushing a legal description over the limit.
func normalizeDescription(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	if len([]rune(s)) >= spec.MaxDescriptionLength {
		return s
	}
	return s + "."
}

// validateDescription rejects a description the agent card would truncate.
func validateDescription(s string) error {
	if n := len([]rune(s)); n > spec.MaxDescriptionLength {
		return errDescriptionTooLong(n)
	}
	return nil
}

// promptDescription asks what the agent should do. An empty answer is valid:
// the scaffold then omits the description rather than inventing one.
func promptDescription(name string) (string, error) {
	var description string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(msgDescribeAgentTitle(name)).
			Description(msgDescribeAgentHelp()).
			Value(&description).
			Validate(func(s string) error { return validateDescription(normalizeDescription(s)) }),
	))
	if err := runForm(form); err != nil {
		return "", err
	}
	return normalizeDescription(description), nil
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
		if !slices.Contains(config.Integrations, provider) {
			config.Integrations = append(config.Integrations, provider)
		}
	}
}

func printSuccess(w io.Writer, name, targetDir string, aiGateway bool) {
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

	fmt.Fprintln(w)      //nolint:errcheck,gosec
	fmt.Fprintln(w, box) //nolint:errcheck,gosec
	fmt.Fprintln(w)      //nolint:errcheck,gosec
}

func printCodingPrompt(w io.Writer, targetDir string, config scaffold.ScaffoldConfig) {
	// Verify the project was actually created before printing the prompt.
	if _, err := os.Stat(filepath.Join(targetDir, "astropods.yml")); os.IsNotExist(err) {
		return
	}

	dim := lipgloss.NewStyle().Faint(true)
	prompt := buildCodingPrompt(config.Name, config.Description)

	fmt.Fprintln(w)                                      //nolint:errcheck,gosec
	fmt.Fprintln(w, dim.Render(msgPasteToCodingAgent())) //nolint:errcheck,gosec
	fmt.Fprintln(w)                                      //nolint:errcheck,gosec
	fmt.Fprintln(w, prompt)                              //nolint:errcheck,gosec
	fmt.Fprintln(w)                                      //nolint:errcheck,gosec
}
