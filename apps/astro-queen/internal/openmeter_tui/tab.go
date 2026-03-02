package openmeter_tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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
	Tip() string
	Hints() []KeyHint
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
// and message handling to the callbacks so any tab can present a rich modal form.
type showFormMsg struct {
	view     func(width int) string
	update   func(tea.Msg) (done bool, cmd tea.Cmd)
	maxWidth int // 0 = default (130)
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

// ─── shared huh theme ────────────────────────────────────────────────────────

var huhTheme = func() *huh.Theme {
	t := huh.ThemeBase()
	t.Focused.Title = t.Focused.Title.Foreground(colAccent)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colAccent)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colOrange)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colOrange)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colGreen)
	t.Focused.Base = t.Focused.Base.BorderForeground(colAccent)
	t.Blurred.TextInput.Prompt = t.Blurred.TextInput.Prompt.Foreground(colDimmed)
	return t
}()

// ─── inline form helpers ─────────────────────────────────────────────────────

// formField renders a labeled input field as a fixed-width lipgloss block.
// The label is padded to a fixed 10-char width for alignment.
func formField(label string, content string, focused bool, width int) string {
	marker := " "
	if focused {
		marker = focusStyle.Render("▸")
	}
	lbl := labelStyle.Width(10).Render(label)
	inner := marker + lbl + content
	return lipgloss.NewStyle().Width(width).Render(inner)
}

// formLine joins multiple field blocks horizontally on one line.
func formLine(fields ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, fields...)
}

// formSeparator renders a full-width horizontal rule.
func formSeparator(width int) string {
	return lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", width))
}

// borderLabel replaces the top border of a rendered rounded-border box with
// "╭ label ───…───╮", colored to match the border. Works with any content
// previously rendered with lipgloss.RoundedBorder().
func borderLabel(rendered, label string, color lipgloss.Color) string {
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 0 {
		return rendered
	}
	totalW := lipgloss.Width(lines[0])
	if totalW < 4 {
		return rendered
	}

	s := lipgloss.NewStyle().Foreground(color)
	labelText := s.Bold(true).Render(" " + label + " ")
	labelW := len([]rune(" " + label + " ")) // visual width (no wide chars)
	fillW := totalW - 2 - labelW             // minus corners
	if fillW < 0 {
		fillW = 0
	}

	newTop := s.Render("╭") + labelText + s.Render(strings.Repeat("─", fillW)+"╮")
	if len(lines) > 1 {
		return newTop + "\n" + lines[1]
	}
	return newTop
}
