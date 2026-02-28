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

type imagesView struct {
	app    *App
	table  *tview.Table
	status *tview.TextView
	flex   *tview.Flex
}

func newImagesView(a *App) *imagesView {
	v := &imagesView{app: a}

	v.status = tview.NewTextView().SetDynamicColors(true)

	v.table = tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetBorders(false)

	v.table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Rune() == 'R' {
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

func (v *imagesView) root() tview.Primitive { return v.flex }

func (v *imagesView) load() {
	v.app.update(func() { v.status.SetText("[yellow]Loading…[-]") })

	resp, err := v.app.client.ListImages(context.Background(), &adminv1.ListImagesRequest{})
	if err != nil {
		v.app.update(func() { v.status.SetText(fmt.Sprintf("[red]Error: %s[-]", err)) })
		return
	}

	v.app.update(func() {
		v.table.Clear()
		for col, h := range []string{"Repository", "Namespace", "Name", "Tags"} {
			v.table.SetCell(0, col, headerCell(h))
		}
		for i, img := range resp.Images {
			row := i + 1
			v.table.SetCell(row, 0, tview.NewTableCell(img.Repository).SetExpansion(2))
			v.table.SetCell(row, 1, tview.NewTableCell(img.Namespace).SetExpansion(1))
			v.table.SetCell(row, 2, tview.NewTableCell(img.Name).SetExpansion(1))
			v.table.SetCell(row, 3, tview.NewTableCell(strings.Join(img.Tags, ", ")).SetExpansion(2))
		}
		v.status.SetText(fmt.Sprintf(
			"[gray]%d images — last updated %s[-]",
			len(resp.Images),
			time.Now().Format("15:04:05"),
		))
	})
}
