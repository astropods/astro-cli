package create

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)
