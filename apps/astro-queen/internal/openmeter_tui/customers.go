package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/postman/astro/apps/astro-queen/internal/openmeter"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type customersLoadedMsg struct {
	rows [][]string
	ids  []string
	err  error
	at   time.Time
}

type customerDeletedMsg struct{ err error }
type customerCreatedMsg struct{ err error }
type customerUpdatedMsg struct{ err error }

// ─── model ────────────────────────────────────────────────────────────────────

type customersModel struct {
	client *openmeter.Client
	t      tableModel
	ids    []string
	status string
	width  int
	height int
}

func newCustomersModel(client *openmeter.Client) *customersModel {
	t := newTableModel([]string{"ID", "Name", "Description", "Created"})
	t.SetFocused(true)
	return &customersModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *customersModel) Name() string { return "Customers" }

func (m *customersModel) Init() tea.Cmd { return m.load() }

func (m *customersModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case customersLoadedMsg:
		if msg.err != nil {
			m.status = statusOK.Render("—")
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		} else {
			m.t.SetRows(msg.rows)
			m.ids = msg.ids
			m.status = statusOK.Render(fmt.Sprintf(
				"%d customers  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
			))
		}
		return m, nil

	case customerDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case customerCreatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case customerUpdatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case tea.KeyMsg:
		if m.t.filtering {
			cmd := m.t.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "R":
			m.status = statusWIP.Render("Refreshing…")
			return m, m.load()

		case "c":
			return m, func() tea.Msg {
				return showInputMsg{
					title:       "Create customer (JSON)",
					placeholder: `{"name":"Acme Corp","description":"Main customer"}`,
					fn: func(val string) tea.Cmd {
						if val == "" {
							return nil
						}
						return m.createCustomer(val)
					},
				}
			}

		case "e":
			row := m.t.SelectedRow()
			idx := m.t.selectedRealIndex()
			if len(row) == 0 || idx < 0 || idx >= len(m.ids) {
				return m, nil
			}
			id := m.ids[idx]
			return m, func() tea.Msg {
				return showInputMsg{
					title:       fmt.Sprintf("Update customer %s (JSON)", statusWIP.Render(id)),
					placeholder: `{"name":"New Name","description":"Updated"}`,
					fn: func(val string) tea.Cmd {
						if val == "" {
							return nil
						}
						return m.updateCustomer(id, val)
					},
				}
			}

		case "d":
			idx := m.t.selectedRealIndex()
			if idx < 0 || idx >= len(m.ids) {
				return m, nil
			}
			id := m.ids[idx]
			return m, func() tea.Msg {
				return showConfirmMsg{
					text: fmt.Sprintf("Delete customer %s?", statusWIP.Render(id)),
					fn: func() tea.Cmd {
						return func() tea.Msg {
							err := m.client.DeleteCustomer(id)
							return customerDeletedMsg{err}
						}
					},
				}
			}
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *customersModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	tip := tipStyle.Render("Customers map to billable entities. Use c to create, e to edit, d to delete.")
	return tip + "\n" + m.t.View()
}

func (m *customersModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *customersModel) Status() string { return m.status }

func (m *customersModel) Hints(navMode bool) []KeyHint {
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
		{"c", "create"},
		{"e", "edit"},
		{"d", "delete"},
		{"R", "refresh"},
		{"Esc", "nav mode"},
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *customersModel) load() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListCustomers()
		if err != nil {
			return customersLoadedMsg{err: err, at: time.Now()}
		}

		var resp struct {
			Items []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				CreatedAt   string `json:"createdAt"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return customersLoadedMsg{err: fmt.Errorf("parse: %w", err), at: time.Now()}
		}
		customers := resp.Items

		rows := make([][]string, len(customers))
		ids := make([]string, len(customers))
		for i, c := range customers {
			rows[i] = []string{c.ID, c.Name, c.Description, c.CreatedAt}
			ids[i] = c.ID
		}
		return customersLoadedMsg{rows: rows, ids: ids, at: time.Now()}
	}
}

func (m *customersModel) createCustomer(jsonStr string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.client.CreateCustomer(json.RawMessage(jsonStr))
		return customerCreatedMsg{err}
	}
}

func (m *customersModel) updateCustomer(id, jsonStr string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.client.UpdateCustomer(id, json.RawMessage(jsonStr))
		return customerUpdatedMsg{err}
	}
}
