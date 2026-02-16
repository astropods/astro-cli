package create

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
)

type screen int

const (
	screenDescription screen = iota
	screenInterface
	screenModel
	screenModelName
	screenKnowledge
	screenIntegrations
	screenIntegrationKey
	screenIngestion
	screenConfirm
)

type option struct {
	label string
	value string
}

type model struct {
	name   string
	screen screen
	config scaffold.ScaffoldConfig

	// Text input for description
	descInput textinput.Model
	// Text input for ollama model name
	modelInput textinput.Model
	// Masked input for integration API keys
	keyInput textinput.Model

	// Select state
	options  []option
	cursor   int
	selected map[int]bool // for multi-select

	// Integration key collection state
	pendingKeys []string // integrations that need keys
	keyIndex    int      // current index into pendingKeys

	// State
	done     bool
	quitting bool
}

func initialModel(name string) model {
	ti := textinput.New()
	ti.Placeholder = "An AI-powered agent"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 60

	return model{
		name:      name,
		screen:    screenDescription,
		descInput: ti,
		config: scaffold.ScaffoldConfig{
			Name:            name,
			Interfaces:      []string{},
			Integrations:    []string{},
			IntegrationKeys: map[string]string{},
		},
		selected: make(map[int]bool),
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.screen != screenDescription {
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	switch m.screen {
	case screenDescription:
		return m.updateDescription(msg)
	case screenInterface:
		return m.updateInterface(msg)
	case screenModel:
		return m.updateModel(msg)
	case screenModelName:
		return m.updateModelName(msg)
	case screenKnowledge:
		return m.updateKnowledge(msg)
	case screenIntegrations:
		return m.updateIntegrations(msg)
	case screenIntegrationKey:
		return m.updateIntegrationKey(msg)
	case screenIngestion:
		return m.updateIngestion(msg)
	case screenConfirm:
		return m.updateConfirm(msg)
	}

	return m, nil
}

func (m model) updateDescription(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			desc := m.descInput.Value()
			if desc == "" {
				desc = "An AI-powered agent"
			}
			m.config.Description = desc
			m.screen = screenInterface
			m.cursor = 0
			m.selected = make(map[int]bool)
			m.selected[0] = true // Pre-select "Web" (first option)
			m.options = []option{
				{"Web (HTTP/SSE)", "web"},
				{"Slack", "slack"},
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.descInput, cmd = m.descInput.Update(msg)
	return m, cmd
}

func (m model) updateInterface(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Web (HTTP/SSE)", "web"},
		{"Slack", "slack"},
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			interfaces := []string{}
			for i, opt := range m.options {
				if m.selected[i] {
					interfaces = append(interfaces, opt.value)
				}
			}
			// Default to web if nothing selected
			if len(interfaces) == 0 {
				interfaces = []string{"web"}
			}
			m.config.Interfaces = interfaces
			m.screen = screenModel
			m.cursor = 0
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Ollama", "ollama"},
		{"Hugging Face", "huggingface"},
		{"None", "none"},
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			provider := m.options[m.cursor].value
			if provider == "none" {
				m.config.ModelProvider = ""
				m.config.Model = ""
				m.screen = screenKnowledge
				m.cursor = 0
				m.selected = make(map[int]bool)
				return m, nil
			}
			m.config.ModelProvider = provider
			ti := textinput.New()
			ti.Placeholder = "e.g. llama3, mistral"
			ti.Width = 60
			m.modelInput = ti
			m.screen = screenModelName
			return m, m.modelInput.Focus()
		}
	}

	return m, nil
}

func (m model) updateModelName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			m.config.Model = strings.TrimSpace(m.modelInput.Value())
			m.screen = screenKnowledge
			m.cursor = 0
			m.selected = make(map[int]bool)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.modelInput, cmd = m.modelInput.Update(msg)
	return m, cmd
}

