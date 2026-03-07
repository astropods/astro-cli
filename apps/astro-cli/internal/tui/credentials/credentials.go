package credentials

import (
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	promptStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle = lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
)

// Credential describes a single credential to collect.
type Credential struct {
	Name   string
	Secret bool
}

// Result maps each credential name to the value entered by the user.
// Credentials left blank are omitted.
type Result map[string]string

type model struct {
	provider string
	creds    []Credential
	index    int
	input    textinput.Model
	values   map[string]string
	done     bool
	quitting bool
}

func newModel(provider string, creds []Credential) model {
	if len(creds) == 0 {
		panic("newModel called with empty creds")
	}
	ti := textinput.New()
	ti.Width = 48
	ti.CharLimit = 512
	applyInputStyle(&ti, creds[0])
	ti.Focus()

	return model{
		provider: provider,
		creds:    creds,
		index:    0,
		input:    ti,
		values:   make(map[string]string),
	}
}

func applyInputStyle(ti *textinput.Model, c Credential) {
	if c.Secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		ti.Placeholder = "••••••••••••••••"
	} else {
		ti.EchoMode = textinput.EchoNormal
		ti.Placeholder = "value"
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
		case "enter":
			val := strings.TrimSpace(m.input.Value())
			if val != "" {
				m.values[m.creds[m.index].Name] = val
			}
			m.index++
			if m.index >= len(m.creds) {
				m.done = true
				return m, tea.Quit
			}
			m.input.Reset()
			applyInputStyle(&m.input, m.creds[m.index])
			return m, m.input.Focus()
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.done || m.quitting || m.index >= len(m.creds) {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("  %s credentials", m.provider)))
	b.WriteString("\n\n")

	// Step dots
	for i, c := range m.creds {
		if i == m.index {
			b.WriteString(selectedStyle.Render("● " + c.Name))
		} else if i < m.index {
			b.WriteString(dimStyle.Render("✓ " + c.Name))
		} else {
			b.WriteString(dimStyle.Render("○ " + c.Name))
		}
		if i < len(m.creds)-1 {
			b.WriteString(dimStyle.Render(" → "))
		}
	}
	b.WriteString("\n\n")

	current := m.creds[m.index]
	b.WriteString(promptStyle.Render("  " + current.Name))
	b.WriteString("\n")
	if current.Secret {
		b.WriteString(hintStyle.Render("  Stored securely in .env and never logged."))
	} else {
		b.WriteString(hintStyle.Render("  Will be written to .env."))
	}
	b.WriteString("\n\n")
	b.WriteString("  " + m.input.View())
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("  enter confirm · leave blank to skip"))
	b.WriteString("\n")

	return b.String()
}

// Run launches the credentials TUI and returns the collected values.
func Run(provider string, creds []Credential) (Result, error) {
	if len(creds) == 0 {
		return Result{}, nil
	}

	m := newModel(provider, creds)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := result.(model)
	if final.quitting {
		return Result{}, nil // cancelled — not an error, just skip
	}
	return final.values, nil
}
