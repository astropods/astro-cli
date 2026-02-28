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
	depFocusResourcePicker
	depFocusResourceList
	depFocusResourceDetail
	depFocusLogs
	depFocusEnv
)

// ─── resource types ───────────────────────────────────────────────────────────

type resourceType int

const (
	resTypePods resourceType = iota
	resTypeServices
	resTypeIngresses
	resTypeNetworkPolicies
	resTypeK8sDeployments
	resTypeEvents
	resTypeCount // sentinel
)

func (r resourceType) label() string {
	switch r {
	case resTypePods:
		return "Pods"
	case resTypeServices:
		return "Services"
	case resTypeIngresses:
		return "Ingresses"
	case resTypeNetworkPolicies:
		return "Network Policies"
	case resTypeK8sDeployments:
		return "K8s Deployments"
	case resTypeEvents:
		return "Events"
	}
	return ""
}

func (m *deploymentsModel) resourceCount(r resourceType) int {
	cs := m.detail.ClusterStatus
	if cs == nil {
		return 0
	}
	switch r {
	case resTypePods:
		return len(cs.Pods)
	case resTypeServices:
		return len(cs.Services)
	case resTypeIngresses:
		return len(cs.Ingresses)
	case resTypeNetworkPolicies:
		return len(cs.NetworkPolicies)
	case resTypeK8sDeployments:
		return len(cs.Deployments)
	case resTypeEvents:
		return len(cs.Events)
	}
	return 0
}

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

	selectedResourceType resourceType
	selectedResource     int
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
		case depFocusResourcePicker:
			return m.updateResourcePicker(msg)
		case depFocusResourceList:
			return m.updateResourceList(msg)
		case depFocusResourceDetail:
			return m.updateResourceDetail(msg)
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
	case "b":
		if m.detail != nil && m.detail.ClusterStatus != nil {
			m.focus = depFocusResourcePicker
			m.selectedResourceType = resTypePods
			m.selectedResource = 0
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

func (m *deploymentsModel) updateResourcePicker(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusDetail
		m.detailScroll = 0
		m.detailLines = m.buildDetailLines()
		return m, nil
	case "j", "down":
		for next := m.selectedResourceType + 1; next < resTypeCount; next++ {
			m.selectedResourceType = next
			return m, nil
		}
	case "k", "up":
		for prev := m.selectedResourceType - 1; prev >= 0; prev-- {
			m.selectedResourceType = prev
			return m, nil
		}
	case "enter":
		if m.resourceCount(m.selectedResourceType) > 0 {
			m.focus = depFocusResourceList
			m.selectedResource = 0
			return m, nil
		}
	}
	return m, nil
}

func (m *deploymentsModel) updateResourceList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	count := m.resourceCount(m.selectedResourceType)
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusResourcePicker
		return m, nil
	case "j", "down":
		if m.selectedResource < count-1 {
			m.selectedResource++
		}
	case "k", "up":
		if m.selectedResource > 0 {
			m.selectedResource--
		}
	case "enter":
		m.focus = depFocusResourceDetail
		m.detailScroll = 0
		m.detailLines = m.buildResourceDescribeLines()
		return m, nil
	case "l":
		if m.selectedResourceType == resTypePods {
			p := m.detail.ClusterStatus.Pods[m.selectedResource]
			m.status = statusWIP.Render("Loading logs…")
			return m, func() tea.Msg {
				resp, err := m.client.GetPodLogs(context.Background(), &adminv1.GetPodLogsRequest{
					Namespace: p.Namespace, Pod: p.Name, TailLines: 100,
				})
				if err != nil {
					return podLogsMsg{err: err}
				}
				return podLogsMsg{logs: resp.Logs}
			}
		}
	case "e":
		if m.selectedResourceType == resTypePods {
			p := m.detail.ClusterStatus.Pods[m.selectedResource]
			m.status = statusWIP.Render("Loading env…")
			return m, func() tea.Msg {
				resp, err := m.client.GetPodEnv(context.Background(), &adminv1.GetPodEnvRequest{
					Namespace: p.Namespace, Pod: p.Name,
				})
				if err != nil {
					return podEnvMsg{err: err}
				}
				return podEnvMsg{containers: resp.Containers}
			}
		}
	case "r":
		if m.selectedResourceType == resTypePods {
			p := m.detail.ClusterStatus.Pods[m.selectedResource]
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
	}
	return m, nil
}

