package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── Tab interface ───────────────────────────────────────────────────────────

// Tab is the interface every tab must implement.
type Tab interface {
	Name() string                      // label for header
	Init() tea.Cmd                     // called on first switch
	Update(msg tea.Msg) (Tab, tea.Cmd) // Elm update
	View(width, height int) string     // Elm view
	SetSize(width, height int)         // resize
	Status() string                    // rendered status string for header bar
	Hints(navMode bool) []KeyHint      // footer key hints (navMode=true when in navigate mode)
}

// KeyHint describes a key binding shown in the footer.
type KeyHint struct {
	Key  string
	Desc string
}

// ─── shared overlay messages ─────────────────────────────────────────────────

// showConfirmMsg requests a y/n confirmation overlay.
type showConfirmMsg struct {
	text string
	fn   func() tea.Cmd
}

// showInputMsg requests a single-field text-input overlay.
type showInputMsg struct {
	title       string
	placeholder string
	fn          func(string) tea.Cmd
}

// showErrMsg requests a dismissable error overlay.
type showErrMsg struct{ text string }

// loadingLogMsg appends a line to the loading overlay log.
type loadingLogMsg string

// loadingDoneMsg dismisses the loading overlay.
type loadingDoneMsg struct{}

// ─── styles ──────────────────────────────────────────────────────────────────

var (
	brandStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
	headerBarStyle   = lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	barBg            = lipgloss.NewStyle()

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))

	keyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	statusOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	statusErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusWIP  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	statusGood = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3)
	errModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(1, 3)
)

// hint formats a "key  desc" pair for the footer.
func hint(key, desc string) string {
	return keyStyle.Render(key) + descStyle.Render(" "+desc)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
