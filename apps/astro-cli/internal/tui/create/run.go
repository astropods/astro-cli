package create

import (
	"errors"
	"fmt"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
)

// ollamaModels returns a curated list of popular models from the Ollama library.
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

// Run launches the interactive huh form and returns the user's configuration.
func Run(name string) (scaffold.ScaffoldConfig, error) {
	var (
		description string
		interfaces  = []string{"web"}
		models      = []string{"ollama"} // "ollama", "anthropic", "openai"
		ollamaModel = ollamaModelList[0]
		knowledge   []string
		tools       []string
		ingestion   []string

		// API keys
		anthropicKey  string
		openaiKey     string
		githubToken   string
		slackBotToken string
		slackAppToken string

		confirm = true
	)

	hasOllama    := func() bool { return slices.Contains(models, "ollama") }
	hasAnthropic := func() bool { return slices.Contains(models, "anthropic") }
	hasOpenAI    := func() bool { return slices.Contains(models, "openai") }
	hasGitHub    := func() bool { return slices.Contains(tools, "github") }
	hasSlack     := func() bool { return slices.Contains(interfaces, "slack") }

	needsName := name == ""

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Agent name").
				Description("Lowercase letters, numbers, and hyphens. Must start with a letter.").
				Placeholder("my-agent").
				Value(&name).
				Validate(scaffold.ValidateName),
		).WithHideFunc(func() bool { return !needsName }),

		huh.NewGroup(
			huh.NewInput().
				Title("Description").
				Description("A short summary of what your agent does.").
				Placeholder("Summarizes weekly workspace activity and highlights projects that need attention.").
				Value(&description),
		),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Interfaces").
				Description("How users interact with your agent.").
				Options(
					huh.NewOption("Web (HTTP/SSE)", "web"),
					huh.NewOption("Slack", "slack"),
				).
				Value(&interfaces).
				Validate(func(v []string) error {
					if len(v) == 0 {
						return fmt.Errorf("select at least one interface")
					}
					return nil
				}),
		),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("LLM providers").
				Description("Ollama runs models locally. Cloud providers (Anthropic, OpenAI) are added as API integrations and can be used alongside a local model.").
				Options(
					huh.NewOption("Ollama (self-hosted)", "ollama"),
					huh.NewOption("Anthropic", "anthropic"),
					huh.NewOption("OpenAI", "openai"),
				).
				Value(&models),
		),

		// Ollama model — only shown when Ollama is selected
		huh.NewGroup(
			huh.NewInput().
				Title("Ollama model").
				Description("Type to filter or enter any model name from the Ollama library.").
				Suggestions(ollamaModelList).
				Value(&ollamaModel).
				Validate(func(v string) error {
					if v == "" {
						return fmt.Errorf("model name is required")
					}
					return nil
				}),
		).WithHideFunc(func() bool { return !hasOllama() }),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Knowledge").
				Description("Vector stores, caches, and graph DBs for your agent.").
				Options(
					huh.NewOption("Qdrant (vector store)", "qdrant"),
					huh.NewOption("Redis (key-value)", "redis"),
					huh.NewOption("Neo4j (graph)", "neo4j"),
				).
				Value(&knowledge),
		),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Tools").
				Description("Tool integrations for your agent.").
				Options(
					huh.NewOption("GitHub", "github"),
				).
				Value(&tools),
		),

		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Data ingestion").
				Description("How to trigger your data pipeline that populates knowledge stores.").
				Options(
					huh.NewOption("Scheduled (cron)", "schedule"),
					huh.NewOption("Webhook", "webhook"),
					huh.NewOption("Manual trigger", "manual"),
					huh.NewOption("On startup", "startup"),
				).
				Value(&ingestion),
		),

		// API key groups — each conditionally shown based on selections above
		huh.NewGroup(
			huh.NewInput().
				Title("Anthropic API key").
				Description("Will be saved as ANTHROPIC_API_KEY in .env — you can also set it later.").
				EchoMode(huh.EchoModePassword).
				Value(&anthropicKey),
		).WithHideFunc(func() bool { return !hasAnthropic() }),

		huh.NewGroup(
			huh.NewInput().
				Title("OpenAI API key").
				Description("Will be saved as OPENAI_API_KEY in .env — you can also set it later.").
				EchoMode(huh.EchoModePassword).
				Value(&openaiKey),
		).WithHideFunc(func() bool { return !hasOpenAI() }),

		huh.NewGroup(
			huh.NewInput().
				Title("GitHub token").
				Description("Will be saved as GITHUB_TOKEN in .env — you can also set it later.").
				EchoMode(huh.EchoModePassword).
				Value(&githubToken),
		).WithHideFunc(func() bool { return !hasGitHub() }),

		huh.NewGroup(
			huh.NewInput().
				Title("Slack bot token").
				Description("Will be saved as SLACK_BOT_TOKEN in .env — you can also set it later.").
				EchoMode(huh.EchoModePassword).
				Value(&slackBotToken),
			huh.NewInput().
				Title("Slack app token").
				Description("Will be saved as SLACK_APP_TOKEN in .env — you can also set it later.").
				EchoMode(huh.EchoModePassword).
				Value(&slackAppToken),
		).WithHideFunc(func() bool { return !hasSlack() }),

		huh.NewGroup(
			huh.NewConfirm().
				TitleFunc(func() string { return fmt.Sprintf("Create agent %q?", name) }, &name).
				Value(&confirm),
		),
	)

	teal := lipgloss.Color("6")
	theme := huh.ThemeCharm()
	theme.Focused.Title = theme.Focused.Title.Foreground(teal)
	theme.Focused.SelectedOption = theme.Focused.SelectedOption.Foreground(teal)
	theme.Focused.SelectedPrefix = theme.Focused.SelectedPrefix.Foreground(teal)
	theme.Focused.FocusedButton = theme.Focused.FocusedButton.Background(teal)
	theme.Focused.SelectSelector = theme.Focused.SelectSelector.Foreground(teal)
	theme.Focused.MultiSelectSelector = theme.Focused.MultiSelectSelector.Foreground(teal)
	theme.Focused.TextInput.Cursor = theme.Focused.TextInput.Cursor.Foreground(teal)
	theme.Focused.TextInput.Prompt = theme.Focused.TextInput.Prompt.Foreground(teal)
	theme.Focused.Next = theme.Focused.Next.Foreground(teal)
	theme.Blurred.Title = theme.Blurred.Title.Foreground(teal)
	theme.Blurred.SelectedOption = theme.Blurred.SelectedOption.Foreground(teal)
	theme.Blurred.SelectedPrefix = theme.Blurred.SelectedPrefix.Foreground(teal)

	if err := form.WithTheme(theme).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return scaffold.ScaffoldConfig{}, ErrCancelled
		}
		return scaffold.ScaffoldConfig{}, err
	}

	if !confirm {
		return scaffold.ScaffoldConfig{}, ErrCancelled
	}

	return buildConfig(formInputs{
		name:          name,
		description:   description,
		interfaces:    interfaces,
		models:        models,
		ollamaModel:   ollamaModel,
		knowledge:     knowledge,
		tools:         tools,
		ingestion:     ingestion,
		anthropicKey:  anthropicKey,
		openaiKey:     openaiKey,
		githubToken:   githubToken,
		slackBotToken: slackBotToken,
		slackAppToken: slackAppToken,
	}), nil
}