func (m *deploymentsModel) updateResourceDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	maxScroll := len(m.detailLines) - m.detailViewHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusResourceList
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

func (m *deploymentsModel) updateOverlay(msg tea.KeyMsg) (Tab, tea.Cmd) {
	maxScroll := len(m.detailLines) - m.detailViewHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch msg.String() {
	case "backspace", "esc":
		m.focus = depFocusResourceList
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
	case depFocusResourcePicker:
		return m.renderResourcePicker()
	case depFocusResourceList:
		return m.renderResourceList()
	case depFocusResourceDetail:
		return m.renderDetail()
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
			{"b", "browse resources"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back to list"},
			{"Esc", "nav mode"},
		}
	case depFocusResourcePicker:
		return []KeyHint{
			{"↑↓/jk", "select type"},
			{"Enter", "browse"},
			{"Backspace", "back"},
			{"Esc", "nav mode"},
		}
	case depFocusResourceList:
		hints := []KeyHint{
			{"↑↓/jk", "select"},
			{"Enter", "describe"},
		}
		if m.selectedResourceType == resTypePods {
			hints = append(hints,
				KeyHint{"l", "logs"},
				KeyHint{"e", "env"},
				KeyHint{"r", "restart"},
			)
		}
		hints = append(hints, KeyHint{"Backspace", "back"}, KeyHint{"Esc", "nav mode"})
		return hints
	case depFocusResourceDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back"},
			{"Esc", "nav mode"},
		}
	case depFocusLogs, depFocusEnv:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"PgUp/Dn", "page"},
			{"Backspace", "back"},
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

var resourceSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Background(lipgloss.Color("236"))

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

