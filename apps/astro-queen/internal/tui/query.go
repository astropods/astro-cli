package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type queryResultMsg struct {
	resp *adminv1.QueryDatabaseResponse
	err  error
}

// ─── focus constants ──────────────────────────────────────────────────────────

type queryFocus int

const (
	focusInput queryFocus = iota
	focusTable
)

// ─── model ────────────────────────────────────────────────────────────────────

type queryModel struct {
	client adminv1.AdminServiceClient
	input  textinput.Model
	t      tableModel
	focus  queryFocus
	status string
	width  int
	height int
}

func newQueryModel(client adminv1.AdminServiceClient) *queryModel {
	ti := textinput.New()
	ti.Placeholder = "SELECT * FROM deployments LIMIT 10"
	ti.CharLimit = 500
	ti.Width = 60

	t := newTableModel(nil)

	return &queryModel{
		client: client,
		input:  ti,
		t:      t,
		focus:  focusInput,
		status: descStyle.Render("Type a query and press Enter to run it."),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *queryModel) Name() string { return "Query" }

func (m *queryModel) Init() tea.Cmd {
	m.focus = focusInput
	m.input.Focus()
	return textinput.Blink
}

func (m *queryModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case queryResultMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
			m.focus = focusInput
			m.input.Focus()
			return m, textinput.Blink
		}
		resp := msg.resp
		rows := make([][]string, len(resp.Rows))
		for i, row := range resp.Rows {
			rows[i] = row.Values
		}
		m.t.headers = resp.Columns
		m.t.SetRows(rows)
		m.t.SetFocused(true)
		m.status = statusOK.Render(fmt.Sprintf("%d rows, %d columns", len(rows), len(resp.Columns)))
		m.focus = focusTable
		m.input.Blur()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "esc":
			if m.focus == focusInput {
				m.focus = focusTable
				m.input.Blur()
				m.t.SetFocused(true)
				return m, nil
			}
			m.focus = focusInput
			m.input.Focus()
			m.t.SetFocused(false)
			return m, textinput.Blink

		case "i":
			if m.focus == focusTable {
				m.focus = focusInput
				m.input.Focus()
				m.t.SetFocused(false)
				return m, textinput.Blink
			}

		case "enter":
			if m.focus == focusInput {
				m.status = statusWIP.Render("Running…")
				return m, m.runQuery()
			}
		}
	}

	var cmd tea.Cmd
	if m.focus == focusInput {
		m.input, cmd = m.input.Update(msg)
	} else {
		cmd = m.t.Update(msg)
	}
	return m, cmd
}

func (m *queryModel) View(w, _ int) string {
	var inputHeader string
	if m.focus == focusInput {
		inputHeader = activeTabStyle.Render("SQL Query") +
			descStyle.Render("  Enter to run  •  Tab/Esc to focus results")
	} else {
		inputHeader = inactiveTabStyle.Render("SQL Query") +
			descStyle.Render("  i/Tab/Esc to edit")
	}
	inputHeader = barBg.Width(w).Render(inputHeader)

	inputRow := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(w).
		Render(m.input.View())

	var resultsHeader string
	if m.focus == focusTable {
		resultsHeader = barBg.Width(w).Render(
			activeTabStyle.Render("Results") + "  " + m.status,
		)
	} else {
		resultsHeader = barBg.Width(w).Render(
			inactiveTabStyle.Render("Results") + "  " + m.status,
		)
	}

	var tableSection string
	if m.focus == focusTable {
		tableSection = m.t.View()
	} else {
		tableSection = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(m.t.View())
	}

	return strings.Join([]string{
		inputHeader,
		inputRow,
		resultsHeader,
		tableSection,
	}, "\n")
}

func (m *queryModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.input.Width = w - 10
	// Reserve 3 lines for input header, input row, results header.
	tableH := h - 3
	if tableH < 4 {
		tableH = 4
	}
	m.t.SetSize(w, tableH)
}

func (m *queryModel) Status() string { return m.status }

func (m *queryModel) Hints() []KeyHint {
	if m.focus == focusInput {
		return []KeyHint{
			{"Enter", "run query"},
			{"Tab/Esc", "focus results"},
			{"q", "quit"},
		}
	}
	return []KeyHint{
		{"↑↓", "navigate results"},
		{"i/Tab/Esc", "edit query"},
		{"q", "quit"},
	}
}

func (m *queryModel) ConsumesKey(key string) bool {
	switch key {
	case "tab", "esc":
		return true
	case "q", "Q":
		return m.focus == focusInput
	}
	return false
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *queryModel) runQuery() tea.Cmd {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		return nil
	}
	return func() tea.Msg {
		resp, err := m.client.QueryDatabase(context.Background(), &adminv1.QueryDatabaseRequest{Query: q})
		if err != nil {
			return queryResultMsg{err: err}
		}
		return queryResultMsg{resp: resp}
	}
}
