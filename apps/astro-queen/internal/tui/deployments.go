package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── focus ────────────────────────────────────────────────────────────────────

type deploymentFocus int

const (
	depFocusList deploymentFocus = iota
	depFocusDetail
	depFocusPods
	depFocusLogs
	depFocusEnv
)

// ─── messages ─────────────────────────────────────────────────────────────────

type deploymentsLoadedMsg struct {
	rows [][]string
	err  error
	at   time.Time
}

type deploymentDetailMsg struct {
	resp *adminv1.GetDeploymentResponse
	err  error
}

type deploymentDeletedMsg struct{ err error }
type deploymentRestartedMsg struct{ err error }
type podLogsMsg struct {
	logs string
	err  error
}
type podEnvMsg struct {
	containers []*adminv1.ContainerEnv
	err        error
}

// ─── model ────────────────────────────────────────────────────────────────────

type deploymentsModel struct {
	client adminv1.AdminServiceClient
	t      tableModel
	status string
	width  int
	height int

	focus        deploymentFocus
	detail       *adminv1.GetDeploymentResponse
	detailLines  []string
	detailScroll int

	selectedPod int
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

	case deploymentDetailMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
			return m, nil
		}
		m.focus = depFocusDetail
		m.detail = msg.resp
		m.detailScroll = 0
		m.detailLines = m.buildDetailLines()
		if msg.resp.Deployment != nil {
			m.status = statusOK.Render(msg.resp.Deployment.Name + " — " + msg.resp.Deployment.Namespace)
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
		// Refresh detail to show updated pod list
		if m.detail != nil && m.detail.Deployment != nil {
			return m, m.loadDetail(m.detail.Deployment.Namespace)
		}
		return m, nil

	case podLogsMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Logs error: " + msg.err.Error())
			return m, nil
		}
		m.focus = depFocusLogs
		m.detailScroll = 0
		m.detailLines = strings.Split(msg.logs, "\n")
		m.status = statusOK.Render("Pod logs")
		return m, nil

	case podEnvMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Env error: " + msg.err.Error())
			return m, nil
		}
		m.focus = depFocusEnv
		m.detailScroll = 0
		m.detailLines = m.buildEnvLines(msg.containers)
		m.status = statusOK.Render("Pod environment")
		return m, nil

	case tea.KeyMsg:
		switch m.focus {
		case depFocusDetail:
			return m.updateDetail(msg)
		case depFocusPods:
			return m.updatePods(msg)
		case depFocusLogs, depFocusEnv:
			return m.updateOverlay(msg)
		default:
			return m.updateList(msg)
		}
	}

	if m.focus == depFocusList {
		cmd := m.t.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *deploymentsModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "enter":
		row := m.t.SelectedRow()
		if len(row) < 3 || row[2] == "" {
			return m, nil
		}
		ns := row[2]
		m.status = statusWIP.Render("Loading detail…")
		return m, m.loadDetail(ns)

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

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *deploymentsModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	maxScroll := len(m.detailLines) - m.detailViewHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusList
		m.detail = nil
		m.detailLines = nil
		m.detailScroll = 0
		return m, nil
	case "p":
		if m.detail != nil && m.detail.ClusterStatus != nil && len(m.detail.ClusterStatus.Pods) > 0 {
			m.focus = depFocusPods
			m.selectedPod = 0
			return m, nil
		}
	case "up", "k":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "down", "j":
		if m.detailScroll < maxScroll {
			m.detailScroll++
		}
	case "home", "g":
		m.detailScroll = 0
	case "end", "G":
		m.detailScroll = maxScroll
	case "pgup":
		m.detailScroll -= m.detailViewHeight()
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
	case "pgdown":
		m.detailScroll += m.detailViewHeight()
		if m.detailScroll > maxScroll {
			m.detailScroll = maxScroll
		}
	}
	return m, nil
}

