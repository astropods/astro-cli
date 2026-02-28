package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type agentsLoadedMsg struct {
	rows [][]string
	err  error
	at   time.Time
}

// ─── model ────────────────────────────────────────────────────────────────────

type agentsModel struct {
	client adminv1.AdminServiceClient
	t      tableModel
	status string
	width  int
	height int
}

func newAgentsModel(client adminv1.AdminServiceClient) *agentsModel {
	t := newTableModel([]string{"Account", "Name", "Registry", "Builds", "Published", "Latest Build", "Updated At"})
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

	case tea.KeyMsg:
		if m.t.filtering {
			cmd := m.t.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "R":
			m.status = statusWIP.Render("Refreshing…")
			return m, m.load()
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *agentsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	return m.t.View()
}

func (m *agentsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
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
	return []KeyHint{
		{"↑↓/jk", "navigate"},
		{"/", "search"},
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
				a.Registry,
				fmt.Sprintf("%d", a.BuildCount),
				fmt.Sprintf("%d", a.PublishedBuildCount),
				trunc(a.LatestBuildID, 12),
				a.UpdatedAt,
			}
		}
		return agentsLoadedMsg{rows: rows, at: time.Now()}
	}
}
