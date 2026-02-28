package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"github.com/rivo/tview"
)

type deploymentsView struct {
	app    *App
	table  *tview.Table
	status *tview.TextView
	flex   *tview.Flex
}

func newDeploymentsView(a *App) *deploymentsView {
	v := &deploymentsView{app: a}

	v.status = tview.NewTextView().SetDynamicColors(true)

	v.table = tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetBorders(false)

	v.table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Rune() {
		case 'd':
			v.handleDelete()
			return nil
		case 'r':
			v.handleRestart()
			return nil
		case 'R':
			go v.load()
			return nil
		}
		return ev
	})

	v.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.status, 1, 0, false).
		AddItem(v.table, 0, 1, true)

	return v
}

func (v *deploymentsView) root() tview.Primitive { return v.flex }

func (v *deploymentsView) load() {
	v.app.update(func() { v.status.SetText("[yellow]Loading…[-]") })

	resp, err := v.app.client.ListDeployments(context.Background(), &adminv1.ListDeploymentsRequest{})
	if err != nil {
		v.app.update(func() { v.status.SetText(fmt.Sprintf("[red]Error: %s[-]", err)) })
		return
	}

	v.app.update(func() {
		v.table.Clear()
		headers := []string{"Account", "Name", "Namespace", "Status", "Build ID", "Deployed At"}
		for col, h := range headers {
			v.table.SetCell(0, col, headerCell(h))
		}
		for row, d := range resp.Deployments {
			cols := []string{
				d.AccountName,
				d.Name,
				d.Namespace,
				statusColor(d.Status),
				trunc(d.BuildID, 14),
				d.CreatedAt,
			}
			for col, val := range cols {
				cell := tview.NewTableCell(val).SetExpansion(1)
				if col == 3 { // status column already has color markup
					cell.SetMaxWidth(14)
				}
				v.table.SetCell(row+1, col, cell)
			}
		}
		v.status.SetText(fmt.Sprintf(
			"[gray]%d deployments — last updated %s[-]",
			len(resp.Deployments),
			time.Now().Format("15:04:05"),
		))
	})
}

func (v *deploymentsView) selectedNamespace() string {
	row, _ := v.table.GetSelection()
	if row < 1 {
		return ""
	}
	cell := v.table.GetCell(row, 2)
	if cell == nil {
		return ""
	}
	return strings.TrimSpace(cell.Text)
}

func (v *deploymentsView) handleDelete() {
	ns := v.selectedNamespace()
	if ns == "" {
		return
	}
	v.app.confirm(fmt.Sprintf("Delete deployment in namespace\n[red]%s[-]?", ns), func() {
		_, err := v.app.client.DeleteDeployment(
			context.Background(),
			&adminv1.DeleteDeploymentRequest{Namespace: ns},
		)
		if err != nil {
			v.app.showError(err.Error())
			return
		}
		go v.load()
	})
}

func (v *deploymentsView) handleRestart() {
	ns := v.selectedNamespace()
	if ns == "" {
		return
	}
	// Declare form first so the button closure can reference it.
	var form *tview.Form
	form = tview.NewForm().
		AddInputField("Pod name", "", 40, nil, nil).
		AddButton("Restart", func() {
			pod := form.GetFormItemByLabel("Pod name").(*tview.InputField).GetText()
			v.app.pages.RemovePage("restart-form")
			v.app.tv.SetFocus(v.app.pages)
			if pod == "" {
				return
			}
			go func() {
				_, err := v.app.client.RestartDeployment(
					context.Background(),
					&adminv1.RestartDeploymentRequest{Namespace: ns, Pod: pod},
				)
				if err != nil {
					v.app.showError(err.Error())
					return
				}
				go v.load()
			}()
		}).
		AddButton("Cancel", func() {
			v.app.pages.RemovePage("restart-form")
			v.app.tv.SetFocus(v.app.pages)
		})
	form.SetBorder(true).SetTitle(fmt.Sprintf(" Restart pod in %s ", ns)).SetTitleAlign(tview.AlignLeft)

	modal := centered(form, 50, 9)
	v.app.pages.AddPage("restart-form", modal, true, true)
	v.app.tv.SetFocus(form)
}

// helpers

func headerCell(text string) *tview.TableCell {
	return tview.NewTableCell(text).
		SetTextColor(tcell.ColorYellow).
		SetAttributes(tcell.AttrBold).
		SetSelectable(false).
		SetExpansion(1)
}

func statusColor(s string) string {
	switch s {
	case "active", "running":
		return "[green]" + s + "[-]"
	case "failed", "error":
		return "[red]" + s + "[-]"
	case "pending":
		return "[yellow]" + s + "[-]"
	default:
		return s
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// centered wraps a primitive in a centered flex box of fixed size.
func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(tview.NewBox(), 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(tview.NewBox(), 0, 1, false), width, 1, true).
		AddItem(tview.NewBox(), 0, 1, false)
}
