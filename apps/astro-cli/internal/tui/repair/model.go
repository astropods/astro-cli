package repair

import (
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	selectedStyle = lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
)

// Item is a selectable file entry.
type Item struct {
	Label    string
	Selected bool
}

type model struct {
	items    []Item
	cursor   int
	done     bool
	quitting bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("  Select files to check"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("❯ ")
		}
		checkbox := dimStyle.Render("○")
		if item.Selected {
			checkbox = selectedStyle.Render("●")
		}
		_, _ = fmt.Fprintf(&b, "    %s%s %s\n", cursor, checkbox, item.Label)
	}

	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))
	b.WriteString("\n")
	return b.String()
}

// Run shows the multi-select file picker and returns the items with updated selections.
// Returns an error if the user cancels.
func Run(items []Item) ([]Item, error) {
	m := model{items: items}
	p := tea.NewProgram(m)

	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := result.(model)
	if final.quitting {
		return nil, fmt.Errorf("cancelled")
	}
	return final.items, nil
}
