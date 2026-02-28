package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type deploymentsLoadedMsg struct {
	rows [][]string
	err  error
	at   time.Time
}

type deploymentDeletedMsg struct{ err error }
type deploymentRestartedMsg struct{ err error }

// ─── model ────────────────────────────────────────────────────────────────────

type deploymentsModel struct {
	client adminv1.AdminServiceClient
	t      tableModel
	status string
	width  int
	height int
}

func newDeploymentsModel(client adminv1.AdminServiceClient) *deploymentsModel {
	t := newTableModel([]string{"Account", "Name", "Namespace", "Status", "Build ID", "Deployed At"})
	t.SetFocused(true)
	return &deploymentsModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *deploymentsModel) Name() string { return "Deployments" }

func (m *deploymentsModel) Init() tea.Cmd { return m.load() }

func (m *deploymentsModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case deploymentsLoadedMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
		} else {
			m.t.SetRows(msg.rows)
			m.status = statusOK.Render(fmt.Sprintf(
				"%d deployments  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
			))
		}
		return m, nil

	case deploymentDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case deploymentRestartedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.status = statusGood.Render("Pod deleted — Kubernetes will recreate it.")
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "R":
			m.status = statusWIP.Render("Refreshing…")
			return m, m.load()

		case "d":
			row := m.t.SelectedRow()
			if len(row) < 3 || row[2] == "" {
				return m, nil
			}
			ns := row[2]
			return m, func() tea.Msg {
				return showConfirmMsg{
					text: fmt.Sprintf(
						"Delete all resources in namespace\n%s",
						statusErr.Render(ns),
					),
					fn: func() tea.Cmd {
						return func() tea.Msg {
							_, err := m.client.DeleteDeployment(
								context.Background(),
								&adminv1.DeleteDeploymentRequest{Namespace: ns},
							)
							return deploymentDeletedMsg{err}
						}
					},
				}
			}

		case "r":
			row := m.t.SelectedRow()
			if len(row) < 3 || row[2] == "" {
				return m, nil
			}
			ns := row[2]
			return m, func() tea.Msg {
				return showInputMsg{
					title:       fmt.Sprintf("Restart pod in %s", statusWIP.Render(ns)),
					placeholder: "pod-name-xxxxx",
					fn: func(pod string) tea.Cmd {
						if pod == "" {
							return nil
						}
						return func() tea.Msg {
							_, err := m.client.RestartDeployment(
								context.Background(),
								&adminv1.RestartDeploymentRequest{Namespace: ns, Pod: pod},
							)
							return deploymentRestartedMsg{err}
						}
					},
				}
			}
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *deploymentsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	return m.t.View()
}

func (m *deploymentsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *deploymentsModel) Status() string { return m.status }

func (m *deploymentsModel) Hints() []KeyHint {
	return []KeyHint{
		{"↑↓", "navigate"},
		{"d", "delete"},
		{"r", "restart"},
		{"R", "refresh"},
		{"Tab", "next tab"},
		{"q", "quit"},
	}
}

func (m *deploymentsModel) ConsumesKey(_ string) bool { return false }

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *deploymentsModel) load() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListDeployments(context.Background(), &adminv1.ListDeploymentsRequest{})
		if err != nil {
			return deploymentsLoadedMsg{err: err, at: time.Now()}
		}
		rows := make([][]string, len(resp.Deployments))
		for i, d := range resp.Deployments {
			rows[i] = []string{
				d.AccountName,
				d.Name,
				d.Namespace,
				d.Status,
				trunc(d.BuildID, 12),
				d.CreatedAt,
			}
		}
		return deploymentsLoadedMsg{rows: rows, at: time.Now()}
	}
}
