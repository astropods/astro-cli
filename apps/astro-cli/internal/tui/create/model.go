package create

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
)

type screen int

const (
	screenDescription    screen = iota
	screenInterface
	screenInfrastructure
	screenModelName
	screenIntegrationKey
	screenIngestion
	screenConfirm
)

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
	case screenInfrastructure:
		return m.updateInfrastructure(msg)
	case screenModelName:
		return m.updateModelName(msg)
	case screenIntegrationKey:
		return m.updateIntegrationKey(msg)
	case screenIngestion:
		return m.updateIngestion(msg)
	case screenConfirm:
		return m.updateConfirm(msg)
	}

	return m, nil
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