func (m model) updateKnowledge(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Qdrant (vector store)", "qdrant"},
		{"Redis (key-value store)", "redis"},
		{"Neo4j (graph database)", "neo4j"},
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			knowledge := []string{}
			for i, opt := range m.options {
				if m.selected[i] {
					knowledge = append(knowledge, opt.value)
				}
			}
			m.config.Knowledge = knowledge
			m.screen = screenIntegrations
			m.cursor = 0
			m.selected = make(map[int]bool)
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateIntegrations(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Anthropic", "anthropic"},
		{"OpenAI", "openai"},
		{"GitHub", "github"},
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			integrations := []string{}
			for i, opt := range m.options {
				if m.selected[i] {
					integrations = append(integrations, opt.value)
				}
			}
			m.config.Integrations = integrations
			// Collect keys for integrations and interfaces that need them
			m.pendingKeys = nil
			// Slack interface tokens
			for _, iface := range m.config.Interfaces {
				if iface == "slack" {
					m.pendingKeys = append(m.pendingKeys, "slack_bot_token", "slack_app_token")
				}
			}
			// Integration API keys
			for _, name := range integrations {
				if _, ok := integrationKeyEnvVar[name]; ok {
					m.pendingKeys = append(m.pendingKeys, name)
				}
			}
			if len(m.pendingKeys) > 0 {
				m.keyIndex = 0
				m.keyInput = newKeyInput()
				m.screen = screenIntegrationKey
				return m, m.keyInput.Focus()
			}
			m.screen = screenIngestion
			m.cursor = 4 // Default to None
			return m, nil
		}
	}

	return m, nil
}

// integrationKeyEnvVar maps key names to their environment variable names.
var integrationKeyEnvVar = map[string]string{
	"anthropic":       "ANTHROPIC_API_KEY",
	"openai":          "OPENAI_API_KEY",
	"github":          "GITHUB_TOKEN",
	"slack_bot_token": "SLACK_BOT_TOKEN",
	"slack_app_token": "SLACK_APP_TOKEN",
}

// integrationKeyLabel maps key names to display labels.
var integrationKeyLabel = map[string]string{
	"anthropic":       "Anthropic",
	"openai":          "OpenAI",
	"github":          "GitHub",
	"slack_bot_token": "Slack Bot Token",
	"slack_app_token": "Slack App Token",
}

func newKeyInput() textinput.Model {
	ti := textinput.New()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Placeholder = ""
	ti.Width = 60
	return ti
}

func (m model) updateIntegrationKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			key := strings.TrimSpace(m.keyInput.Value())
			if key != "" {
				name := m.pendingKeys[m.keyIndex]
				m.config.IntegrationKeys[name] = key
			}
			m.keyIndex++
			if m.keyIndex < len(m.pendingKeys) {
				m.keyInput = newKeyInput()
				m.screen = screenIntegrationKey
				return m, m.keyInput.Focus()
			}
			m.screen = screenIngestion
			m.cursor = 4 // Default to None
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}

