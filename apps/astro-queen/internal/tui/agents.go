package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── focus ─────────────────────────────────────────────────────────────────────

type agentFocus int

const (
	agentFocusList   agentFocus = iota
	agentFocusDetail            // two-pane: table left, builds right
)

// ─── messages ─────────────────────────────────────────────────────────────────

type agentsLoadedMsg struct {
	rows [][]string
	err  error
	at   time.Time
}

type agentBuildsMsg struct {
	builds []*adminv1.AgentBuild
	err    error
}

// ─── model ────────────────────────────────────────────────────────────────────

type agentsModel struct {
	client adminv1.AdminServiceClient
	t      tableModel
	status string
	width  int
	height int

	focus         agentFocus
	builds        []*adminv1.AgentBuild
	rightLines    []string
	rightScroll   int
	leftWidth     int
	selectedAgent string // "account/name"
}

func newAgentsModel(client adminv1.AdminServiceClient) *agentsModel {
	t := newTableModel([]string{"Account", "Name", "Builds", "Published", "Latest Build", "Updated At"})
	t.SetFocused(true)
	return &agentsModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *agentsModel) Name() string { return "Agents" }

func (m *agentsModel) Init() tea.Cmd { return m.load() }

func (m *agentsModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case agentsLoadedMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
		} else {
			m.t.SetRows(msg.rows)
			m.status = statusOK.Render(fmt.Sprintf(
				"%d agents  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
			))
		}
		return m, nil

	case agentBuildsMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
			return m, nil
		}
		m.builds = msg.builds
		m.focus = agentFocusDetail
		m.rightLines = m.buildRightLines()
		m.rightScroll = 0
		m.computeLeftWidth()
		m.t.SetSize(m.leftWidth, m.height)
		m.status = statusOK.Render(fmt.Sprintf("%s — %d builds", m.selectedAgent, len(msg.builds)))
		return m, nil

	case tea.KeyMsg:
		switch m.focus {
		case agentFocusDetail:
			return m.updateDetail(msg)
		default:
			return m.updateList(msg)
		}
	}

	if m.focus == agentFocusList {
		cmd := m.t.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *agentsModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	if m.t.filtering {
		cmd := m.t.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "enter":
		row := m.t.SelectedRow()
		if len(row) < 2 {
			return m, nil
		}
		acct, name := row[0], row[1]
		m.selectedAgent = acct + "/" + name
		m.status = statusWIP.Render("Loading builds…")
		return m, m.loadBuilds(acct, name)

	case "R":
		m.status = statusWIP.Render("Refreshing…")
		return m, m.load()
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *agentsModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	rightH := m.rightPaneHeight()
	maxScroll := len(m.rightLines) - rightH
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "backspace":
		m.focus = agentFocusList
		m.builds = nil
		m.rightLines = nil
		m.rightScroll = 0
		m.t.SetSize(m.width, m.height)
		return m, nil
	case "up", "k":
		if m.rightScroll > 0 {
			m.rightScroll--
		}
	case "down", "j":
		if m.rightScroll < maxScroll {
			m.rightScroll++
		}
	case "home", "g":
		m.rightScroll = 0
	case "end", "G":
		m.rightScroll = maxScroll
	case "pgup":
		m.rightScroll -= rightH
		if m.rightScroll < 0 {
			m.rightScroll = 0
		}
	case "pgdown":
		m.rightScroll += rightH
		if m.rightScroll > maxScroll {
			m.rightScroll = maxScroll
		}
	}
	return m, nil
}

func (m *agentsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	if m.focus == agentFocusDetail {
		return m.renderDetail()
	}
	return m.t.View()
}

func (m *agentsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.computeLeftWidth()
	if m.focus == agentFocusDetail {
		m.t.SetSize(m.leftWidth, h)
	} else {
		m.t.SetSize(w, h)
	}
}

func (m *agentsModel) computeLeftWidth() {
	m.leftWidth = m.width / 2
	if m.leftWidth < 30 {
		m.leftWidth = 30
	}
	if m.leftWidth > m.width-20 {
		m.leftWidth = m.width - 20
	}
}

func (m *agentsModel) rightPaneHeight() int {
	h := m.height - 2 // border top+bottom
	if h < 1 {
		h = 1
	}
	return h
}

func (m *agentsModel) Status() string { return m.status }

func (m *agentsModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case agentFocusDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back"},
			{"Esc", "nav mode"},
		}
	}
	return []KeyHint{
		{"↑↓/jk", "navigate"},
		{"/", "search"},
		{"Enter", "builds"},
		{"R", "refresh"},
		{"Esc", "nav mode"},
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *agentsModel) load() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListAgents(context.Background(), &adminv1.ListAgentsRequest{})
		if err != nil {
			return agentsLoadedMsg{err: err, at: time.Now()}
		}
		rows := make([][]string, len(resp.Agents))
		for i, a := range resp.Agents {
			rows[i] = []string{
				a.AccountName,
				a.Name,
				fmt.Sprintf("%d", a.BuildCount),
				fmt.Sprintf("%d", a.PublishedBuildCount),
				trunc(a.LatestBuildID, 12),
				a.UpdatedAt,
			}
		}
		return agentsLoadedMsg{rows: rows, at: time.Now()}
	}
}

func (m *agentsModel) loadBuilds(accountName, agentName string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.GetAgentBuilds(context.Background(), &adminv1.GetAgentBuildsRequest{
			AccountName: accountName,
			AgentName:   agentName,
		})
		if err != nil {
			return agentBuildsMsg{err: err}
		}
		return agentBuildsMsg{builds: resp.Builds}
	}
}

// ─── detail rendering ─────────────────────────────────────────────────────────

var tagBadge = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")) // green

func (m *agentsModel) buildRightLines() []string {
	if len(m.builds) == 0 {
		return []string{detailDim.Render("(no builds)")}
	}

	var lines []string
	lines = append(lines, detailHeading.Render(fmt.Sprintf("Builds for %s", m.selectedAgent)))
	lines = append(lines, "")

	for _, b := range m.builds {
		id := truncStr(b.BuildID, 12)
		tag := detailDim.Render("(untagged)")
		if b.TaggedVersion != "" {
			tag = tagBadge.Render(b.TaggedVersion)
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s",
			detailLabel.Render(id),
			detailVal.Render(b.PublishedAt),
			tag,
		))
	}
	return lines
}

func (m *agentsModel) renderDetail() string {
	leftW := m.leftWidth
	rightW := m.width - leftW - 4 // borders take 4 cols total
	if rightW < 10 {
		rightW = 10
	}
	contentH := m.rightPaneHeight()

	// Left pane: table
	leftContent := m.t.View()
	leftBox := inactiveBorder.Width(leftW).Height(contentH).Render(leftContent)

	// Right pane: builds
	rightSrc := m.rightLines
	if len(rightSrc) == 0 {
		rightSrc = []string{detailDim.Render("(no content)")}
	}

	start := m.rightScroll
	if start > len(rightSrc) {
		start = len(rightSrc)
	}
	end := start + contentH
	if end > len(rightSrc) {
		end = len(rightSrc)
	}
	rightContent := strings.Join(rightSrc[start:end], "\n")
	rightBox := activeBorder.Width(rightW).Height(contentH).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
}
