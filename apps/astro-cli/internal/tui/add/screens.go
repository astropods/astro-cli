package add

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.err = "Name cannot be empty."
				return m, nil
			}
			m.err = ""
			m.name = name
			m.screen = m.nextScreenAfterName()
			m.cursor = 0
			return m, m.focusForScreen()
		}
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m model) nextScreenAfterName() screen {
	switch m.domain {
	case "model":
		if m.provider == "ollama" {
			return screenOllamaModel
		}
		return screenConfirm
	case "knowledge":
		return screenPersistent
	case "integration":
		return screenConfirm
	case "ingestion":
		return screenImage
	}
	return screenConfirm
}

// updateRadio handles navigation and selection for all radio-style screens.
func (m model) updateRadio(msg tea.Msg, opts []option) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(opts)-1 {
				m.cursor++
			}
		case " ", "enter":
			val := opts[m.cursor].value
			switch m.screen {
			case screenOllamaModel:
				m.ollamaModel = val
				m.screen = screenConfirm
				m.cursor = 0
			case screenPersistent:
				m.persistent = val == "true"
				m.screen = screenConfirm
				m.cursor = 0
			case screenTrigger:
				m.triggerType = val
				m.screen = screenConfirm
				m.cursor = 0
			case screenVarSecret:
				m.vars = append(m.vars, providerVar{name: m.currentVarName, secret: val == "true"})
				m.screen = screenAddAnother
				m.cursor = 0
			case screenAddAnother:
				if val == "true" {
					m.varNameInput.Reset()
					m.screen = screenVarName
					m.cursor = 0
					return m, m.varNameInput.Focus()
				}
				m.screen = screenConfirm
				m.cursor = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateScopeMulti(msg tea.Msg) (tea.Model, tea.Cmd) {
	opts := scopeOptions()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(opts)-1 {
				m.cursor++
			}
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			for i := range opts {
				if m.selected[i] {
					m.err = ""
					m.screen = screenVarName
					m.cursor = 0
					return m, m.varNameInput.Focus()
				}
			}
			m.err = "Select at least one scope."
		}
	}
	return m, nil
}

func (m model) updateVarName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.varNameInput.Value())
			if name == "" {
				m.err = "Variable name cannot be empty."
				return m, nil
			}
			m.err = ""
			m.currentVarName = name
			m.screen = screenVarSecret
			m.cursor = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.varNameInput, cmd = m.varNameInput.Update(msg)
	return m, cmd
}

func (m model) updateImage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			img := strings.TrimSpace(m.imageInput.Value())
			if img == "" {
				m.err = "Image cannot be empty."
				return m, nil
			}
			m.err = ""
			m.screen = screenTrigger
			m.cursor = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.imageInput, cmd = m.imageInput.Update(msg)
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

func (m model) focusForScreen() tea.Cmd {
	if m.screen == screenImage {
		return m.imageInput.Focus()
	}
	if m.screen == screenVarName {
		return m.varNameInput.Focus()
	}
	return nil
}

// buildEntry produces the YAML map for the new entry based on domain and collected inputs.
func (m model) buildEntry() map[string]any {
	switch m.domain {
	case "model":
		if m.provider == "ollama" {
			ollamaModel := m.ollamaModel
			if ollamaModel == "" {
				ollamaModel = ollamaModelOptions()[0].value
			}
			return map[string]any{"provider": "ollama", "models": []string{ollamaModel}}
		}
		return map[string]any{"provider": m.provider}
	case "knowledge":
		if m.persistent {
			return map[string]any{"provider": m.provider, "persistent": true}
		}
		return map[string]any{"provider": m.provider}
	case "integration":
		return map[string]any{"provider": m.provider}
	case "ingestion":
		trigger := m.triggerType
		if trigger == "" {
			trigger = "manual"
		}
		return map[string]any{
			"container": map[string]any{"image": m.imageInput.Value()},
			"trigger":   map[string]any{"type": trigger},
		}
	case "provider":
		variables := make([]map[string]any, 0, len(m.vars))
		for _, v := range m.vars {
			entry := map[string]any{"name": v.name, "datatype": "string"}
			if v.secret {
				entry["secret"] = true
			}
			variables = append(variables, entry)
		}
		return map[string]any{
			"scope":     m.scopeToSlice(),
			"variables": variables,
		}
	}
	return map[string]any{}
}

func (m model) scopeToSlice() []string {
	opts := scopeOptions()
	var result []string
	for i, opt := range opts {
		if m.selected[i] {
			result = append(result, opt.value)
		}
	}
	return result
}
