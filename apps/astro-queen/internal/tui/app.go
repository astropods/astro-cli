package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"github.com/rivo/tview"
)

const (
	tabDeployments = 0
	tabCluster     = 1
	tabImages      = 2
	tabQuery       = 3
)

var tabNames = []string{"Deployments", "Cluster", "Images", "Query"}
var pageNames = []string{"deployments", "cluster", "images", "query"}

// App is the root tview application.
type App struct {
	tv     *tview.Application
	pages  *tview.Pages
	header *tview.TextView
	footer *tview.TextView

	deployments *deploymentsView
	cluster     *clusterView
	images      *imagesView
	query       *queryView

	activeTab int
	client    adminv1.AdminServiceClient
}

// New creates and wires up the tview application.
func New(client adminv1.AdminServiceClient) *App {
	a := &App{
		tv:     tview.NewApplication(),
		pages:  tview.NewPages(),
		client: client,
	}

	a.deployments = newDeploymentsView(a)
	a.cluster = newClusterView(a)
	a.images = newImagesView(a)
	a.query = newQueryView(a)

	a.header = tview.NewTextView().
		SetDynamicColors(true).
		SetText(a.headerText(tabDeployments))

	a.footer = tview.NewTextView().
		SetDynamicColors(true).
		SetText(footerText())

	a.pages.AddPage("deployments", a.deployments.root(), true, true)
	a.pages.AddPage("cluster", a.cluster.root(), true, false)
	a.pages.AddPage("images", a.images.root(), true, false)
	a.pages.AddPage("query", a.query.root(), true, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 1, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(a.footer, 1, 0, false)

	a.tv.SetRoot(layout, true).EnableMouse(false)
	a.tv.SetInputCapture(a.globalKeys)

	go a.deployments.load()
	return a
}

// Run starts the TUI event loop.
func Run(client adminv1.AdminServiceClient) error {
	return New(client).tv.Run()
}

// globalKeys handles tab switching and quit; all other keys pass through.
func (a *App) globalKeys(ev *tcell.EventKey) *tcell.EventKey {
	// Let modals and the query input consume keys first.
	if name, _ := a.pages.GetFrontPage(); name == "confirm" || name == "error" {
		return ev
	}

	switch ev.Key() {
	case tcell.KeyTab:
		a.switchTab((a.activeTab + 1) % 4)
		return nil
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q', 'Q':
			a.tv.Stop()
			return nil
		case '1':
			a.switchTab(tabDeployments)
			return nil
		case '2':
			a.switchTab(tabCluster)
			return nil
		case '3':
			a.switchTab(tabImages)
			return nil
		case '4':
			a.switchTab(tabQuery)
			return nil
		}
	}
	return ev
}

func (a *App) switchTab(tab int) {
	a.activeTab = tab
	a.pages.SwitchToPage(pageNames[tab])
	a.header.SetText(a.headerText(tab))
	switch tab {
	case tabDeployments:
		go a.deployments.load()
	case tabCluster:
		go a.cluster.load()
	case tabImages:
		go a.images.load()
	}
}

func (a *App) headerText(active int) string {
	s := "[yellow::b]astro-queen[-:-:-]  "
	for i, name := range tabNames {
		label := fmt.Sprintf("[%d]%s", i+1, name)
		if i == active {
			s += fmt.Sprintf("[black:blue:b] %s [-:-:-] ", label)
		} else {
			s += fmt.Sprintf("[gray] %s [-] ", label)
		}
	}
	return s
}

func footerText() string {
	return "[gray]↑↓ navigate  " +
		"[white]d[-][gray] delete  " +
		"[white]r[-][gray] restart  " +
		"[white]R[-][gray] refresh  " +
		"[white]Tab[-][gray] cycle  " +
		"[white]q[-][gray] quit[-]"
}

// confirm shows a modal confirmation dialog.
func (a *App) confirm(text string, onYes func()) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			a.pages.RemovePage("confirm")
			a.tv.SetFocus(a.pages)
			if label == "Yes" {
				go onYes()
			}
		})
	a.pages.AddPage("confirm", modal, false, true)
	a.tv.SetFocus(modal)
}

// showError shows a modal error dialog.
func (a *App) showError(msg string) {
	modal := tview.NewModal().
		SetText("[red]Error:[-]\n" + msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			a.pages.RemovePage("error")
			a.tv.SetFocus(a.pages)
		})
	a.pages.AddPage("error", modal, false, true)
	a.tv.SetFocus(modal)
}

// update calls fn on the UI thread.
func (a *App) update(fn func()) {
	a.tv.QueueUpdateDraw(fn)
}
