package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
type customerUpdatedMsg struct{ err error }

type customerDetailMsg struct {
	info customerInfo
	err  error
}

type customerAccessMsg struct {
	rows [][]string
	err  error
}

type customerAppsMsg struct {
	rows [][]string
	err  error
}

type customerEntitlementsMsg struct {
	rows [][]string
	ids  []string
	err  error
}

type entitlementValueMsg struct {
	info entitlementDetail
	err  error
}

type entitlementGrantsMsg struct {
	rows [][]string
	err  error
}

type entitlementDeletedMsg struct{ err error }
type entitlementCreatedMsg struct{ err error }
type entitlementGrantCreatedMsg struct{ err error }
type entitlementResetMsg struct{ err error }

// ─── data types ──────────────────────────────────────────────────────────────

type customerInfo struct {
	ID           string
	Name         string
	Key          string
	Description  string
	Email        string
	Currency     string
	Timezone     string
	Subjects     []string
	Subscription string
	CreatedAt    string
	UpdatedAt    string
	// Billing address
	BillingCountry    string
	BillingCity       string
	BillingLine1      string
	BillingLine2      string
	BillingState      string
	BillingPostalCode string
	BillingPhone      string
}

type entitlementDetail struct {
	HasAccess bool
	Balance   string
	Usage     string
	Overage   string
	Config    string
}

// ─── focus / section enums ──────────────────────────────────────────────────

type customerFocus int

const (
	customerFocusList customerFocus = iota
	customerFocusDetail
)

type customerSection int

const (
	sectionOverview customerSection = iota
	sectionAccess
	sectionApps
	sectionEntitlements
)

type entitlementFocus int

const (
	entFocusList entitlementFocus = iota
	entFocusDetail
)

// ─── update form field indices ──────────────────────────────────────────────

const (
	custFieldName = iota
	custFieldDescription
	custFieldKey
	custFieldEmail
	custFieldCurrency
	custFieldSubjects
	custFieldBillingCountry
	custFieldBillingCity
	custFieldBillingLine1
	custFieldBillingPostal
	custFieldCount
)

// ─── entitlement create form field indices ───────────────────────────────────

const (
	entCreateFieldFeatureKey = iota
	entCreateFieldType       // cycle selector
	entCreateFieldMetered    // for metered: issueAfterReset, isSoftLimit
	entCreateFieldStatic     // for static: JSON config
	entCreateFieldCount
)

var entitlementTypes = []string{"metered", "static", "boolean"}

// ─── model ────────────────────────────────────────────────────────────────────

type customersModel struct {
	client *openmeter.Client
	t      tableModel
	ids    []string
	status string
	width  int
	height int
	focus  customerFocus

	// Detail state
	detailID      string
	detailInfo    customerInfo
	detailLoaded  bool
	section       customerSection
	sectionLoaded [4]bool // track which sections have been loaded

	// Section data
	accessRows      [][]string
	appsRows        [][]string
	entitlementRows [][]string
	entitlementIDs  []string
	entTable        tableModel

	// Entitlement drill-down
	entFocus        entitlementFocus
	entDetailID     string
	entDetailValue  entitlementDetail
	entDetailGrants [][]string
	entDetailLoaded bool
	entGrantsLoaded bool

	// Update form fields
	updateFields  [10]textinput.Model
	updateFocused int
	updateStatus  string

	// Entitlement create form
	entCreateFields  [4]textinput.Model
	entCreateFocused int
	entCreateTypeIdx int
	entCreateStatus  string

	// Grant create form
	grantFields  [4]textinput.Model
	grantFocused int
	grantStatus  string
}

