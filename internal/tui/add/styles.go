package add

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/postman/astro/apps/astro-cli/internal/theme"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Primary)

	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(theme.Primary).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)
