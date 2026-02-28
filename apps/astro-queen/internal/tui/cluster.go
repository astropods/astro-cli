package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"github.com/rivo/tview"
)

type clusterView struct {
	app     *App
	table   *tview.Table
	status  *tview.TextView
	subTabs *tview.TextView
	flex    *tview.Flex
	subTab  string // "pods" | "deps" | "services"
	resp    *adminv1.GetClusterStatusResponse
}

func newClusterView(a *App) *clusterView {
	v := &clusterView{app: a, subTab: "pods"}

	v.subTabs = tview.NewTextView().SetDynamicColors(true)
	v.status = tview.NewTextView().SetDynamicColors(true)

	v.table = tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetBorders(false)

	v.table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Rune() {
		case 'p':
			v.setSubTab("pods")
			return nil
		case 'k':
			v.setSubTab("deps")
			return nil
		case 's':
			v.setSubTab("services")
			return nil
		case 'R':
			go v.load()
			return nil
		}
		return ev
	})

	v.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.subTabs, 1, 0, false).
		AddItem(v.status, 1, 0, false).
		AddItem(v.table, 0, 1, true)

	v.renderSubTabs()
	return v
}

func (v *clusterView) root() tview.Primitive { return v.flex }

func (v *clusterView) load() {
	v.app.update(func() { v.status.SetText("[yellow]Loading…[-]") })

	resp, err := v.app.client.GetClusterStatus(context.Background(), &adminv1.GetClusterStatusRequest{})
	if err != nil {
		v.app.update(func() { v.status.SetText(fmt.Sprintf("[red]Error: %s[-]", err)) })
		return
	}

	v.app.update(func() {
		v.resp = resp
		v.refreshTable()

		s := resp.Summary
		ns := resp.Namespace
		if ns == "" {
			ns = "all"
		}
		v.status.SetText(fmt.Sprintf(
			"[gray]ns:%s  deps:%d  pods:%d (run:[green]%d[-] pend:[yellow]%d[-] fail:[red]%d[-])  svc:%d  %s[-]",
			ns, s.TotalDeployments, s.TotalPods,
			s.RunningPods, s.PendingPods, s.FailedPods,
			s.TotalServices, time.Now().Format("15:04:05"),
		))
	})
}

func (v *clusterView) setSubTab(tab string) {
	v.subTab = tab
	v.renderSubTabs()
	v.refreshTable()
}

func (v *clusterView) renderSubTabs() {
	tabs := []struct{ key, label, id string }{
		{"p", "Pods", "pods"},
		{"k", "Deployments", "deps"},
		{"s", "Services", "services"},
	}
	s := ""
	for _, t := range tabs {
		label := fmt.Sprintf("[%s]%s", t.key, t.label)
		if v.subTab == t.id {
			s += fmt.Sprintf("[black:blue:b] %s [-:-:-] ", label)
		} else {
			s += fmt.Sprintf("[gray] %s [-] ", label)
		}
	}
	v.subTabs.SetText(s)
}

func (v *clusterView) refreshTable() {
	v.table.Clear()
	if v.resp == nil {
		return
	}
	switch v.subTab {
	case "pods":
		for col, h := range []string{"Name", "Namespace", "Phase", "Pod IP", "Node"} {
			v.table.SetCell(0, col, headerCell(h))
		}
		for i, p := range v.resp.Pods {
			row := i + 1
			v.table.SetCell(row, 0, tview.NewTableCell(p.Name).SetExpansion(2))
			v.table.SetCell(row, 1, tview.NewTableCell(p.Namespace).SetExpansion(1))
			v.table.SetCell(row, 2, tview.NewTableCell(podPhaseColor(p.Phase)).SetExpansion(1))
			v.table.SetCell(row, 3, tview.NewTableCell(p.PodIP).SetExpansion(1))
			v.table.SetCell(row, 4, tview.NewTableCell(p.NodeName).SetExpansion(2))
		}
	case "deps":
		for col, h := range []string{"Name", "Namespace", "Replicas", "Ready", "Available"} {
			v.table.SetCell(0, col, headerCell(h))
		}
		for i, d := range v.resp.Deployments {
			row := i + 1
			v.table.SetCell(row, 0, tview.NewTableCell(d.Name).SetExpansion(2))
			v.table.SetCell(row, 1, tview.NewTableCell(d.Namespace).SetExpansion(1))
			v.table.SetCell(row, 2, tview.NewTableCell(fmt.Sprint(d.Replicas)).SetExpansion(1).SetAlign(tview.AlignRight))
			v.table.SetCell(row, 3, tview.NewTableCell(fmt.Sprint(d.ReadyReplicas)).SetExpansion(1).SetAlign(tview.AlignRight))
			v.table.SetCell(row, 4, tview.NewTableCell(fmt.Sprint(d.AvailableReplicas)).SetExpansion(1).SetAlign(tview.AlignRight))
		}
	case "services":
		for col, h := range []string{"Name", "Namespace", "Type", "Cluster IP", "Ports"} {
			v.table.SetCell(0, col, headerCell(h))
		}
		for i, s := range v.resp.Services {
			row := i + 1
			ports := ""
			for j, p := range s.Ports {
				if j > 0 {
					ports += ","
				}
				ports += fmt.Sprintf("%d/%s", p.Port, p.Protocol)
			}
			v.table.SetCell(row, 0, tview.NewTableCell(s.Name).SetExpansion(2))
			v.table.SetCell(row, 1, tview.NewTableCell(s.Namespace).SetExpansion(1))
			v.table.SetCell(row, 2, tview.NewTableCell(s.Type).SetExpansion(1))
			v.table.SetCell(row, 3, tview.NewTableCell(s.ClusterIP).SetExpansion(1))
			v.table.SetCell(row, 4, tview.NewTableCell(ports).SetExpansion(1))
		}
	}
}

func podPhaseColor(phase string) string {
	switch phase {
	case "Running":
		return "[green]" + phase + "[-]"
	case "Pending":
		return "[yellow]" + phase + "[-]"
	case "Failed", "Unknown":
		return "[red]" + phase + "[-]"
	default:
		return phase
	}
}
