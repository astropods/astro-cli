package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/postman/astro/apps/astro-queen/internal/config"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── root model ───────────────────────────────────────────────────────────────

type overlayKind int

const (
	overlayNone    overlayKind = iota
	overlayConfirm             // y/n prompt
	overlayInput               // single text input
	overlayError               // dismissable error
	overlayLoading             // startup loading card
)

type appModel struct {
	width, height int
	tabs          []Tab
	active        int
	navMode       bool

	overlay     overlayKind
	confirmText string
	confirmFn   func() tea.Cmd
	inputTitle  string
	inputField  textinput.Model
	inputFn     func(string) tea.Cmd
	errText     string

	// Loading overlay state.
	spinner     spinner.Model
	loadingLogs []string
	loadingErr  string
	cfg         *config.Config
}

func newAppModel(tabs []Tab, cfg *config.Config) appModel {
	ti := textinput.New()
	ti.CharLimit = 200
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	return appModel{
		tabs:       tabs,
		active:     0,
		inputField: ti,
		spinner:    s,
		overlay:    overlayLoading,
		cfg:        cfg,
	}
}

// Run starts the bubbletea program.
func Run(client adminv1.AdminServiceClient, cfg *config.Config) error {
	tabs := []Tab{
		newDeploymentsModel(client),
		newQueryModel(client),
	}
	p := tea.NewProgram(newAppModel(tabs, cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tea.ClearScreen,
		m.spinner.Tick,
		m.emitLoadingLogs(),
	)
}

// emitLoadingLogs sends config/connection info as sequential log messages, then inits the first tab.
func (m appModel) emitLoadingLogs() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		return loadingLogBatchMsg{cfg: cfg}
	}
}

// loadingLogBatchMsg triggers the sequential log emission inside Update.
type loadingLogBatchMsg struct{ cfg *config.Config }

func buildLoadingSteps(cfg *config.Config) []string {
	cfgPath := config.DefaultPath()
	steps := []string{
		fmt.Sprintf("Config loaded from %s", cfgPath),
		fmt.Sprintf("Server: %s", cfg.Server),
	}
	if cfg.CertFile != "" {
		steps = append(steps, fmt.Sprintf("TLS: mTLS (cert: %s, ca: %s)", cfg.CertFile, cfg.CAFile))
	} else {
		steps = append(steps, "TLS: insecure")
	}
	steps = append(steps, "Connecting to server...")
	return steps
}

func emitStepsSequentially(steps []string, initTab tea.Cmd) tea.Cmd {
	if len(steps) == 0 {
		return initTab
	}
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return loadingStepMsg{steps: steps, initTab: initTab}
	})
}

type loadingStepMsg struct {
	steps   []string
	initTab tea.Cmd
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Loading overlay handles its own messages but also forwards some through.
	if m.overlay == overlayLoading {
		return m.updateLoading(msg)
	}

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

		if m.navMode {
			// Navigate mode: app owns keys for tab switching and quitting.
			switch key {
			case "q", "Q":
				return m, tea.Quit
			case "tab":
				return m.switchTab((m.active + 1) % len(m.tabs))
			case "esc":
				m.navMode = false
				return m, nil
			}
			// Number keys switch tabs (1-9).
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				idx := int(key[0] - '1')
				if idx < len(m.tabs) && idx != m.active {
					return m.switchTab(idx)
				}
			}
			return m, nil
		}

		// Tab-focused mode: Esc enters navigate mode. Everything else → tab.
		if key == "esc" {
			m.navMode = true
			return m, nil
		}
	}

	// Forward non-intercepted messages to the active tab.
	var cmd tea.Cmd
	m.tabs[m.active], cmd = m.tabs[m.active].Update(msg)
	return m, cmd
}

func (m appModel) updateLoading(msg tea.Msg) (appModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inner := m.innerHeight()
		for _, tab := range m.tabs {
			tab.SetSize(m.width, inner)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "Q":
			return m, tea.Quit
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case loadingLogBatchMsg:
		steps := buildLoadingSteps(msg.cfg)
		return m, emitStepsSequentially(steps, m.tabs[0].Init())

	case loadingStepMsg:
		m.loadingLogs = append(m.loadingLogs, msg.steps[0])
		remaining := msg.steps[1:]
		return m, emitStepsSequentially(remaining, msg.initTab)

	case loadingLogMsg:
		m.loadingLogs = append(m.loadingLogs, string(msg))
		return m, nil

	case loadingDoneMsg:
		m.overlay = overlayNone
		return m, nil

	case deploymentsLoadedMsg:
		if msg.err != nil {
			// Stay on loading overlay but show the error — no way to dismiss into main UI.
			m.loadingErr = fmt.Sprintf("Connection failed: %s", msg.err)
			return m, nil
		}
		// First data arrived — dismiss loading overlay and forward to tab.
		m.overlay = overlayNone
		var cmd tea.Cmd
		m.tabs[m.active], cmd = m.tabs[m.active].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m appModel) switchTab(tab int) (appModel, tea.Cmd) {
	m.navMode = false
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
	if m.overlay != overlayNone {
		if m.width == 0 {
			return statusWIP.Render("Loading…")
		}
		return m.renderOverlay()
	}

	if m.width == 0 {
		return statusWIP.Render("Loading…")
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

	// Left: mode indicator + tab status.
	var mode string
	if m.navMode {
		mode = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render(" NAV ")
	} else {
		mode = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82")).Render(" ● ")
	}
	left := mode + " " + tab.Status()

	// Right: key hints.
	hints := tab.Hints(m.navMode)
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

	case overlayLoading:
		failed := m.loadingErr != ""
		var title string
		if failed {
			title = statusErr.Render("Connection Failed")
		} else {
			title = brandStyle.Render("Connecting")
		}
		var lines []string
		checkmark := statusGood.Render("✓")
		crossmark := statusErr.Render("✗")
		for i, line := range m.loadingLogs {
			if i < len(m.loadingLogs)-1 || failed {
				lines = append(lines, checkmark+" "+line)
			} else {
				lines = append(lines, m.spinner.View()+" "+line)
			}
		}
		if len(lines) == 0 {
			lines = append(lines, m.spinner.View()+" Initializing...")
		}
		if failed {
			lines = append(lines, crossmark+" "+m.loadingErr)
			lines = append(lines, "")
			lines = append(lines, descStyle.Render("Press q to quit"))
		}
		content = title + "\n\n" + strings.Join(lines, "\n")
		style := modalStyle
		if failed {
			style = errModalStyle
		}
		boxStr = style.Width(min(m.width-6, 60)).Render(content)
	}

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStr,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")),
	)
}
