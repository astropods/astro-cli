package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── root model ───────────────────────────────────────────────────────────────

type overlayKind int

const (
	overlayNone    overlayKind = iota
	overlayConfirm             // y/n prompt
	overlayInput               // single text input
	overlayError               // dismissable error
)

type appModel struct {
	width, height int
	tabs          []Tab
	active        int

	overlay     overlayKind
	confirmText string
	confirmFn   func() tea.Cmd
	inputTitle  string
	inputField  textinput.Model
	inputFn     func(string) tea.Cmd
	errText     string
}

func newAppModel(tabs []Tab) appModel {
	ti := textinput.New()
	ti.CharLimit = 200
	return appModel{
		tabs:       tabs,
		active:     0,
		inputField: ti,
	}
}

// Run starts the bubbletea program.
func Run(client adminv1.AdminServiceClient) error {
	tabs := []Tab{
		newDeploymentsModel(client),
		newQueryModel(client),
	}
	p := tea.NewProgram(newAppModel(tabs), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func (m appModel) Init() tea.Cmd {
	return tea.Batch(tea.ClearScreen, m.tabs[0].Init())
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.overlay != overlayNone {
		return m.updateOverlay(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inner := m.innerHeight()
		for _, tab := range m.tabs {
			tab.SetSize(m.width, inner)
		}
		return m, nil

	case showConfirmMsg:
		m.overlay = overlayConfirm
		m.confirmText = msg.text
		m.confirmFn = msg.fn
		return m, nil

	case showInputMsg:
		m.overlay = overlayInput
		m.inputTitle = msg.title
		m.inputField.Reset()
		m.inputField.Placeholder = msg.placeholder
		m.inputField.Focus()
		m.inputFn = msg.fn
		return m, textinput.Blink

	case showErrMsg:
		m.overlay = overlayError
		m.errText = msg.text
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if msg.Y == 0 {
				if idx := m.headerTabAtX(msg.X); idx >= 0 && idx != m.active {
					return m.switchTab(idx)
				}
				return m, nil
			}
		}
		// Forward mouse events to active tab.
		var cmd tea.Cmd
		m.tabs[m.active], cmd = m.tabs[m.active].Update(msg)
		return m, cmd

	case tea.KeyMsg:
		key := msg.String()

		if key == "ctrl+c" {
			return m, tea.Quit
		}

		// Check if active tab consumes this key before handling globals.
		if !m.tabs[m.active].ConsumesKey(key) {
			switch key {
			case "q", "Q":
				return m, tea.Quit
			case "tab":
				return m.switchTab((m.active + 1) % len(m.tabs))
			}
		}

		// Number keys switch tabs (1-9).
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0] - '1')
			if idx < len(m.tabs) && idx != m.active {
				return m.switchTab(idx)
			}
		}
	}

	var cmd tea.Cmd
	m.tabs[m.active], cmd = m.tabs[m.active].Update(msg)
	return m, cmd
}

func (m appModel) switchTab(tab int) (appModel, tea.Cmd) {
	m.active = tab
	return m, m.tabs[tab].Init()
}

func (m appModel) updateOverlay(msg tea.Msg) (appModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if m.overlay == overlayInput {
			var cmd tea.Cmd
			m.inputField, cmd = m.inputField.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.overlay {
	case overlayError:
		m.overlay = overlayNone
		return m, nil

	case overlayConfirm:
		switch key.String() {
		case "y", "Y", "enter":
			m.overlay = overlayNone
			return m, m.confirmFn()
		case "n", "N", "esc", "q":
			m.overlay = overlayNone
		}
		return m, nil

	case overlayInput:
		switch key.String() {
		case "enter":
			val := m.inputField.Value()
			m.overlay = overlayNone
			m.inputField.Reset()
			return m, m.inputFn(val)
		case "esc":
			m.overlay = overlayNone
			m.inputField.Reset()
			return m, nil
		default:
			var cmd tea.Cmd
			m.inputField, cmd = m.inputField.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m appModel) View() string {
	if m.width == 0 {
		return statusWIP.Render("Loading…")
	}

	if m.overlay != overlayNone {
		return m.renderOverlay()
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	inner := m.innerHeight()

	content := m.tabs[m.active].View(m.width, inner)

	// Pad content to fill inner height so footer stays at the bottom.
	contentLines := strings.Count(content, "\n") + 1
	if contentLines < inner {
		content += strings.Repeat("\n", inner-contentLines)
	}

	return strings.Join([]string{header, content, footer}, "\n")
}

func (m appModel) innerHeight() int {
	h := m.height - 3 // header (1) + header border (1) + footer (1)
	if h < 0 {
		return 0
	}
	return h
}

func (m appModel) renderHeader() string {
	tabs := make([]string, 0, len(m.tabs)+1)
	tabs = append(tabs, brandStyle.Render(" astro-queen")+"  ")
	for i, tab := range m.tabs {
		label := fmt.Sprintf("[%d] %s", i+1, tab.Name())
		if i == m.active {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}

	left := strings.Join(tabs, "")
	return headerBarStyle.Width(m.width).Render(left)
}

// headerTabAtX returns the tab index at column x in the header, or -1.
func (m appModel) headerTabAtX(x int) int {
	// Calculate the width of the brand section.
	brandWidth := lipgloss.Width(brandStyle.Render(" astro-queen") + "  ")
	pos := brandWidth
	for i, tab := range m.tabs {
		label := fmt.Sprintf("[%d] %s", i+1, tab.Name())
		var w int
		if i == m.active {
			w = lipgloss.Width(activeTabStyle.Render(label))
		} else {
			w = lipgloss.Width(inactiveTabStyle.Render(label))
		}
		if x >= pos && x < pos+w {
			return i
		}
		pos += w
	}
	return -1
}

func (m appModel) renderFooter() string {
	tab := m.tabs[m.active]

	// Left: tab status.
	left := " " + tab.Status()

	// Right: key hints.
	hints := tab.Hints()
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = hint(h.Key, h.Desc)
	}
	right := strings.Join(parts, "  ") + " "

	// Fill the gap between left and right.
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.width - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	line := left + strings.Repeat(" ", gap) + right
	return statusBarStyle.Width(m.width).Render(line)
}

func (m appModel) renderOverlay() string {
	var content, boxStr string

	switch m.overlay {
	case overlayConfirm:
		yesNo := keyStyle.Render("y / Enter") + descStyle.Render("  yes") +
			"   " + keyStyle.Render("n / Esc") + descStyle.Render("  no")
		content = m.confirmText + "\n\n" + yesNo
		boxStr = modalStyle.Width(min(m.width-6, 60)).Render(content)

	case overlayInput:
		content = m.inputTitle + "\n\n" +
			m.inputField.View() + "\n\n" +
			descStyle.Render("Enter to confirm  •  Esc to cancel")
		boxStr = modalStyle.Width(min(m.width-6, 60)).Render(content)

	case overlayError:
		content = statusErr.Render("Error") + "\n\n" +
			m.errText + "\n\n" +
			descStyle.Render("Press any key to dismiss")
		boxStr = errModalStyle.Width(min(m.width-6, 60)).Render(content)
	}

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStr,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")),
	)
}
