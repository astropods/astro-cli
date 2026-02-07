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
	screenApiKey
	screenKnowledge
	screenTools
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
	// Masked input for API key (used on screenApiKey)
	apiKeyInput textinput.Model

	// Select state
	options  []option
	cursor   int
	selected map[int]bool // for multi-select (tools)

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
			Name:       name,
			Interfaces: []string{},
			Tools:      []string{},
		},
		selected: make(map[int]bool),
		// apiKeyInput is initialized when entering screenApiKey
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
	case screenApiKey:
		return m.updateApiKey(msg)
	case screenKnowledge:
		return m.updateKnowledge(msg)
	case screenTools:
		return m.updateTools(msg)
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
			m.cursor = 1 // Default to OpenAI
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Anthropic (Claude)", "anthropic"},
		{"OpenAI (GPT)", "openai"},
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
			m.config.Model = m.options[m.cursor].value
			if m.config.Model == "none" {
				m.screen = screenKnowledge
				m.cursor = 0
				return m, nil
			}
			// Show API key step for OpenAI or Anthropic
			ti := textinput.New()
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '*'
			ti.Placeholder = ""
			ti.Width = 60
			m.apiKeyInput = ti
			m.screen = screenApiKey
			return m, m.apiKeyInput.Focus()
		}
	}

	return m, nil
}

func (m model) updateApiKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			key := strings.TrimSpace(m.apiKeyInput.Value())
			if key != "" && strings.ToLower(key) != "n" {
				m.config.ModelApiKey = key
			}
			m.screen = screenKnowledge
			m.cursor = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return m, cmd
}

func (m model) updateKnowledge(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Vector Store", "vector"},
		{"Key-Value Store", "kv"},
		{"Both", "both"},
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
			m.config.Knowledge = m.options[m.cursor].value
			m.screen = screenTools
			m.cursor = 0
			m.selected = make(map[int]bool)
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateTools(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
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
			tools := []string{}
			for i, opt := range m.options {
				if m.selected[i] {
					tools = append(tools, opt.value)
				}
			}
			m.config.Tools = tools
			m.screen = screenIngestion
			m.cursor = 3 // Default to None
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateIngestion(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = []option{
		{"Scheduled (cron)", "schedule"},
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

	b.WriteString(titleStyle.Render(fmt.Sprintf("Creating agent: %s", m.name)))
	b.WriteString("\n\n")

	switch m.screen {
	case screenDescription:
		b.WriteString(promptStyle.Render("Description:"))
		b.WriteString("\n")
		b.WriteString(m.descInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Enter to continue"))
	case screenInterface:
		b.WriteString(promptStyle.Render("Interface type (select one or more):"))
		b.WriteString("\n")
		b.WriteString(m.renderMultiSelectOptions([]option{
			{"Web (HTTP/SSE)", "web"},
			{"Slack", "slack"},
		}))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to navigate, Space to toggle, Enter to continue"))
	case screenModel:
		b.WriteString(promptStyle.Render("Model provider:"))
		b.WriteString("\n")
		b.WriteString(m.renderOptions([]option{
			{"Anthropic (Claude)", "anthropic"},
			{"OpenAI (GPT)", "openai"},
			{"None", "none"},
		}))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to navigate, Enter to select"))
	case screenApiKey:
		modelLabel := "OpenAI"
		if m.config.Model == "anthropic" {
			modelLabel = "Anthropic"
		}
		b.WriteString(promptStyle.Render(fmt.Sprintf("Add your %s API key now?", modelLabel)))
		b.WriteString("\n")
		b.WriteString(m.apiKeyInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Paste key (masked) or press Enter to skip"))
	case screenKnowledge:
		b.WriteString(promptStyle.Render("Knowledge store:"))
		b.WriteString("\n")
		b.WriteString(m.renderOptions([]option{
			{"Vector Store", "vector"},
			{"Key-Value Store", "kv"},
			{"Both", "both"},
			{"None", "none"},
		}))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to navigate, Enter to select"))
	case screenTools:
		b.WriteString(promptStyle.Render("Tools (optional):"))
		b.WriteString("\n")
		b.WriteString(m.renderMultiSelectOptions([]option{
			{"GitHub", "github"},
		}))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to navigate, Space to toggle, Enter to continue"))
	case screenIngestion:
		b.WriteString(promptStyle.Render("Data ingestion trigger:"))
		b.WriteString("\n")
		b.WriteString(m.renderOptions([]option{
			{"Scheduled (cron)", "schedule"},
			{"Manual trigger", "manual"},
			{"On startup", "startup"},
			{"None", "none"},
		}))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑/↓ to navigate, Enter to select"))
	case screenConfirm:
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("Create this agent? (Y/n)"))
	}

	return b.String()
}

func (m model) renderOptions(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("❯ " + opt.label))
		} else {
			b.WriteString("  " + opt.label)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) renderMultiSelectOptions(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		prefix := "  "
		if i == m.cursor {
			prefix = selectedStyle.Render("❯ ")
		}
		checkbox := "[ ]"
		if m.selected[i] {
			checkbox = selectedStyle.Render("[✓]")
		}
		b.WriteString(prefix + checkbox + " " + opt.label + "\n")
	}
	return b.String()
}

func (m model) renderSummary() string {
	var b strings.Builder
	b.WriteString(promptStyle.Render("Summary:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Name:        %s\n", m.config.Name))
	b.WriteString(fmt.Sprintf("  Description: %s\n", m.config.Description))
	if len(m.config.Interfaces) > 0 {
		b.WriteString(fmt.Sprintf("  Interfaces:  %s\n", strings.Join(m.config.Interfaces, ", ")))
	} else {
		b.WriteString("  Interfaces:  web\n")
	}
	b.WriteString(fmt.Sprintf("  Model:       %s\n", m.config.Model))
	if m.config.Model == "openai" || m.config.Model == "anthropic" {
		if m.config.ModelApiKey != "" {
			b.WriteString("  API key:     (set)\n")
		} else {
			b.WriteString("  API key:     (not set)\n")
		}
	}
	b.WriteString(fmt.Sprintf("  Knowledge:   %s\n", m.config.Knowledge))
	if len(m.config.Tools) > 0 {
		b.WriteString(fmt.Sprintf("  Tools:       %s\n", strings.Join(m.config.Tools, ", ")))
	} else {
		b.WriteString("  Tools:       none\n")
	}
	b.WriteString(fmt.Sprintf("  Ingestion:   %s\n", m.config.Ingestion))
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
