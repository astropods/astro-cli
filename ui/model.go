package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionState int

const (
	mainMenuView sessionState = iota
	settingsView
)

type Model struct {
	state        sessionState
	menuCursor   int
	menuItems    []string
	width        int
	height       int
	selectedItem string
}

func NewModel() Model {
	return Model{
		state: mainMenuView,
		menuItems: []string{
			"Settings",
			"Quit",
		},
		menuCursor: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case mainMenuView:
			return m.updateMainMenu(msg)
		case settingsView:
			return m.updateSettings(msg)
		}
	}

	return m, nil
}

func (m Model) updateMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}

	case "down", "j":
		if m.menuCursor < len(m.menuItems)-1 {
			m.menuCursor++
		}

	case "enter", " ":
		m.selectedItem = m.menuItems[m.menuCursor]
		switch m.selectedItem {
		case "Settings":
			m.state = settingsView
		case "Quit":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.state = mainMenuView
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	switch m.state {
	case mainMenuView:
		return m.viewMainMenu()
	case settingsView:
		return m.viewSettings()
	default:
		return "Unknown state"
	}
}

func (m Model) viewMainMenu() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(1, 0)

	menuStyle := lipgloss.NewStyle().
		Padding(1, 2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true)

	s := titleStyle.Render("✨ Astro CLI") + "\n\n"

	for i, item := range m.menuItems {
		cursor := " "
		if m.menuCursor == i {
			cursor = ">"
			item = selectedStyle.Render(item)
		}
		s += fmt.Sprintf("%s %s\n", cursor, item)
	}

	s += "\n\n"
	s += lipgloss.NewStyle().
		Faint(true).
		Render("↑/↓ or j/k: navigate • enter: select • q: quit")

	return menuStyle.Render(s)
}

func (m Model) viewSettings() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(1, 0)

	contentStyle := lipgloss.NewStyle().
		Padding(1, 2)

	s := titleStyle.Render("⚙️  Settings") + "\n\n"
	s += "Settings configuration will be implemented here.\n"
	s += "Configure CLI preferences and defaults.\n\n"
	s += lipgloss.NewStyle().
		Faint(true).
		Render("esc: back to menu • q: quit")

	return contentStyle.Render(s)
}