func (m *deploymentsModel) renderResourcePicker() string {
	var lines []string
	lines = append(lines, detailHeading.Render("── Select Resource Type ────────────────"))
	lines = append(lines, "")
	for i := resourceType(0); i < resTypeCount; i++ {
		count := m.resourceCount(i)
		countStr := fmt.Sprintf("(%d)", count)
		if count == 0 {
			countStr = detailDim.Render("(0)")
		}
		line := fmt.Sprintf("  %s  %s", i.label(), countStr)
		if i == m.selectedResourceType {
			line = resourceSelected.Render(fmt.Sprintf("> %s", i.label())) + "  " + countStr
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, detailDim.Render("  Enter=browse  Backspace=back"))
	vh := m.detailViewHeight()
	if len(lines) > vh {
		lines = lines[:vh]
	}
	return strings.Join(lines, "\n")
}

func (m *deploymentsModel) renderResourceList() string {
	cs := m.detail.ClusterStatus
	if cs == nil {
		return detailDim.Render("(no resources)")
	}

	var lines []string
	lines = append(lines, detailHeading.Render("── "+m.selectedResourceType.label()+" ────────────────────────────"))
	lines = append(lines, "")

	switch m.selectedResourceType {
	case resTypePods:
		for i, p := range cs.Pods {
			phase := p.Phase
			if phase == "Running" {
				phase = statusGood.Render(phase)
			} else if phase == "Failed" {
				phase = statusErr.Render(phase)
			} else {
				phase = statusWIP.Render(phase)
			}
			line := fmt.Sprintf("  %s  %s", p.Name, phase)
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+p.Name) + "  " + phase
			}
			lines = append(lines, line)
		}
	case resTypeServices:
		for i, svc := range cs.Services {
			line := fmt.Sprintf("  %s  %s", svc.Name, detailDim.Render(svc.Type))
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+svc.Name) + "  " + detailDim.Render(svc.Type)
			}
			lines = append(lines, line)
		}
	case resTypeIngresses:
		for i, ing := range cs.Ingresses {
			cls := ing.IngressClassName
			if cls == "" {
				cls = "-"
			}
			line := fmt.Sprintf("  %s  %s", ing.Name, detailDim.Render(cls))
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+ing.Name) + "  " + detailDim.Render(cls)
			}
			lines = append(lines, line)
		}
	case resTypeNetworkPolicies:
		for i, np := range cs.NetworkPolicies {
			types := strings.Join(np.PolicyTypes, ",")
			if types == "" {
				types = "-"
			}
			line := fmt.Sprintf("  %s  %s", np.Name, detailDim.Render(types))
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+np.Name) + "  " + detailDim.Render(types)
			}
			lines = append(lines, line)
		}
	case resTypeK8sDeployments:
		for i, dep := range cs.Deployments {
			ready := fmt.Sprintf("%d/%d", dep.ReadyReplicas, dep.Replicas)
			line := fmt.Sprintf("  %s  %s", dep.Name, detailDim.Render(ready))
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+dep.Name) + "  " + detailDim.Render(ready)
			}
			lines = append(lines, line)
		}
	case resTypeEvents:
		for i, ev := range cs.Events {
			typeStyle := detailDim
			if ev.Type == "Warning" {
				typeStyle = statusErr
			}
			summary := ev.Reason + ": " + ev.Message
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			line := fmt.Sprintf("  %s  %s", typeStyle.Render(ev.Type), summary)
			if i == m.selectedResource {
				line = resourceSelected.Render("> "+ev.Type) + "  " + summary
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	hint := "  Enter=describe"
	if m.selectedResourceType == resTypePods {
		hint += "  l=logs  e=env  r=restart"
	}
	hint += "  Backspace=back"
	lines = append(lines, detailDim.Render(hint))
	vh := m.detailViewHeight()
	if len(lines) > vh {
		lines = lines[:vh]
	}
	return strings.Join(lines, "\n")
}

func (m *deploymentsModel) buildResourceDescribeLines() []string {
	cs := m.detail.ClusterStatus
	if cs == nil {
		return nil
	}
	switch m.selectedResourceType {
	case resTypePods:
		if m.selectedResource < len(cs.Pods) {
			return m.buildPodDetailLines(cs.Pods[m.selectedResource])
		}
	case resTypeServices:
		if m.selectedResource < len(cs.Services) {
			return m.buildServiceDetailLines(cs.Services[m.selectedResource])
		}
	case resTypeIngresses:
		if m.selectedResource < len(cs.Ingresses) {
			return m.buildIngressDetailLines(cs.Ingresses[m.selectedResource])
		}
	case resTypeNetworkPolicies:
		if m.selectedResource < len(cs.NetworkPolicies) {
			return m.buildNetworkPolicyDetailLines(cs.NetworkPolicies[m.selectedResource])
		}
	case resTypeK8sDeployments:
		if m.selectedResource < len(cs.Deployments) {
			return m.buildK8sDeploymentDetailLines(cs.Deployments[m.selectedResource])
		}
	case resTypeEvents:
		if m.selectedResource < len(cs.Events) {
			return m.buildEventDetailLines(cs.Events[m.selectedResource])
		}
	}
	return nil
}

func (m *deploymentsModel) buildPodDetailLines(p *adminv1.K8sPodInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── Pod: "+p.Name+" ──────────────────"))
	lines = append(lines, kv("Namespace", p.Namespace))
	lines = append(lines, kv("Phase", p.Phase))
	lines = append(lines, kv("Node", p.NodeName))
	lines = append(lines, kv("IP", p.PodIP))
	lines = append(lines, kv("Created", p.CreatedAt))

	if len(p.Conditions) > 0 {
		lines = append(lines, kv("Conditions", strings.Join(p.Conditions, ", ")))
	}

	if len(p.ContainerStatuses) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailHeading.Render("── Container Statuses ──────────────────"))
		for _, cs := range p.ContainerStatuses {
			readyStr := statusErr.Render("Not Ready")
			if cs.Ready {
				readyStr = statusGood.Render("Ready")
			}
			lines = append(lines, detailLabel.Render(cs.Name)+"  "+readyStr)
			lines = append(lines, kv("  State", cs.State))
			lines = append(lines, kv("  Restarts", fmt.Sprintf("%d", cs.RestartCount)))
			lines = append(lines, kv("  Image", cs.Image))
		}
	}

	if len(p.Containers) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailHeading.Render("── Resources ───────────────────────────"))
		for _, c := range p.Containers {
			lines = append(lines, detailLabel.Render(c.Name))
			req := formatResourcePair(c.RequestCPU, c.RequestMemory)
			lim := formatResourcePair(c.LimitCPU, c.LimitMemory)
			lines = append(lines, kv("  Requests", req))
			lines = append(lines, kv("  Limits", lim))
		}
	}

	if len(p.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Labels:"))
		for k, v := range p.Labels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	return lines
}

