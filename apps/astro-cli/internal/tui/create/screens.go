package create

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleOptionScreen handles up/down/space/enter for any option list. Returns the updated model,
// the selected option values (opt.value for each selected index), and true if enter was pressed.
// Caller applies the selection and transitions to the next screen.
func (m model) handleOptionScreen(msg tea.Msg, opts []option) (model, []string, bool) {
	numOptions := len(opts)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < numOptions-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			selected := make([]string, 0, numOptions)
			for i, opt := range opts {
				if m.selected[i] {
					selected = append(selected, opt.value)
				}
			}
			return m, selected, true
		}
	}
	return m, nil, false
}

// optionsForScreen returns the option list for the given screen. Used by the single updateOptionScreen path.
func optionsForScreen(s screen) []option {
	switch s {
	case screenInterface:
		return interfaceOptions()
	case screenModel:
		return modelOptions()
	case screenOllamaModel:
		return ollamaModelOptions()
	case screenKnowledge:
		return knowledgeOptions()
	case screenIntegrations:
		return toolsOptions()
	case screenIngestion:
		return ingestionOptions()
	default:
		return nil
	}
}

// applySelectionAndTransition applies the selected values to config and sets the next screen.
// Returns the updated model and any tea.Cmd (e.g. Focus).
func applySelectionAndTransition(m model, selected []string) (model, tea.Cmd) {
	switch m.screen {
	case screenInterface:
		if len(selected) == 0 {
			m.err = "Select at least one interface."
			return m, nil
		}
		m.err = ""
		m.config.Interfaces = selected
		m.screen = screenModel
		m.cursor = 0
		m.selected = make(map[int]bool)
		m.selected[0] = true
		m.options = modelOptions()
		return m, nil
	case screenModel:
		m.config.ModelProvider = ""
		m.config.Model = ""
		m.config.Integrations = nil
		needOllamaModel := false
		for _, v := range selected {
			switch v {
			case "ollama":
				m.config.ModelProvider = "ollama"
				needOllamaModel = true
			case "anthropic", "openai":
				m.config.Integrations = append(m.config.Integrations, v)
			}
		}
		if needOllamaModel {
			m.screen = screenOllamaModel
			m.cursor = 0
			m.selected = make(map[int]bool)
			m.selected[0] = true
			m.options = ollamaModelOptions()
			return m, nil
		}
		m.screen = screenKnowledge
		m.cursor = 0
		m.selected = make(map[int]bool)
		m.options = knowledgeOptions()
		return m, nil
	case screenOllamaModel:
		if len(selected) > 0 {
			m.config.Model = selected[0]
		} else {
			opts := ollamaModelOptions()
			if m.cursor < len(opts) {
				m.config.Model = opts[m.cursor].value
			}
		}
		m.screen = screenKnowledge
		m.cursor = 0
		m.selected = make(map[int]bool)
		m.options = knowledgeOptions()
		return m, nil
	case screenKnowledge:
		m.config.Knowledge = selected
		m.screen = screenIntegrations
		m.cursor = 0
		m.selected = make(map[int]bool)
		m.options = toolsOptions()
		return m, nil
	case screenIntegrations:
		for _, v := range selected {
			m.config.Integrations = append(m.config.Integrations, v)
		}
		return transitionToKeys(m)
	case screenIngestion:
		m.config.Ingestions = selected
		m.screen = screenConfirm
		return m, nil
	default:
		return m, nil
	}
}

// updateOptionScreen is the single update path for all option-list screens (Interface, Model, OllamaModel, Knowledge, Integrations, Ingestion).
func (m model) updateOptionScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	opts := optionsForScreen(m.screen)
	m.options = opts
	m2, selected, entered := m.handleOptionScreen(msg, opts)
	if !entered {
		return m2, nil
	}
	m3, cmd := applySelectionAndTransition(m2, selected)
	return m3, cmd
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
			m.selected[0] = true
			m.options = interfaceOptions()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.descInput, cmd = m.descInput.Update(msg)
	return m, cmd
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
			m.cursor = 0
			m.selected = make(map[int]bool)
			m.options = ingestionOptions()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
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

// transitionToKeys sets up key collection and transitions to the appropriate screen.
func transitionToKeys(m model) (model, tea.Cmd) {
	m.pendingKeys = nil
	// Slack interface tokens
	for _, iface := range m.config.Interfaces {
		if iface == "slack" {
			m.pendingKeys = append(m.pendingKeys, "slack_bot_token", "slack_app_token")
		}
	}
	// Integration API keys
	for _, name := range m.config.Integrations {
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
	m.cursor = 0
	m.selected = make(map[int]bool)
	m.options = ingestionOptions()
	return m, nil
}
