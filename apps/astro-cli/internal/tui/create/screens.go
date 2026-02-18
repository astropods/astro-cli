package create

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

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
			m.options = interfaceOptions()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.descInput, cmd = m.descInput.Update(msg)
	return m, cmd
}

func (m model) updateInterface(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = interfaceOptions()

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
			m.screen = screenInfrastructure
			m.cursor = 1 // First selectable item (skip header at 0)
			m.selected = make(map[int]bool)
			m.options = infrastructureOptions()
			return m, nil
		}
	}

	return m, nil
}

func (m model) updateInfrastructure(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = infrastructureOptions()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.cursor = prevSelectable(m.options, m.cursor)
		case "down", "j":
			m.cursor = nextSelectable(m.options, m.cursor)
		case " ":
			if !m.options[m.cursor].isHeader {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "enter":
			// Parse selections into config fields
			m.config.ModelProvider = ""
			m.config.Model = ""
			m.config.Knowledge = nil
			m.config.Integrations = nil

			needsOllama := false
			for i, opt := range m.options {
				if !m.selected[i] || opt.isHeader {
					continue
				}
				switch opt.value {
				case "ollama":
					m.config.ModelProvider = "ollama"
					needsOllama = true
				case "anthropic", "openai", "github":
					m.config.Integrations = append(m.config.Integrations, opt.value)
				case "qdrant", "redis", "neo4j":
					m.config.Knowledge = append(m.config.Knowledge, opt.value)
				}
			}

			if needsOllama {
				ti := textinput.New()
				ti.Placeholder = "e.g. llama3, mistral"
				ti.Width = 60
				m.modelInput = ti
				m.screen = screenModelName
				return m, m.modelInput.Focus()
			}

			return transitionToKeys(m)
		}
	}

	return m, nil
}

func (m model) updateModelName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			m.config.Model = strings.TrimSpace(m.modelInput.Value())
			return transitionToKeys(m)
		}
	}

	var cmd tea.Cmd
	m.modelInput, cmd = m.modelInput.Update(msg)
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
			m.cursor = 4 // Default to None
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}

func (m model) updateIngestion(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.options = ingestionOptions()

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
	m.cursor = 4 // Default to None
	return m, nil
}