func (m *deploymentsModel) updatePods(msg tea.KeyMsg) (Tab, tea.Cmd) {
	pods := m.detail.ClusterStatus.Pods
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusDetail
		m.detailScroll = 0
		m.detailLines = m.buildDetailLines()
		return m, nil
	case "j", "down":
		if m.selectedPod < len(pods)-1 {
			m.selectedPod++
		}
	case "k", "up":
		if m.selectedPod > 0 {
			m.selectedPod--
		}
	case "l":
		p := pods[m.selectedPod]
		ns := p.Namespace
		name := p.Name
		m.status = statusWIP.Render("Loading logs…")
		return m, func() tea.Msg {
			resp, err := m.client.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
				Namespace: ns, Pod: name, TailLines: 100,
			})
			if err != nil {
				return podLogsMsg{err: err}
			}
			return podLogsMsg{logs: resp.Logs}
		}
	case "e":
		p := pods[m.selectedPod]
		ns := p.Namespace
		name := p.Name
		m.status = statusWIP.Render("Loading env…")
		return m, func() tea.Msg {
			resp, err := m.client.GetPodEnv(context.Background(), &adminv1.GetPodEnvRequest{
				Namespace: ns, Pod: name,
			})
			if err != nil {
				return podEnvMsg{err: err}
			}
			return podEnvMsg{containers: resp.Containers}
		}
	case "r":
		p := pods[m.selectedPod]
		ns := p.Namespace
		name := p.Name
		return m, func() tea.Msg {
			return showConfirmMsg{
				text: fmt.Sprintf("Restart pod\n%s", statusErr.Render(name)),
				fn: func() tea.Cmd {
					return func() tea.Msg {
						_, err := m.client.RestartDeployment(
							context.Background(),
							&adminv1.RestartDeploymentRequest{Namespace: ns, Pod: name},
						)
						return deploymentRestartedMsg{err}
					}
				},
			}
		}
	}
	return m, nil
}

func (m *deploymentsModel) updateOverlay(msg tea.KeyMsg) (Tab, tea.Cmd) {
	maxScroll := len(m.detailLines) - m.detailViewHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusPods
		m.detailScroll = 0
		return m, nil
	case "up", "k":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "down", "j":
		if m.detailScroll < maxScroll {
			m.detailScroll++
		}
	case "home", "g":
		m.detailScroll = 0
	case "end", "G":
		m.detailScroll = maxScroll
	case "pgup":
		m.detailScroll -= m.detailViewHeight()
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
	case "pgdown":
		m.detailScroll += m.detailViewHeight()
		if m.detailScroll > maxScroll {
			m.detailScroll = maxScroll
		}
	}
	return m, nil
}

func (m *deploymentsModel) buildEnvLines(containers []*adminv1.ContainerEnv) []string {
	var lines []string
	for _, c := range containers {
		lines = append(lines, detailHeading.Render("── "+c.Container+" ──────────────────────────"))
		for _, v := range c.Vars {
			if v.ValueFrom != "" {
				lines = append(lines, detailLabel.Render(v.Name+"=")+"  "+detailDim.Render(v.ValueFrom))
			} else {
				lines = append(lines, detailLabel.Render(v.Name+"=")+detailVal.Render(v.Value))
			}
		}
		lines = append(lines, "")
	}
	return lines
}

func (m *deploymentsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.focus {
	case depFocusDetail:
		return m.renderDetail()
	case depFocusPods:
		return m.renderPods()
	case depFocusLogs, depFocusEnv:
		return m.renderDetail() // reuse scrollable view for overlay content
	}
	return m.t.View()
}

func (m *deploymentsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *deploymentsModel) Status() string { return m.status }

func (m *deploymentsModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case depFocusDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"p", "pods"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back to list"},
			{"Esc", "nav mode"},
		}
	case depFocusPods:
		return []KeyHint{
			{"↑↓/jk", "select pod"},
			{"l", "logs"},
			{"e", "env"},
			{"r", "restart"},
			{"Backspace", "back"},
			{"Esc", "nav mode"},
		}
	case depFocusLogs, depFocusEnv:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back to pods"},
			{"Esc", "nav mode"},
		}
	}
	return []KeyHint{
		{"↑↓/jk", "navigate"},
		{"Enter", "detail"},
		{"d", "delete"},
		{"r", "restart"},
		{"R", "refresh"},
		{"Esc", "nav mode"},
	}
}

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

func (m *deploymentsModel) loadDetail(namespace string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.GetDeployment(context.Background(), &adminv1.GetDeploymentRequest{Namespace: namespace})
		return deploymentDetailMsg{resp: resp, err: err}
	}
}

// ─── detail rendering ─────────────────────────────────────────────────────────

var (
	detailHeading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	detailLabel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	detailVal     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	detailDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m *deploymentsModel) detailViewHeight() int {
	if m.height > 2 {
		return m.height - 2
	}
	return 1
}

func (m *deploymentsModel) renderDetail() string {
	vh := m.detailViewHeight()
	if len(m.detailLines) == 0 {
		return detailDim.Render("(no detail)")
	}

	end := m.detailScroll + vh
	if end > len(m.detailLines) {
		end = len(m.detailLines)
	}
	start := m.detailScroll
	if start > len(m.detailLines) {
		start = len(m.detailLines)
	}

	visible := m.detailLines[start:end]
	return strings.Join(visible, "\n")
}

var podSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Background(lipgloss.Color("236"))

func (m *deploymentsModel) renderPods() string {
	if m.detail == nil || m.detail.ClusterStatus == nil {
		return detailDim.Render("(no pods)")
	}
	pods := m.detail.ClusterStatus.Pods
	var lines []string
	lines = append(lines, detailHeading.Render("── Select Pod ──────────────────────────"))
	lines = append(lines, "")
	for i, p := range pods {
		phase := p.Phase
		if phase == "Running" {
			phase = statusGood.Render(phase)
		} else if phase == "Failed" {
			phase = statusErr.Render(phase)
		} else {
			phase = statusWIP.Render(phase)
		}
		line := fmt.Sprintf("  %s  %s", p.Name, phase)
		if i == m.selectedPod {
			line = podSelected.Render("> "+p.Name) + "  " + phase
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, detailDim.Render("  l=logs  e=env  r=restart  Backspace=back"))
	vh := m.detailViewHeight()
	if len(lines) > vh {
		lines = lines[:vh]
	}
	return strings.Join(lines, "\n")
}

func (m *deploymentsModel) buildDetailLines() []string {
	if m.detail == nil {
		return nil
	}
	var lines []string

	d := m.detail.Deployment
	if d != nil {
		lines = append(lines, detailHeading.Render("── Deployment ──────────────────────────"))
		lines = append(lines, kv("Agent", d.Name))
		lines = append(lines, kv("Account", d.AccountName))
		lines = append(lines, kv("Namespace", d.Namespace))
		lines = append(lines, kv("Build ID", d.BuildID))
		lines = append(lines, kv("Status", d.Status))
		lines = append(lines, kv("Deployed At", d.CreatedAt))
		lines = append(lines, "")
	}

	// Spec JSON
	if m.detail.SpecJSON != "" {
		lines = append(lines, detailHeading.Render("── Spec ────────────────────────────────"))
		lines = append(lines, formatSpecJSON(m.detail.SpecJSON)...)
		lines = append(lines, "")
	}

	cs := m.detail.ClusterStatus
	if cs == nil {
		return lines
	}

	// K8s Deployments
	if len(cs.Deployments) > 0 {
		lines = append(lines, detailHeading.Render("── K8s Deployments ─────────────────────"))
		for _, dep := range cs.Deployments {
			lines = append(lines, detailLabel.Render(dep.Name))
			lines = append(lines, kv("  Replicas", fmt.Sprintf("%d/%d ready", dep.ReadyReplicas, dep.Replicas)))
			lines = append(lines, kv("  Available", fmt.Sprintf("%d", dep.AvailableReplicas)))
			lines = append(lines, kv("  Created", dep.CreatedAt))
		}
		lines = append(lines, "")
	}

	// Pods
	if len(cs.Pods) > 0 {
		lines = append(lines, detailHeading.Render("── Pods ────────────────────────────────"))
		for _, p := range cs.Pods {
			phase := p.Phase
			if phase == "Running" {
				phase = statusGood.Render(phase)
			} else if phase == "Failed" {
				phase = statusErr.Render(phase)
			} else {
				phase = statusWIP.Render(phase)
			}
			lines = append(lines, detailLabel.Render(p.Name)+"  "+phase)
			lines = append(lines, kv("  Node", p.NodeName))
			lines = append(lines, kv("  IP", p.PodIP))
			lines = append(lines, kv("  Created", p.CreatedAt))
		}
		lines = append(lines, "")
	}

	// Services
	if len(cs.Services) > 0 {
		lines = append(lines, detailHeading.Render("── Services ────────────────────────────"))
		for _, svc := range cs.Services {
			lines = append(lines, detailLabel.Render(svc.Name))
			lines = append(lines, kv("  Type", svc.Type))
			lines = append(lines, kv("  ClusterIP", svc.ClusterIP))
			if len(svc.ExternalIP) > 0 {
				lines = append(lines, kv("  ExternalIP", strings.Join(svc.ExternalIP, ", ")))
			}
			for _, port := range svc.Ports {
				lines = append(lines, kv("  Port", fmt.Sprintf("%d→%s/%s", port.Port, port.TargetPort, port.Protocol)))
			}
			lines = append(lines, kv("  Created", svc.CreatedAt))
		}
		lines = append(lines, "")
	}

	return lines
}

func kv(label, value string) string {
	if value == "" {
		value = "-"
	}
	return detailLabel.Render(label+": ") + detailVal.Render(value)
}

func formatSpecJSON(raw string) []string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return []string{detailDim.Render(raw)}
	}
	return strings.Split(buf.String(), "\n")
}
