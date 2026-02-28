package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type accountsLoadedMsg struct {
	rows [][]string
	ids  []string // account IDs parallel to rows
	err  error
	at   time.Time
}

type accountRenamedMsg struct{ err error }

// ─── model ────────────────────────────────────────────────────────────────────

type accountsModel struct {
	client adminv1.AdminServiceClient
	t      tableModel
	ids    []string // account IDs parallel to table rows
	status string
	width  int
	height int
}

func newAccountsModel(client adminv1.AdminServiceClient) *accountsModel {
	t := newTableModel([]string{"Name", "Type", "Owner", "Members", "Created At"})
	t.SetFocused(true)
	return &accountsModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *accountsModel) Name() string { return "Accounts" }

func (m *accountsModel) Init() tea.Cmd { return m.load() }

func (m *accountsModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case accountsLoadedMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
		} else {
			m.t.SetRows(msg.rows)
			m.ids = msg.ids
			m.status = statusOK.Render(fmt.Sprintf(
				"%d accounts  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
			))
		}
		return m, nil

	case accountRenamedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case tea.KeyMsg:
		switch msg.String() {
		case "R":
			m.status = statusWIP.Render("Refreshing…")
			return m, m.load()

		case "e":
			row := m.t.SelectedRow()
			idx := m.t.selected
			if len(row) == 0 || idx < 0 || idx >= len(m.ids) {
				return m, nil
			}
			accountID := m.ids[idx]
			currentName := row[0]
			return m, func() tea.Msg {
				return showInputMsg{
					title:       fmt.Sprintf("Edit account name %s", statusWIP.Render(currentName)),
					placeholder: "new-account-name",
					fn: func(newName string) tea.Cmd {
						if newName == "" {
							return nil
						}
						return func() tea.Msg {
							_, err := m.client.RenameAccount(
								context.Background(),
								&adminv1.RenameAccountRequest{
									AccountID: accountID,
									NewName:   newName,
								},
							)
							return accountRenamedMsg{err}
						}
					},
				}
			}
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *accountsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	return m.t.View()
}

func (m *accountsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *accountsModel) Status() string { return m.status }

func (m *accountsModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	return []KeyHint{
		{"↑↓/jk", "navigate"},
		{"e", "edit name"},
		{"R", "refresh"},
		{"Esc", "nav mode"},
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *accountsModel) load() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListAccounts(context.Background(), &adminv1.ListAccountsRequest{})
		if err != nil {
			return accountsLoadedMsg{err: err, at: time.Now()}
		}
		rows := make([][]string, len(resp.Accounts))
		ids := make([]string, len(resp.Accounts))
		for i, a := range resp.Accounts {
			rows[i] = []string{
				a.Name,
				a.Type,
				trunc(a.OwnerUserID, 12),
				fmt.Sprintf("%d", a.MemberCount),
				a.CreatedAt,
			}
			ids[i] = a.ID
		}
		return accountsLoadedMsg{rows: rows, ids: ids, at: time.Now()}
	}
}