func formatResourcePair(cpu, mem string) string {
	if cpu == "" && mem == "" {
		return "-"
	}
	if cpu == "" {
		cpu = "-"
	}
	if mem == "" {
		mem = "-"
	}
	return fmt.Sprintf("cpu: %s, memory: %s", cpu, mem)
}

func (m *deploymentsModel) buildServiceDetailLines(svc *adminv1.K8sServiceInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── Service: "+svc.Name+" ──────────────"))
	lines = append(lines, kv("Namespace", svc.Namespace))
	lines = append(lines, kv("Type", svc.Type))
	lines = append(lines, kv("ClusterIP", svc.ClusterIP))
	if len(svc.ExternalIP) > 0 {
		lines = append(lines, kv("ExternalIP", strings.Join(svc.ExternalIP, ", ")))
	}
	lines = append(lines, kv("Created", svc.CreatedAt))
	if len(svc.Ports) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Ports:"))
		for _, port := range svc.Ports {
			lines = append(lines, fmt.Sprintf("  %s %s",
				detailLabel.Render(fmt.Sprintf("%d→%s", port.Port, port.TargetPort)),
				detailDim.Render(port.Protocol),
			))
			if port.Name != "" {
				lines = append(lines, "    "+detailDim.Render("name: "+port.Name))
			}
		}
	}
	if len(svc.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Labels:"))
		for k, v := range svc.Labels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	return lines
}

func (m *deploymentsModel) buildIngressDetailLines(ing *adminv1.K8sIngressInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── Ingress: "+ing.Name+" ─────────────"))
	lines = append(lines, kv("Namespace", ing.Namespace))
	lines = append(lines, kv("Class", ing.IngressClassName))
	lines = append(lines, kv("Created", ing.CreatedAt))
	if len(ing.Rules) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Rules:"))
		for _, rule := range ing.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			lines = append(lines, "  "+detailLabel.Render(host))
			for _, path := range rule.Paths {
				lines = append(lines, fmt.Sprintf("    %s → %s:%s  %s",
					detailVal.Render(path.Path),
					detailVal.Render(path.BackendService),
					detailVal.Render(path.BackendPort),
					detailDim.Render(path.PathType),
				))
			}
		}
	}
	if len(ing.TLS) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("TLS:"))
		for _, tls := range ing.TLS {
			lines = append(lines, "  "+kv("Secret", tls.SecretName))
			if len(tls.Hosts) > 0 {
				lines = append(lines, "  "+kv("Hosts", strings.Join(tls.Hosts, ", ")))
			}
		}
	}
	if len(ing.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Labels:"))
		for k, v := range ing.Labels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	return lines
}