func newCustomersModel(client *openmeter.Client) *customersModel {
	t := newTableModel([]string{"ID", "Name", "Key", "Email", "Currency", "Created"})
	t.SetFocused(true)

	entT := newTableModel([]string{"ID", "Type", "Feature Key", "Created"})

	mkInput := func(placeholder string, charLimit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		ti.Prompt = ""
		return ti
	}

	return &customersModel{
		client:   client,
		t:        t,
		entTable: entT,
		status:   statusWIP.Render("Loading…"),
		updateFields: [10]textinput.Model{
			mkInput("Acme Corp", 256),
			mkInput("Main customer account", 1024),
			mkInput("acme-corp", 256),
			mkInput("billing@acme.com", 256),
			mkInput("USD", 10),
			mkInput("subject-1, subject-2", 500),
			mkInput("US", 64),
			mkInput("San Francisco", 128),
			mkInput("123 Main St", 256),
			mkInput("94102", 20),
		},
		entCreateFields: [4]textinput.Model{
			mkInput("api_requests", 256),
			mkInput("", 0), // cycle selector placeholder
			mkInput("true, false", 64),
			mkInput(`{"maxAmount": 1000}`, 1024),
		},
		grantFields: [4]textinput.Model{
			mkInput("100", 20),
			mkInput("1", 10),
			mkInput("2024-01-01T00:00:00Z", 30),
			mkInput("2025-01-01T00:00:00Z", 30),
		},
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
		}
		m.t.SetRows(msg.rows)
		m.ids = msg.ids
		m.status = statusOK.Render(fmt.Sprintf(
			"%d customers  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
		))
		return m, nil

	case customerDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.focus = customerFocusList
		return m, m.load()

	case customerUpdatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, tea.Batch(m.load(), m.loadDetail(m.detailID))

	case customerDetailMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.detailInfo = msg.info
		m.detailLoaded = true
		return m, nil

	case customerAccessMsg:
		m.sectionLoaded[sectionAccess] = true
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.accessRows = msg.rows
		return m, nil

	case customerAppsMsg:
		m.sectionLoaded[sectionApps] = true
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.appsRows = msg.rows
		return m, nil

	case customerEntitlementsMsg:
		m.sectionLoaded[sectionEntitlements] = true
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.entitlementRows = msg.rows
		m.entitlementIDs = msg.ids
		m.entTable.SetRows(msg.rows)
		return m, nil

	case entitlementValueMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.entDetailValue = msg.info
		m.entDetailLoaded = true
		return m, nil

	case entitlementGrantsMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.entDetailGrants = msg.rows
		m.entGrantsLoaded = true
		return m, nil

	case entitlementDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.sectionLoaded[sectionEntitlements] = false
		return m, m.loadEntitlements(m.detailID)

	case entitlementCreatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.sectionLoaded[sectionEntitlements] = false
		return m, m.loadEntitlements(m.detailID)

	case entitlementGrantCreatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, tea.Batch(
			m.loadEntitlementValue(m.detailID, m.entDetailID),
			m.loadEntitlementGrants(m.detailID, m.entDetailID),
		)

	case entitlementResetMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.loadEntitlementValue(m.detailID, m.entDetailID)

	case tea.KeyMsg:
		switch m.focus {
		case customerFocusList:
			return m.updateList(msg)
		case customerFocusDetail:
			return m.updateDetail(msg)
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

// ─── list view ──────────────────────────────────────────────────────────────

func (m *customersModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	if m.t.filtering {
		cmd := m.t.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "R":
		m.status = statusWIP.Render("Refreshing…")
		return m, m.load()

	case "enter":
		idx := m.t.selectedRealIndex()
		if idx < 0 || idx >= len(m.ids) {
			return m, nil
		}
		id := m.ids[idx]
		m.focus = customerFocusDetail
		m.detailID = id
		m.detailLoaded = false
		m.section = sectionOverview
		m.sectionLoaded = [4]bool{true, false, false, false} // overview loaded via detail
		m.entFocus = entFocusList
		m.accessRows = nil
		m.appsRows = nil
		m.entitlementRows = nil
		m.entitlementIDs = nil
		return m, m.loadDetail(id)

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

	cmd := m.t.Update(msg)
	return m, cmd
}

// ─── detail view ────────────────────────────────────────────────────────────

func (m *customersModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	key := msg.String()

	// Section hotkeys always available in detail
	switch key {
	case "backspace":
		if m.section == sectionEntitlements && m.entFocus == entFocusDetail {
			m.entFocus = entFocusList
			m.entTable.SetFocused(true)
			return m, nil
		}
		m.focus = customerFocusList
		return m, nil
	case "o":
		m.section = sectionOverview
		return m, nil
	case "a":
		m.section = sectionAccess
		if !m.sectionLoaded[sectionAccess] {
			return m, m.loadAccess(m.detailID)
		}
		return m, nil
	case "p":
		m.section = sectionApps
		if !m.sectionLoaded[sectionApps] {
			return m, m.loadApps(m.detailID)
		}
		return m, nil
	case "t":
		m.section = sectionEntitlements
		m.entFocus = entFocusList
		m.entTable.SetFocused(true)
		if !m.sectionLoaded[sectionEntitlements] {
			return m, m.loadEntitlements(m.detailID)
		}
		return m, nil
	}

	// Section-specific keys
	switch m.section {
	case sectionOverview:
		if key == "e" {
			return m, m.openUpdateForm()
		}
		if key == "d" {
			return m, func() tea.Msg {
				id := m.detailID
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

	case sectionEntitlements:
		return m.updateEntitlements(msg)
	}

	return m, nil
}

func (m *customersModel) updateEntitlements(msg tea.KeyMsg) (Tab, tea.Cmd) {
	key := msg.String()

	if m.entFocus == entFocusDetail {
		switch key {
		case "g":
			return m, m.openGrantForm()
		case "r":
			custID := m.detailID
			entID := m.entDetailID
			return m, func() tea.Msg {
				return showConfirmMsg{
					text: "Reset entitlement usage?",
					fn: func() tea.Cmd {
						return func() tea.Msg {
							_, err := m.client.ResetEntitlement(custID, entID, json.RawMessage(`{}`))
							return entitlementResetMsg{err}
						}
					},
				}
			}
		}
		return m, nil
	}

	// entFocusList
	if m.entTable.filtering {
		cmd := m.entTable.Update(msg)
		return m, cmd
	}

	switch key {
	case "enter":
		idx := m.entTable.selectedRealIndex()
		if idx < 0 || idx >= len(m.entitlementIDs) {
			return m, nil
		}
		entID := m.entitlementIDs[idx]
		m.entFocus = entFocusDetail
		m.entDetailID = entID
		m.entDetailLoaded = false
		m.entGrantsLoaded = false
		m.entDetailGrants = nil
		m.entTable.SetFocused(false)
		return m, tea.Batch(
			m.loadEntitlementValue(m.detailID, entID),
			m.loadEntitlementGrants(m.detailID, entID),
		)
	case "c":
		return m, m.openEntitlementCreateForm()
	case "d":
		idx := m.entTable.selectedRealIndex()
		if idx < 0 || idx >= len(m.entitlementIDs) {
			return m, nil
		}
		entID := m.entitlementIDs[idx]
		custID := m.detailID
		return m, func() tea.Msg {
			return showConfirmMsg{
				text: fmt.Sprintf("Delete entitlement %s?", statusWIP.Render(entID)),
				fn: func() tea.Cmd {
					return func() tea.Msg {
						err := m.client.DeleteCustomerEntitlement(custID, entID)
						return entitlementDeletedMsg{err}
					}
				},
			}
		}
	}

	cmd := m.entTable.Update(msg)
	return m, cmd
}

// ─── View ───────────────────────────────────────────────────────────────────

func (m *customersModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.focus {
	case customerFocusDetail:
		return m.viewDetail()
	default:
		tip := tipStyle.Render("Customers map to billable entities. Enter to view details, d to delete, R to refresh.")
		return tip + "\n" + m.t.View()
	}
}

func (m *customersModel) viewDetail() string {
	if !m.detailLoaded {
		return statusWIP.Render("Loading customer…")
	}

	var b strings.Builder
	info := m.detailInfo
	kvStyle := lipgloss.NewStyle().Foreground(colDimmed)
	valStyle := lipgloss.NewStyle().Foreground(colFg)

	// Header
	b.WriteString(titleStyle.Render(info.Name))
	if info.Key != "" {
		b.WriteString("  " + kvStyle.Render("key: ") + valStyle.Render(info.Key))
	}
	b.WriteString("\n")

	// Summary line
	var parts []string
	if info.Email != "" {
		parts = append(parts, kvStyle.Render("email: ")+valStyle.Render(info.Email))
	}
	if info.Currency != "" {
		parts = append(parts, kvStyle.Render("currency: ")+valStyle.Render(info.Currency))
	}
	if info.Subscription != "" {
		parts = append(parts, kvStyle.Render("subscription: ")+valStyle.Render(info.Subscription))
	}
	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, "  ") + "\n")
	}

	var parts2 []string
	if info.CreatedAt != "" {
		parts2 = append(parts2, kvStyle.Render("created: ")+valStyle.Render(info.CreatedAt))
	}
	if info.UpdatedAt != "" {
		parts2 = append(parts2, kvStyle.Render("updated: ")+valStyle.Render(info.UpdatedAt))
	}
	if len(parts2) > 0 {
		b.WriteString(strings.Join(parts2, "  ") + "\n")
	}
	if len(info.Subjects) > 0 {
		b.WriteString(kvStyle.Render("usage: ") + valStyle.Render(strings.Join(info.Subjects, ", ")) + "\n")
	}
	if info.BillingLine1 != "" || info.BillingCity != "" || info.BillingCountry != "" {
		addrParts := []string{}
		if info.BillingLine1 != "" {
			addrParts = append(addrParts, info.BillingLine1)
		}
		if info.BillingCity != "" {
			addrParts = append(addrParts, info.BillingCity)
		}
		if info.BillingState != "" {
			addrParts = append(addrParts, info.BillingState)
		}
		if info.BillingCountry != "" {
			addrParts = append(addrParts, info.BillingCountry)
		}
		b.WriteString(kvStyle.Render("billing: ") + valStyle.Render(strings.Join(addrParts, ", ")) + "\n")
	}

	// Section tabs
	b.WriteString(formSeparator(m.width) + "\n")
	sectionNames := []struct {
		key     string
		label   string
		section customerSection
	}{
		{"o", "Overview", sectionOverview},
		{"a", "Access", sectionAccess},
		{"p", "Apps", sectionApps},
		{"t", "Entitlements", sectionEntitlements},
	}
	var tabs []string
	for _, s := range sectionNames {
		label := fmt.Sprintf("[%s] %s", s.key, s.label)
		if m.section == s.section {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	b.WriteString(strings.Join(tabs, " ") + "\n")
	b.WriteString(formSeparator(m.width) + "\n")

	// Section content
	switch m.section {
	case sectionOverview:
		b.WriteString(m.viewOverviewSection())
	case sectionAccess:
		b.WriteString(m.viewAccessSection())
	case sectionApps:
		b.WriteString(m.viewAppsSection())
	case sectionEntitlements:
		b.WriteString(m.viewEntitlementsSection())
	}

	return b.String()
}

func (m *customersModel) viewOverviewSection() string {
	info := m.detailInfo
	kvStyle := lipgloss.NewStyle().Foreground(colDimmed)
	valStyle := lipgloss.NewStyle().Foreground(colFg)

	var b strings.Builder
	b.WriteString(kvStyle.Render("ID:          ") + valStyle.Render(info.ID) + "\n")
	b.WriteString(kvStyle.Render("Name:        ") + valStyle.Render(info.Name) + "\n")
	if info.Key != "" {
		b.WriteString(kvStyle.Render("Key:         ") + valStyle.Render(info.Key) + "\n")
	}
	if info.Description != "" {
		b.WriteString(kvStyle.Render("Description: ") + valStyle.Render(info.Description) + "\n")
	}
	if info.Email != "" {
		b.WriteString(kvStyle.Render("Email:       ") + valStyle.Render(info.Email) + "\n")
	}
	if info.Currency != "" {
		b.WriteString(kvStyle.Render("Currency:    ") + valStyle.Render(info.Currency) + "\n")
	}
	if info.Timezone != "" {
		b.WriteString(kvStyle.Render("Timezone:    ") + valStyle.Render(info.Timezone) + "\n")
	}
	b.WriteString("\n" + descStyle.Render("Press e to edit, d to delete"))
	return b.String()
}

func (m *customersModel) viewAccessSection() string {
	if !m.sectionLoaded[sectionAccess] {
		return statusWIP.Render("Loading access…")
	}
	if len(m.accessRows) == 0 {
		return descStyle.Render("No entitlement access data")
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(fitCell("Feature Key", 25)+"  "+fitCell("Has Access", 12)+"  "+fitCell("Balance", 12)+"  "+fitCell("Usage", 12)+"  "+fitCell("Overage", 12)) + "\n")
	for i, row := range m.accessRows {
		style := lipgloss.NewStyle().Foreground(colFg)
		if i%2 == 0 {
			style = dimStyle
		}
		line := ""
		for ci, w := range []int{25, 12, 12, 12, 12} {
			if ci > 0 {
				line += "  "
			}
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			line += style.Render(fitCell(val, w))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *customersModel) viewAppsSection() string {
	if !m.sectionLoaded[sectionApps] {
		return statusWIP.Render("Loading apps…")
	}
	if len(m.appsRows) == 0 {
		return descStyle.Render("No app data")
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(fitCell("Type", 15)+"  "+fitCell("App ID", 25)+"  "+fitCell("Details", 40)) + "\n")
	for i, row := range m.appsRows {
		style := lipgloss.NewStyle().Foreground(colFg)
		if i%2 == 0 {
			style = dimStyle
		}
		line := ""
		for ci, w := range []int{15, 25, 40} {
			if ci > 0 {
				line += "  "
			}
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			line += style.Render(fitCell(val, w))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *customersModel) viewEntitlementsSection() string {
	if !m.sectionLoaded[sectionEntitlements] {
		return statusWIP.Render("Loading entitlements…")
	}

	if m.entFocus == entFocusDetail {
		return m.viewEntitlementDetail()
	}

	if len(m.entitlementRows) == 0 {
		return descStyle.Render("No entitlements. Press c to create one.")
	}

	m.entTable.SetSize(m.width, m.height-10)
	return m.entTable.View() + "\n" + descStyle.Render("Enter to inspect, c to create, d to delete")
}

func (m *customersModel) viewEntitlementDetail() string {
	var b strings.Builder
	kvStyle := lipgloss.NewStyle().Foreground(colDimmed)
	valStyle := lipgloss.NewStyle().Foreground(colFg)

	b.WriteString(titleStyle.Render("Entitlement: "+m.entDetailID) + "\n\n")

	if !m.entDetailLoaded {
		b.WriteString(statusWIP.Render("Loading value…") + "\n")
	} else {
		v := m.entDetailValue
		if v.HasAccess {
			b.WriteString(kvStyle.Render("Has Access: ") + statusGood.Render("yes") + "\n")
		} else {
			b.WriteString(kvStyle.Render("Has Access: ") + statusErr.Render("no") + "\n")
		}
		if v.Balance != "" {
			b.WriteString(kvStyle.Render("Balance:    ") + valStyle.Render(v.Balance) + "\n")
		}
		if v.Usage != "" {
			b.WriteString(kvStyle.Render("Usage:      ") + valStyle.Render(v.Usage) + "\n")
		}
		if v.Overage != "" {
			b.WriteString(kvStyle.Render("Overage:    ") + valStyle.Render(v.Overage) + "\n")
		}
		if v.Config != "" {
			b.WriteString(kvStyle.Render("Config:     ") + valStyle.Render(v.Config) + "\n")
		}
	}

	b.WriteString("\n")

	// Grants table
	b.WriteString(headerStyle.Render("Grants") + "\n")
	if !m.entGrantsLoaded {
		b.WriteString(statusWIP.Render("Loading grants…") + "\n")
	} else if len(m.entDetailGrants) == 0 {
		b.WriteString(descStyle.Render("No grants") + "\n")
	} else {
		b.WriteString(headerStyle.Render(
			fitCell("ID", 25)+"  "+fitCell("Amount", 10)+"  "+fitCell("Priority", 10)+"  "+fitCell("Effective At", 22)+"  "+fitCell("Expires At", 22),
		) + "\n")
		for i, row := range m.entDetailGrants {
			style := lipgloss.NewStyle().Foreground(colFg)
			if i%2 == 0 {
				style = dimStyle
			}
			line := ""
			for ci, w := range []int{25, 10, 10, 22, 22} {
				if ci > 0 {
					line += "  "
				}
				val := ""
				if ci < len(row) {
					val = row[ci]
				}
				line += style.Render(fitCell(val, w))
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n" + descStyle.Render("g create grant • r reset usage • Bksp back"))
	return b.String()
}

func (m *customersModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *customersModel) Status() string {
	switch m.focus {
	case customerFocusDetail:
		sections := []string{"overview", "access", "apps", "entitlements"}
		return statusOK.Render("customer: " + m.detailID + " [" + sections[m.section] + "]")
	default:
		return m.status
	}
}

func (m *customersModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case customerFocusDetail:
		base := []KeyHint{
			{"o/a/p/t", "sections"},
			{"Bksp", "back"},
		}
		switch m.section {
		case sectionOverview:
			base = append(base, KeyHint{"e", "edit"}, KeyHint{"d", "delete"})
		case sectionEntitlements:
			if m.entFocus == entFocusDetail {
				base = append(base, KeyHint{"g", "grant"}, KeyHint{"r", "reset"})
			} else {
				base = append(base, KeyHint{"Enter", "inspect"}, KeyHint{"c", "create"}, KeyHint{"d", "delete"})
			}
		}
		return base
	default:
		return []KeyHint{
			{"↑↓/jk", "navigate"},
			{"/", "search"},
			{"Enter", "detail"},
			{"d", "delete"},
			{"R", "refresh"},
			{"Esc", "nav mode"},
		}
	}
}

// ─── update form ────────────────────────────────────────────────────────────

func (m *customersModel) openUpdateForm() tea.Cmd {
	m.updateFocused = custFieldName
	m.updateStatus = ""
	for i := range m.updateFields {
		m.updateFields[i].Reset()
		m.updateFields[i].Blur()
	}
	// Pre-fill
	if m.detailLoaded {
		info := m.detailInfo
		m.updateFields[0].SetValue(info.Name)
		m.updateFields[1].SetValue(info.Description)
		m.updateFields[2].SetValue(info.Key)
		m.updateFields[3].SetValue(info.Email)
		m.updateFields[4].SetValue(info.Currency)
		m.updateFields[5].SetValue(strings.Join(info.Subjects, ", "))
		m.updateFields[6].SetValue(info.BillingCountry)
		m.updateFields[7].SetValue(info.BillingCity)
		m.updateFields[8].SetValue(info.BillingLine1)
		m.updateFields[9].SetValue(info.BillingPostalCode)
	}
	m.updateFields[0].Focus()
	return func() tea.Msg {
		return showFormMsg{
			view:   m.viewUpdateForm,
			update: m.updateUpdateForm,
		}
	}
}

func (m *customersModel) updateUpdateForm(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	key := keyMsg.String()
	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		next := (m.updateFocused + 1) % custFieldCount
		m.updateFields[m.updateFocused].Blur()
		m.updateFocused = next
		m.updateFields[next].Focus()
		return false, textinput.Blink
	case "shift+tab", "up":
		next := (m.updateFocused + custFieldCount - 1) % custFieldCount
		m.updateFields[m.updateFocused].Blur()
		m.updateFocused = next
		m.updateFields[next].Focus()
		return false, textinput.Blink
	case "enter":
		cmd := m.submitUpdate()
		if cmd == nil {
			return false, nil
		}
		return true, cmd
	}

	var cmd tea.Cmd
	m.updateFields[m.updateFocused], cmd = m.updateFields[m.updateFocused].Update(keyMsg)
	return false, cmd
}

func (m *customersModel) submitUpdate() tea.Cmd {
	name := m.updateFields[0].Value()
	desc := m.updateFields[1].Value()
	key := m.updateFields[2].Value()
	email := m.updateFields[3].Value()
	currency := m.updateFields[4].Value()
	subjects := m.updateFields[5].Value()
	billingCountry := m.updateFields[6].Value()
	billingCity := m.updateFields[7].Value()
	billingLine1 := m.updateFields[8].Value()
	billingPostal := m.updateFields[9].Value()

	if name == "" {
		m.updateStatus = statusErr.Render("Name is required")
		return nil
	}

	body := map[string]any{
		"name": name,
	}
	if desc != "" {
		body["description"] = desc
	}
	if key != "" {
		body["key"] = key
	}
	if email != "" {
		body["primaryEmail"] = email
	}
	if currency != "" {
		body["currency"] = currency
	}
	if subjects != "" {
		var subjectKeys []string
		for _, s := range strings.Split(subjects, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				subjectKeys = append(subjectKeys, s)
			}
		}
		if len(subjectKeys) > 0 {
			body["usageAttribution"] = map[string]any{"subjectKeys": subjectKeys}
		}
	}

	hasBilling := billingCountry != "" || billingCity != "" || billingLine1 != "" || billingPostal != ""
	if hasBilling {
		addr := map[string]any{}
		if billingCountry != "" {
			addr["country"] = billingCountry
		}
		if billingCity != "" {
			addr["city"] = billingCity
		}
		if billingLine1 != "" {
			addr["line1"] = billingLine1
		}
		if billingPostal != "" {
			addr["postalCode"] = billingPostal
		}
		body["billingAddress"] = addr
	}

	id := m.detailID
	m.updateStatus = statusWIP.Render("Updating…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.UpdateCustomer(id, raw)
		return customerUpdatedMsg{err}
	}
}

func (m *customersModel) custUpdateFieldHelp() (string, string) {
	switch m.updateFocused {
	case custFieldName:
		return "Name (required)", "Display name for the customer.\nUsed in UI and billing."
	case custFieldDescription:
		return "Description", "Optional description.\nFor internal notes."
	case custFieldKey:
		return "Key", "Unique key identifier.\nAlphanumeric + hyphens."
	case custFieldEmail:
		return "Primary Email", "Primary billing email address."
	case custFieldCurrency:
		return "Currency", "Three-letter ISO currency code.\n\n  Example: USD, EUR, GBP"
	case custFieldSubjects:
		return "Usage Attribution", "Comma-separated subject keys\nfor usage tracking.\n\n  Example: subject-1, subject-2"
	case custFieldBillingCountry:
		return "Billing Country", "Two-letter country code.\n\n  Example: US, GB, DE"
	case custFieldBillingCity:
		return "Billing City", "City name for billing address."
	case custFieldBillingLine1:
		return "Billing Address", "Street address line 1."
	case custFieldBillingPostal:
		return "Billing Postal Code", "Postal / ZIP code."
	}
	return "", ""
}

func (m *customersModel) viewUpdateForm(width int) string {
	return m.viewTwoPaneForm(width, "Update Customer: "+m.detailInfo.Name, m.updateFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Name:", custFieldName, true},
			{"Description:", custFieldDescription, false},
			{"Key:", custFieldKey, false},
			{"Email:", custFieldEmail, false},
			{"Currency:", custFieldCurrency, false},
			{"Subjects:", custFieldSubjects, false},
			{"Country:", custFieldBillingCountry, false},
			{"City:", custFieldBillingCity, false},
			{"Address:", custFieldBillingLine1, false},
			{"Postal:", custFieldBillingPostal, false},
		}
		title, body := m.custUpdateFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		m.updateFields[r.logical].Width = inputW
		return m.updateFields[r.logical].View()
	}, m.updateStatus)
}

// ─── entitlement create form ────────────────────────────────────────────────

func (m *customersModel) openEntitlementCreateForm() tea.Cmd {
	m.entCreateFocused = entCreateFieldFeatureKey
	m.entCreateTypeIdx = 0
	m.entCreateStatus = ""
	for i := range m.entCreateFields {
		m.entCreateFields[i].Reset()
		m.entCreateFields[i].Blur()
	}
	m.entCreateFields[0].Focus()
	return func() tea.Msg {
		return showFormMsg{
			view:   m.viewEntCreateForm,
			update: m.updateEntCreateForm,
		}
	}
}

func (m *customersModel) updateEntCreateForm(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	key := keyMsg.String()
	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		return false, m.entCreateMoveFocus((m.entCreateFocused + 1) % entCreateFieldCount)
	case "shift+tab", "up":
		return false, m.entCreateMoveFocus((m.entCreateFocused + entCreateFieldCount - 1) % entCreateFieldCount)
	case "enter":
		cmd := m.submitEntitlementCreate()
		if cmd == nil {
			return false, nil
		}
		return true, cmd
	}

	if m.entCreateFocused == entCreateFieldType {
		switch key {
		case "left", "h":
			m.entCreateTypeIdx = (m.entCreateTypeIdx + len(entitlementTypes) - 1) % len(entitlementTypes)
			return false, nil
		case "right", "l":
			m.entCreateTypeIdx = (m.entCreateTypeIdx + 1) % len(entitlementTypes)
			return false, nil
		}
		return false, nil
	}

	ti := m.entCreateTextFieldIdx(m.entCreateFocused)
	if ti >= 0 {
		var cmd tea.Cmd
		m.entCreateFields[ti], cmd = m.entCreateFields[ti].Update(keyMsg)
		return false, cmd
	}
	return false, nil
}

func (m *customersModel) entCreateTextFieldIdx(logical int) int {
	switch logical {
	case entCreateFieldFeatureKey:
		return 0
	case entCreateFieldMetered:
		return 2
	case entCreateFieldStatic:
		return 3
	default:
		return -1
	}
}

func (m *customersModel) entCreateMoveFocus(next int) tea.Cmd {
	if ti := m.entCreateTextFieldIdx(m.entCreateFocused); ti >= 0 {
		m.entCreateFields[ti].Blur()
	}
	m.entCreateFocused = next
	if ti := m.entCreateTextFieldIdx(next); ti >= 0 {
		m.entCreateFields[ti].Focus()
		return textinput.Blink
	}
	return nil
}

func (m *customersModel) submitEntitlementCreate() tea.Cmd {
	featureKey := m.entCreateFields[0].Value()
	entType := entitlementTypes[m.entCreateTypeIdx]

	if featureKey == "" {
		m.entCreateStatus = statusErr.Render("Feature key is required")
		return nil
	}

	body := map[string]any{
		"type":       entType,
		"featureKey": featureKey,
	}

	switch entType {
	case "metered":
		meteredVal := m.entCreateFields[2].Value()
		if meteredVal != "" {
			// Parse as issueAfterReset, isSoftLimit
			parts := strings.SplitN(meteredVal, ",", 2)
			if len(parts) >= 1 {
				v := strings.TrimSpace(parts[0])
				if v == "true" {
					body["issueAfterReset"] = true
				}
			}
			if len(parts) >= 2 {
				v := strings.TrimSpace(parts[1])
				if v == "true" {
					body["isSoftLimit"] = true
				}
			}
		}
	case "static":
		configVal := m.entCreateFields[3].Value()
		if configVal != "" {
			var parsed json.RawMessage
			if json.Unmarshal([]byte(configVal), &parsed) == nil {
				body["config"] = parsed
			}
		}
	}

	custID := m.detailID
	m.entCreateStatus = statusWIP.Render("Creating…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.CreateCustomerEntitlement(custID, raw)
		return entitlementCreatedMsg{err}
	}
}

func (m *customersModel) entCreateFieldHelp() (string, string) {
	switch m.entCreateFocused {
	case entCreateFieldFeatureKey:
		return "Feature Key (required)", "The feature key to entitle.\nMust match an existing feature."
	case entCreateFieldType:
		t := entitlementTypes[m.entCreateTypeIdx]
		switch t {
		case "metered":
			return "Type: metered", "Tracks usage against a balance.\nGrants provide the balance.\nUsage resets periodically."
		case "static":
			return "Type: static", "Static JSON config entitlement.\nProvides fixed configuration data."
		case "boolean":
			return "Type: boolean", "Simple on/off access.\nNo balance or usage tracking."
		}
	case entCreateFieldMetered:
		return "Metered Options", "issueAfterReset, isSoftLimit\nComma-separated booleans.\n\n  Example: true, false"
	case entCreateFieldStatic:
		return "Static Config", "JSON config for static entitlement.\n\n  Example: {\"maxAmount\": 1000}"
	}
	return "", ""
}

func (m *customersModel) viewEntCreateForm(width int) string {
	return m.viewTwoPaneForm(width, "Create Entitlement", m.entCreateFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Feature Key:", entCreateFieldFeatureKey, true},
			{"Type:", entCreateFieldType, true},
		}
		t := entitlementTypes[m.entCreateTypeIdx]
		switch t {
		case "metered":
			rows = append(rows, formRow{"Options:", entCreateFieldMetered, false})
		case "static":
			rows = append(rows, formRow{"Config:", entCreateFieldStatic, false})
		}
		title, body := m.entCreateFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		if r.logical == entCreateFieldType {
			return renderEntTypeSelector(m.entCreateTypeIdx)
		}
		ti := m.entCreateTextFieldIdx(r.logical)
		if ti >= 0 {
			m.entCreateFields[ti].Width = inputW
			return m.entCreateFields[ti].View()
		}
		return ""
	}, m.entCreateStatus)
}

func renderEntTypeSelector(activeIdx int) string {
	active := lipgloss.NewStyle().Background(colAccent).Foreground(colBg).Padding(0, 1)
	inactive := lipgloss.NewStyle().Foreground(colMuted)
	var opts []string
	for i, t := range entitlementTypes {
		if i == activeIdx {
			opts = append(opts, active.Render(t))
		} else {
			opts = append(opts, inactive.Render(t))
		}
	}
	return strings.Join(opts, " ")
}

// ─── grant create form ──────────────────────────────────────────────────────

func (m *customersModel) openGrantForm() tea.Cmd {
	m.grantFocused = 0
	m.grantStatus = ""
	for i := range m.grantFields {
		m.grantFields[i].Reset()
		m.grantFields[i].Blur()
	}
	m.grantFields[0].Focus()
	return func() tea.Msg {
		return showFormMsg{
			view:   m.viewGrantForm,
			update: m.updateGrantForm,
		}
	}
}

func (m *customersModel) updateGrantForm(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	key := keyMsg.String()
	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		next := (m.grantFocused + 1) % 4
		m.grantFields[m.grantFocused].Blur()
		m.grantFocused = next
		m.grantFields[next].Focus()
		return false, textinput.Blink
	case "shift+tab", "up":
		next := (m.grantFocused + 3) % 4
		m.grantFields[m.grantFocused].Blur()
		m.grantFocused = next
		m.grantFields[next].Focus()
		return false, textinput.Blink
	case "enter":
		cmd := m.submitGrant()
		if cmd == nil {
			return false, nil
		}
		return true, cmd
	}

	var cmd tea.Cmd
	m.grantFields[m.grantFocused], cmd = m.grantFields[m.grantFocused].Update(keyMsg)
	return false, cmd
}

func (m *customersModel) submitGrant() tea.Cmd {
	amount := m.grantFields[0].Value()
	priority := m.grantFields[1].Value()
	effectiveAt := m.grantFields[2].Value()
	expiresAt := m.grantFields[3].Value()

	if amount == "" {
		m.grantStatus = statusErr.Render("Amount is required")
		return nil
	}

	body := map[string]any{
		"amount": json.Number(amount),
	}
	if priority != "" {
		body["priority"] = json.Number(priority)
	}
	if effectiveAt != "" {
		body["effectiveAt"] = effectiveAt
	}
	if expiresAt != "" {
		body["expiration"] = map[string]any{"expiresAt": expiresAt}
	}

	custID := m.detailID
	entID := m.entDetailID
	m.grantStatus = statusWIP.Render("Creating grant…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.CreateEntitlementGrant(custID, entID, raw)
		return entitlementGrantCreatedMsg{err}
	}
}

func (m *customersModel) grantFieldHelp() (string, string) {
	switch m.grantFocused {
	case 0:
		return "Amount (required)", "Number of units to grant.\n\n  Example: 100"
	case 1:
		return "Priority", "Lower = used first. Default: 1.\n\n  Example: 1"
	case 2:
		return "Effective At", "When the grant becomes active.\nRFC 3339 format. Optional.\n\n  Example: 2024-01-01T00:00:00Z"
	case 3:
		return "Expires At", "When the grant expires.\nRFC 3339 format. Optional.\n\n  Example: 2025-01-01T00:00:00Z"
	}
	return "", ""
}

func (m *customersModel) viewGrantForm(width int) string {
	return m.viewTwoPaneForm(width, "Create Grant", m.grantFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Amount:", 0, true},
			{"Priority:", 1, false},
			{"Effective:", 2, false},
			{"Expires:", 3, false},
		}
		title, body := m.grantFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		m.grantFields[r.logical].Width = inputW
		return m.grantFields[r.logical].View()
	}, m.grantStatus)
}

