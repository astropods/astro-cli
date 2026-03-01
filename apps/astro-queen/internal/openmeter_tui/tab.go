package openmeter_tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── Tab interface ───────────────────────────────────────────────────────────

// Tab is the interface every tab must implement.
type Tab interface {
	Name() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Tab, tea.Cmd)
	View(width, height int) string
	SetSize(width, height int)
	Status() string
	Hints(navMode bool) []KeyHint
}

// KeyHint describes a key binding shown in the footer.
type KeyHint struct {
	Key  string
	Desc string
}

// ─── shared overlay messages ─────────────────────────────────────────────────

type showConfirmMsg struct {
	text string
	fn   func() tea.Cmd
}

type showInputMsg struct {
	title       string
	placeholder string
	fn          func(string) tea.Cmd
}

type showErrMsg struct{ text string }

// showFormMsg requests a multi-field form overlay. The app delegates rendering
// and key handling to the callbacks so any tab can present a rich modal form.
type showFormMsg struct {
	view   func(width int) string
	update func(tea.KeyMsg) (done bool, cmd tea.Cmd)
}

type loadingLogMsg string

type loadingDoneMsg struct{}

// ─── styles (colors from theme.go) ───────────────────────────────────────────

var (
	brandStyle       = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Background(colAccent).Foreground(colBg).Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	headerBarStyle   = lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(colBorder)

	statusBarStyle = lipgloss.NewStyle().
			Background(colBgLight).
			Foreground(colFg)

	keyStyle  = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	descStyle = lipgloss.NewStyle().Foreground(colMuted)

	statusOK  = lipgloss.NewStyle().Foreground(colDimmed)
	statusErr = lipgloss.NewStyle().Foreground(colRed)
	statusWIP = lipgloss.NewStyle().Foreground(colAccent)

	statusGood = lipgloss.NewStyle().Foreground(colGreen)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(1, 3)
	errModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colRed).
			Padding(1, 3)

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	labelStyle  = lipgloss.NewStyle().Foreground(colDimmed)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlue)
	dimStyle    = lipgloss.NewStyle().Foreground(colMuted)
	focusStyle  = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	tipStyle    = lipgloss.NewStyle().Foreground(colMuted).Italic(true)
)

func hint(key, desc string) string {
	return keyStyle.Render(key) + descStyle.Render(" "+desc)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