func (m model) updateIngestion(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Scheduled (cron)", "schedule"},
		{"Webhook", "webhook"},
		{"Manual trigger", "manual"},
		{"On startup", "startup"},
		{"None", "none"},
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			m.config.Ingestion = m.options[m.cursor].value
			m.screen = screenConfirm
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.done = true
			return m, tea.Quit
		case "n", "N":
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting || m.done {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("  Creating agent: %s", m.name)))
	b.WriteString("\n\n")

	// Step indicator
	steps := []string{"Description", "Interfaces", "Model", "Knowledge", "Integrations", "Ingestion", "Confirm"}
	stepIndex := m.screenStep()
	for i, s := range steps {
		if i == stepIndex {
			b.WriteString(selectedStyle.Render("● " + s))
		} else if i < stepIndex {
			b.WriteString(dimStyle.Render("✓ " + s))
		} else {
			b.WriteString(dimStyle.Render("○ " + s))
		}
		if i < len(steps)-1 {
			b.WriteString(dimStyle.Render(" → "))
		}
	}
	b.WriteString("\n\n")

	switch m.screen {
	case screenDescription:
		b.WriteString(promptStyle.Render("  Description"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  A short summary of what your agent does."))
		b.WriteString("\n\n")
		b.WriteString("  " + m.descInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm"))

	case screenInterface:
		b.WriteString(promptStyle.Render("  Interfaces"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  How users interact with your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderMultiSelectOptions([]option{
			{"Web (HTTP/SSE)", "web"},
			{"Slack", "slack"},
		}))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenModel:
		b.WriteString(promptStyle.Render("  Self-hosted model"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Run a model locally alongside your agent."))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  For cloud APIs (Anthropic, OpenAI), skip this and add them as integrations."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptions([]option{
			{"Ollama", "ollama"},
			{"Hugging Face", "huggingface"},
			{"None", "none"},
		}))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter select"))

	case screenModelName:
		b.WriteString(promptStyle.Render(fmt.Sprintf("  Model name (%s)", m.config.ModelProvider)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  e.g. llama3, mistral, codellama"))
		b.WriteString("\n\n")
		b.WriteString("  " + m.modelInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm · leave empty to skip"))

	case screenKnowledge:
		b.WriteString(promptStyle.Render("  Knowledge stores"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Persistent data stores for memory and context."))
		b.WriteString("\n\n")
		b.WriteString(m.renderMultiSelectOptions([]option{
			{"Qdrant (vector store)", "qdrant"},
			{"Redis (key-value store)", "redis"},
			{"Neo4j (graph database)", "neo4j"},
		}))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenIntegrations:
		b.WriteString(promptStyle.Render("  Integrations"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  External services — model APIs for LLM access, tools for extended capabilities."))
		b.WriteString("\n\n")
		b.WriteString(m.renderMultiSelectOptions([]option{
			{"Anthropic", "anthropic"},
			{"OpenAI", "openai"},
			{"GitHub", "github"},
		}))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenIntegrationKey:
		name := m.pendingKeys[m.keyIndex]
		envVar := integrationKeyEnvVar[name]
		label := integrationKeyLabel[name]
		b.WriteString(promptStyle.Render(fmt.Sprintf("  %s API key", label)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(fmt.Sprintf("  Will be saved as %s in .env — you can also set it later.", envVar)))
		b.WriteString("\n\n")
		b.WriteString("  " + m.keyInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm · leave empty to skip"))

	case screenIngestion:
		b.WriteString(promptStyle.Render("  Data ingestion"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  How to trigger your data pipeline that populates knowledge stores."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptions([]option{
			{"Scheduled (cron)", "schedule"},
			{"Webhook", "webhook"},
			{"Manual trigger", "manual"},
			{"On startup", "startup"},
			{"None", "none"},
		}))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter select"))

	case screenConfirm:
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("  Create this agent? "))
		b.WriteString(dimStyle.Render("(Y/n)"))
	}

	b.WriteString("\n")
	return b.String()
}

// screenStep maps the current screen to the step index for the progress indicator.
func (m model) screenStep() int {
	switch m.screen {
	case screenDescription:
		return 0
	case screenInterface:
		return 1
	case screenModel, screenModelName:
		return 2
	case screenKnowledge:
		return 3
	case screenIntegrations, screenIntegrationKey:
		return 4
	case screenIngestion:
		return 5
	case screenConfirm:
		return 6
	}
	return 0
}

func (m model) renderOptions(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("  ❯ " + opt.label))
		} else {
			b.WriteString("    " + opt.label)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) renderMultiSelectOptions(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("❯ ")
		}
		checkbox := "○"
		if m.selected[i] {
			checkbox = selectedStyle.Render("●")
		}
		b.WriteString("  " + cursor + checkbox + " " + opt.label + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) renderSummary() string {
	var b strings.Builder
	b.WriteString(promptStyle.Render("  Summary"))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %-14s", label)))
		b.WriteString(value + "\n")
	}

	row("Name", m.config.Name)
	row("Description", m.config.Description)

	if len(m.config.Interfaces) > 0 {
		row("Interfaces", strings.Join(m.config.Interfaces, ", "))
	} else {
		row("Interfaces", "web")
	}

	if m.config.ModelProvider != "" {
		modelDisplay := m.config.ModelProvider
		if m.config.Model != "" {
			modelDisplay += "/" + m.config.Model
		}
		row("Model", modelDisplay)
	} else {
		row("Model", dimStyle.Render("none"))
	}

	if len(m.config.Knowledge) > 0 {
		row("Knowledge", strings.Join(m.config.Knowledge, ", "))
	} else {
		row("Knowledge", dimStyle.Render("none"))
	}

	if len(m.config.Integrations) > 0 {
		row("Integrations", strings.Join(m.config.Integrations, ", "))
	} else {
		row("Integrations", dimStyle.Render("none"))
	}

	row("Ingestion", m.config.Ingestion)

	return b.String()
}

// Run launches the TUI and returns the user's configuration.
func Run(name string) (scaffold.ScaffoldConfig, error) {
	m := initialModel(name)
	p := tea.NewProgram(m)

	result, err := p.Run()
	if err != nil {
		return scaffold.ScaffoldConfig{}, err
	}

	finalModel := result.(model)
	if finalModel.quitting {
		return scaffold.ScaffoldConfig{}, fmt.Errorf("cancelled")
	}

	return finalModel.config, nil
}