// ─── two-pane form (duplicated from meters.go pattern) ──────────────────────

func (m *customersModel) viewTwoPaneForm(
	width int,
	title string,
	focused int,
	buildRows func() ([]formRow, string),
	renderField func(formRow, int) string,
	status string,
) string {
	rightW := 40
	borderW := 1
	leftW := width - rightW - borderW - 2
	if leftW < 35 {
		leftW = 35
		rightW = width - leftW - borderW - 2
	}

	requiredLabel := labelStyle.Bold(true).Width(16)
	optionalLabel := labelStyle.Width(16)

	inputW := leftW - 20
	if inputW < 10 {
		inputW = 10
	}

	rows, helpContent := buildRows()

	var leftLines []string
	leftLines = append(leftLines, titleStyle.Render(title))
	leftLines = append(leftLines, "")

	for _, r := range rows {
		marker := "  "
		if focused == r.logical {
			marker = focusStyle.Render("▸ ")
		}
		lbl := optionalLabel
		if r.required {
			lbl = requiredLabel
		}
		leftLines = append(leftLines, marker+lbl.Render(r.label)+" "+renderField(r, inputW))
	}

	leftLines = append(leftLines, "")
	if status != "" {
		leftLines = append(leftLines, status)
	}
	leftLines = append(leftLines, descStyle.Render("↑↓/Tab fields • ←→ selector • Enter submit • Esc cancel"))

	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		PaddingRight(1).
		Render(strings.Join(leftLines, "\n"))

	rightPane := lipgloss.NewStyle().
		Width(rightW).
		PaddingLeft(1).
		Render(helpContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
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
				ID        string `json:"id"`
				Name      string `json:"name"`
				Key       string `json:"key"`
				Email     string `json:"primaryEmail"`
				Currency  string `json:"currency"`
				CreatedAt string `json:"createdAt"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return customersLoadedMsg{err: fmt.Errorf("parse: %w", err), at: time.Now()}
		}

		rows := make([][]string, len(resp.Items))
		ids := make([]string, len(resp.Items))
		for i, c := range resp.Items {
			rows[i] = []string{c.ID, c.Name, c.Key, c.Email, c.Currency, c.CreatedAt}
			ids[i] = c.ID
		}
		return customersLoadedMsg{rows: rows, ids: ids, at: time.Now()}
	}
}

func (m *customersModel) loadDetail(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.GetCustomer(id)
		if err != nil {
			return customerDetailMsg{err: err}
		}
		var c struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			Key              string `json:"key"`
			Description      string `json:"description"`
			Email            string `json:"primaryEmail"`
			Currency         string `json:"currency"`
			Timezone         string `json:"timezone"`
			CreatedAt        string `json:"createdAt"`
			UpdatedAt        string `json:"updatedAt"`
			UsageAttribution struct {
				SubjectKeys []string `json:"subjectKeys"`
			} `json:"usageAttribution"`
			CurrentSubscriptionID string `json:"currentSubscriptionId"`
			BillingAddress        struct {
				Country    string `json:"country"`
				City       string `json:"city"`
				Line1      string `json:"line1"`
				Line2      string `json:"line2"`
				State      string `json:"state"`
				PostalCode string `json:"postalCode"`
				Phone      string `json:"phone"`
			} `json:"billingAddress"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return customerDetailMsg{err: fmt.Errorf("parse: %w", err)}
		}
		return customerDetailMsg{info: customerInfo{
			ID:                c.ID,
			Name:              c.Name,
			Key:               c.Key,
			Description:       c.Description,
			Email:             c.Email,
			Currency:          c.Currency,
			Timezone:          c.Timezone,
			Subjects:          c.UsageAttribution.SubjectKeys,
			Subscription:      c.CurrentSubscriptionID,
			CreatedAt:         c.CreatedAt,
			UpdatedAt:         c.UpdatedAt,
			BillingCountry:    c.BillingAddress.Country,
			BillingCity:       c.BillingAddress.City,
			BillingLine1:      c.BillingAddress.Line1,
			BillingLine2:      c.BillingAddress.Line2,
			BillingState:      c.BillingAddress.State,
			BillingPostalCode: c.BillingAddress.PostalCode,
			BillingPhone:      c.BillingAddress.Phone,
		}}
	}
}