func (m *deploymentsModel) buildNetworkPolicyDetailLines(np *adminv1.K8sNetworkPolicyInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── NetworkPolicy: "+np.Name+" ────────"))
	lines = append(lines, kv("Namespace", np.Namespace))
	if len(np.PolicyTypes) > 0 {
		lines = append(lines, kv("Policy Types", strings.Join(np.PolicyTypes, ", ")))
	}
	lines = append(lines, kv("Ingress Rules", fmt.Sprintf("%d", np.IngressRuleCount)))
	lines = append(lines, kv("Egress Rules", fmt.Sprintf("%d", np.EgressRuleCount)))
	lines = append(lines, kv("Created", np.CreatedAt))
	if len(np.PodSelectorLabels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Pod Selector:"))
		for k, v := range np.PodSelectorLabels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	if len(np.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Labels:"))
		for k, v := range np.Labels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	return lines
}

func (m *deploymentsModel) buildK8sDeploymentDetailLines(dep *adminv1.K8sDeploymentInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── K8s Deployment: "+dep.Name+" ──────"))
	lines = append(lines, kv("Namespace", dep.Namespace))
	lines = append(lines, kv("Replicas", fmt.Sprintf("%d", dep.Replicas)))
	lines = append(lines, kv("Ready", fmt.Sprintf("%d", dep.ReadyReplicas)))
	lines = append(lines, kv("Available", fmt.Sprintf("%d", dep.AvailableReplicas)))
	lines = append(lines, kv("Created", dep.CreatedAt))
	if len(dep.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabel.Render("Labels:"))
		for k, v := range dep.Labels {
			lines = append(lines, "  "+detailDim.Render(k+"=")+detailVal.Render(v))
		}
	}
	return lines
}

func (m *deploymentsModel) buildEventDetailLines(ev *adminv1.K8sEventInfo) []string {
	var lines []string
	lines = append(lines, detailHeading.Render("── Event: "+ev.Name+" ──────────────────"))
	lines = append(lines, kv("Namespace", ev.Namespace))
	typeStr := ev.Type
	if ev.Type == "Warning" {
		typeStr = statusErr.Render(ev.Type)
	}
	lines = append(lines, detailLabel.Render("Type: ")+typeStr)
	lines = append(lines, kv("Reason", ev.Reason))
	lines = append(lines, kv("Message", ev.Message))
	lines = append(lines, kv("Involved Object", ev.InvolvedObject))
	lines = append(lines, kv("Count", fmt.Sprintf("%d", ev.Count)))
	lines = append(lines, kv("First Seen", ev.FirstSeen))
	lines = append(lines, kv("Last Seen", ev.LastSeen))
	return lines
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

	// Ingresses
	if len(cs.Ingresses) > 0 {
		lines = append(lines, detailHeading.Render("── Ingresses ───────────────────────────"))
		for _, ing := range cs.Ingresses {
			lines = append(lines, detailLabel.Render(ing.Name))
			lines = append(lines, kv("  Class", ing.IngressClassName))
			for _, rule := range ing.Rules {
				host := rule.Host
				if host == "" {
					host = "*"
				}
				for _, path := range rule.Paths {
					lines = append(lines, kv("  Rule", fmt.Sprintf("%s%s → %s:%s", host, path.Path, path.BackendService, path.BackendPort)))
				}
			}
			lines = append(lines, kv("  Created", ing.CreatedAt))
		}
		lines = append(lines, "")
	}

	// Network Policies
	if len(cs.NetworkPolicies) > 0 {
		lines = append(lines, detailHeading.Render("── Network Policies ────────────────────"))
		for _, np := range cs.NetworkPolicies {
			lines = append(lines, detailLabel.Render(np.Name))
			if len(np.PolicyTypes) > 0 {
				lines = append(lines, kv("  Types", strings.Join(np.PolicyTypes, ", ")))
			}
			lines = append(lines, kv("  Ingress Rules", fmt.Sprintf("%d", np.IngressRuleCount)))
			lines = append(lines, kv("  Egress Rules", fmt.Sprintf("%d", np.EgressRuleCount)))
			lines = append(lines, kv("  Created", np.CreatedAt))
		}
		lines = append(lines, "")
	}

	// Events (Warning only, last 5)
	if len(cs.Events) > 0 {
		var warnings []*adminv1.K8sEventInfo
		for _, ev := range cs.Events {
			if ev.Type == "Warning" {
				warnings = append(warnings, ev)
			}
		}
		if len(warnings) > 0 {
			lines = append(lines, detailHeading.Render("── Warning Events ──────────────────────"))
			show := warnings
			if len(show) > 5 {
				show = show[len(show)-5:]
			}
			for _, ev := range show {
				lines = append(lines, statusErr.Render(ev.Reason)+": "+ev.Message)
				lines = append(lines, kv("  Object", ev.InvolvedObject)+"  "+detailDim.Render(ev.LastSeen))
			}
			if len(warnings) > 5 {
				lines = append(lines, detailDim.Render(fmt.Sprintf("  ... and %d more warnings", len(warnings)-5)))
			}
			lines = append(lines, "")
		}
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