type formInputs struct {
	name          string
	description   string
	interfaces    []string
	models        []string
	ollamaModel   string
	knowledge     []string
	tools         []string
	ingestion     []string
	anthropicKey  string
	openaiKey     string
	githubToken   string
	slackBotToken string
	slackAppToken string
}

func buildConfig(in formInputs) scaffold.ScaffoldConfig {
	config := scaffold.ScaffoldConfig{
		Name:            in.name,
		Description:     in.description,
		Interfaces:      in.interfaces,
		Knowledge:       in.knowledge,
		Ingestions:      in.ingestion,
		IntegrationKeys: map[string]string{},
	}

	if slices.Contains(in.models, "ollama") {
		config.ModelProvider = "ollama"
		config.Model = in.ollamaModel
	}

	for _, m := range in.models {
		if m == "anthropic" || m == "openai" {
			config.Integrations = append(config.Integrations, m)
		}
	}
	config.Integrations = append(config.Integrations, in.tools...)

	if in.anthropicKey != "" {
		config.IntegrationKeys["anthropic"] = in.anthropicKey
	}
	if in.openaiKey != "" {
		config.IntegrationKeys["openai"] = in.openaiKey
	}
	if in.githubToken != "" {
		config.IntegrationKeys["github"] = in.githubToken
	}
	if in.slackBotToken != "" {
		config.IntegrationKeys["slack_bot_token"] = in.slackBotToken
	}
	if in.slackAppToken != "" {
		config.IntegrationKeys["slack_app_token"] = in.slackAppToken
	}

	return config
}

// ErrCancelled is returned when the user aborts the form or answers No to the confirm prompt.
var ErrCancelled = errors.New("cancelled")