func (m *customersModel) loadAccess(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.GetCustomerAccess(id)
		if err != nil {
			return customerAccessMsg{err: err}
		}
		// Parse access: array of entitlement access objects
		var items []struct {
			FeatureKey string   `json:"featureKey"`
			HasAccess  bool     `json:"hasAccess"`
			Balance    *float64 `json:"balance"`
			Usage      *float64 `json:"usage"`
			Overage    *float64 `json:"overage"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			// Try wrapped response
			var wrapped struct {
				Items json.RawMessage `json:"items"`
			}
			if json.Unmarshal(raw, &wrapped) == nil && wrapped.Items != nil {
				json.Unmarshal(wrapped.Items, &items)
			}
			if items == nil {
				return customerAccessMsg{err: fmt.Errorf("parse access: %w", err)}
			}
		}
		rows := make([][]string, len(items))
		for i, a := range items {
			hasAccess := "no"
			if a.HasAccess {
				hasAccess = "yes"
			}
			balance := ""
			if a.Balance != nil {
				balance = fmt.Sprintf("%.2f", *a.Balance)
			}
			usage := ""
			if a.Usage != nil {
				usage = fmt.Sprintf("%.2f", *a.Usage)
			}
			overage := ""
			if a.Overage != nil {
				overage = fmt.Sprintf("%.2f", *a.Overage)
			}
			rows[i] = []string{a.FeatureKey, hasAccess, balance, usage, overage}
		}
		return customerAccessMsg{rows: rows}
	}
}

func (m *customersModel) loadApps(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListCustomerApps(id)
		if err != nil {
			return customerAppsMsg{err: err}
		}
		var items []struct {
			Type  string `json:"type"`
			AppID string `json:"appId"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			var wrapped struct {
				Items json.RawMessage `json:"items"`
			}
			if json.Unmarshal(raw, &wrapped) == nil && wrapped.Items != nil {
				json.Unmarshal(wrapped.Items, &items)
			}
			if items == nil {
				return customerAppsMsg{err: fmt.Errorf("parse apps: %w", err)}
			}
		}
		rows := make([][]string, len(items))
		for i, a := range items {
			rows[i] = []string{a.Type, a.AppID, ""}
		}
		return customerAppsMsg{rows: rows}
	}
}

