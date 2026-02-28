package tui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"github.com/rivo/tview"
)

type queryView struct {
	app    *App
	input  *tview.InputField
	table  *tview.Table
	status *tview.TextView
	flex   *tview.Flex
}

func newQueryView(a *App) *queryView {
	v := &queryView{app: a}

	v.input = tview.NewInputField().
		SetLabel("SQL: ").
		SetPlaceholder("SELECT * FROM deployments LIMIT 10").
		SetFieldWidth(0)

	v.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			go v.runQuery()
		}
	})

	v.status = tview.NewTextView().SetDynamicColors(true)

	v.table = tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetBorders(false)

	v.table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Rune() == '/' || ev.Rune() == 'i' {
			a.tv.SetFocus(v.input)
			return nil
		}
		return ev
	})

	inputBox := tview.NewFrame(v.input).
		SetBorders(0, 0, 0, 0, 1, 1)

	v.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(inputBox, 3, 0, true).
		AddItem(v.status, 1, 0, false).
		AddItem(v.table, 0, 1, false)

	return v
}

func (v *queryView) root() tview.Primitive { return v.flex }

func (v *queryView) runQuery() {
	q := v.input.GetText()
	if q == "" {
		return
	}

	v.app.update(func() {
		v.status.SetText("[yellow]Running…[-]")
		v.table.Clear()
		v.app.tv.SetFocus(v.table)
	})

	resp, err := v.app.client.QueryDatabase(context.Background(), &adminv1.QueryDatabaseRequest{Query: q})
	if err != nil {
		v.app.update(func() {
			v.status.SetText(fmt.Sprintf("[red]Error: %s[-]", err))
		})
		return
	}

	v.app.update(func() {
		v.table.Clear()
		for col, h := range resp.Columns {
			v.table.SetCell(0, col, headerCell(h))
		}
		for i, row := range resp.Rows {
			for col, val := range row.Values {
				v.table.SetCell(i+1, col, tview.NewTableCell(val).SetExpansion(1))
			}
		}
		v.status.SetText(fmt.Sprintf(
			"[gray]%d rows, %d columns — [white]/ or i[-][gray] to edit query[-]",
			len(resp.Rows), len(resp.Columns),
		))
	})
}