func (m *customersModel) loadEntitlements(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListCustomerEntitlements(id)
		if err != nil {
			return customerEntitlementsMsg{err: err}
		}
		var items []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			FeatureKey string `json:"featureKey"`
			CreatedAt  string `json:"createdAt"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			var wrapped struct {
				Items json.RawMessage `json:"items"`
			}
			if json.Unmarshal(raw, &wrapped) == nil && wrapped.Items != nil {
				json.Unmarshal(wrapped.Items, &items)
			}
			if items == nil {
				return customerEntitlementsMsg{err: fmt.Errorf("parse entitlements: %w", err)}
			}
		}
		rows := make([][]string, len(items))
		ids := make([]string, len(items))
		for i, e := range items {
			rows[i] = []string{e.ID, e.Type, e.FeatureKey, e.CreatedAt}
			ids[i] = e.ID
		}
		return customerEntitlementsMsg{rows: rows, ids: ids}
	}
}

func (m *customersModel) loadEntitlementValue(custID, entID string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.GetEntitlementValue(custID, entID)
		if err != nil {
			return entitlementValueMsg{err: err}
		}
		var v struct {
			HasAccess bool        `json:"hasAccess"`
			Balance   json.Number `json:"balance"`
			Usage     json.Number `json:"usage"`
			Overage   json.Number `json:"overage"`
			Config    any         `json:"config"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return entitlementValueMsg{err: fmt.Errorf("parse value: %w", err)}
		}
		configStr := ""
		if v.Config != nil {
			if b, err := json.Marshal(v.Config); err == nil {
				configStr = string(b)
			}
		}
		return entitlementValueMsg{info: entitlementDetail{
			HasAccess: v.HasAccess,
			Balance:   v.Balance.String(),
			Usage:     v.Usage.String(),
			Overage:   v.Overage.String(),
			Config:    configStr,
		}}
	}
}

func (m *customersModel) loadEntitlementGrants(custID, entID string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListEntitlementGrants(custID, entID)
		if err != nil {
			return entitlementGrantsMsg{err: err}
		}
		var items []struct {
			ID          string      `json:"id"`
			Amount      json.Number `json:"amount"`
			Priority    json.Number `json:"priority"`
			EffectiveAt string      `json:"effectiveAt"`
			Expiration  struct {
				ExpiresAt string `json:"expiresAt"`
			} `json:"expiration"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			var wrapped struct {
				Items json.RawMessage `json:"items"`
			}
			if json.Unmarshal(raw, &wrapped) == nil && wrapped.Items != nil {
				json.Unmarshal(wrapped.Items, &items)
			}
			if items == nil {
				return entitlementGrantsMsg{err: fmt.Errorf("parse grants: %w", err)}
			}
		}
		rows := make([][]string, len(items))
		for i, g := range items {
			rows[i] = []string{g.ID, g.Amount.String(), g.Priority.String(), g.EffectiveAt, g.Expiration.ExpiresAt}
		}
		return entitlementGrantsMsg{rows: rows}
	}
}
