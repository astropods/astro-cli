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

type metersLoadedMsg struct {
	rows  [][]string
	slugs []string
	err   error
	at    time.Time
}

type meterDeletedMsg struct{ err error }
type meterCreatedMsg struct{ err error }
type meterUpdatedMsg struct{ err error }

type meterDetailMsg struct {
	info meterInfo
	err  error
}

type meterQueryResultMsg struct {
	header string
	rows   [][]string
	err    error
}

type meterInfo struct {
	Slug          string
	Name          string
	Description   string
	EventType     string
	Aggregation   string
	ValueProperty string
	GroupBy       map[string]string
}

// ─── enums (from OpenMeter OpenAPI spec) ─────────────────────────────────────

var aggregations = []string{"SUM", "COUNT", "UNIQUE_COUNT", "AVG", "MIN", "MAX", "LATEST"}
var windowSizes = []string{"", "MINUTE", "HOUR", "DAY", "MONTH"}

// ─── model ────────────────────────────────────────────────────────────────────

type meterFocus int

const (
	meterFocusList meterFocus = iota
	meterFocusDetail
)

// create form field indices
const (
	createFieldSlug = iota
	createFieldName
	createFieldDescription
	createFieldEventType
	createFieldAggregation // cycle selector
	createFieldValueProp
	createFieldGroupBy
	createFieldEventFrom
	createFieldCount
)

// update form field indices
const (
	updateFieldName = iota
	updateFieldDescription
	updateFieldGroupBy
	updateFieldCount
)

// query form field indices (inline in detail view)
const (
	queryFieldSubject = iota
	queryFieldFrom
	queryFieldTo
	queryFieldWindowSize // cycle selector
	queryFieldWindowTZ
	queryFieldGroupBy
	queryFieldCount
)

type metersModel struct {
	client *openmeter.Client
	t      tableModel
	slugs  []string
	status string
	width  int
	height int
	focus  meterFocus

	// Create form — 7 text inputs
	createFields  [7]textinput.Model
	createFocused int
	createAggIdx  int
	createStatus  string

	// Update form — 3 text inputs
	updateFields  [3]textinput.Model
	updateFocused int
	updateStatus  string

	// Detail view — meter info + inline query form + results
	detailSlug   string
	detailInfo   meterInfo
	detailLoaded bool

	// Inline query form fields
	queryFields     [5]textinput.Model
	queryFocused    int
	queryWinIdx     int
	queryFormStatus string

	// Query results
	queryHeader  string
	queryRows    [][]string
	queryStatus  string
	resultScroll int
}

func newMetersModel(client *openmeter.Client) *metersModel {
	t := newTableModel([]string{"Slug", "Name", "Event Type", "Aggregation", "Value Property", "Group By"})
	t.SetFocused(true)

	// Create form inputs
	slugInput := textinput.New()
	slugInput.Placeholder = "tokens_total"
	slugInput.CharLimit = 64

	nameInput := textinput.New()
	nameInput.Placeholder = "Tokens Total (defaults to slug)"
	nameInput.CharLimit = 256

	descInput := textinput.New()
	descInput.Placeholder = "AI Token Usage"
	descInput.CharLimit = 1024

	eventTypeInput := textinput.New()
	eventTypeInput.Placeholder = "prompt"
	eventTypeInput.CharLimit = 200

	valuePropInput := textinput.New()
	valuePropInput.Placeholder = "$.tokens"
	valuePropInput.CharLimit = 200

	groupByInput := textinput.New()
	groupByInput.Placeholder = "model=$.model,type=$.type"
	groupByInput.CharLimit = 500

	eventFromInput := textinput.New()
	eventFromInput.Placeholder = "2024-01-01T00:00:00Z (optional)"
	eventFromInput.CharLimit = 30

	// Update form inputs
	uName := textinput.New()
	uName.Placeholder = "display name"
	uName.CharLimit = 256

	uDesc := textinput.New()
	uDesc.Placeholder = "description"
	uDesc.CharLimit = 1024

	uGroupBy := textinput.New()
	uGroupBy.Placeholder = "model=$.model,type=$.type"
	uGroupBy.CharLimit = 500

	// Query form inputs
	qSubject := textinput.New()
	qSubject.Placeholder = "subject-1 (blank=all)"
	qSubject.CharLimit = 200

	qFrom := textinput.New()
	qFrom.Placeholder = "2024-01-01T00:00:00Z"
	qFrom.CharLimit = 30

	qTo := textinput.New()
	qTo.Placeholder = "2025-01-01T00:00:00Z"
	qTo.CharLimit = 30

	qWindowTZ := textinput.New()
	qWindowTZ.Placeholder = "UTC"
	qWindowTZ.CharLimit = 50

	qGroupBy := textinput.New()
	qGroupBy.Placeholder = "model,type (blank=all)"
	qGroupBy.CharLimit = 200

	return &metersModel{
		client:       client,
		t:            t,
		status:       statusWIP.Render("Loading…"),
		createFields: [7]textinput.Model{slugInput, nameInput, descInput, eventTypeInput, valuePropInput, groupByInput, eventFromInput},
		updateFields: [3]textinput.Model{uName, uDesc, uGroupBy},
		queryFields:  [5]textinput.Model{qSubject, qFrom, qTo, qWindowTZ, qGroupBy},
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *metersModel) Name() string { return "Meters" }

func (m *metersModel) Init() tea.Cmd { return m.load() }

func (m *metersModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case metersLoadedMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
		} else {
			m.t.SetRows(msg.rows)
			m.slugs = msg.slugs
			m.status = statusOK.Render(fmt.Sprintf(
				"%d meters  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
			))
		}
		return m, nil

	case meterDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case meterCreatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case meterDetailMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.detailInfo = msg.info
		m.detailLoaded = true
		return m, nil

	case meterUpdatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, tea.Batch(m.load(), m.loadDetail(m.detailSlug))

	case meterQueryResultMsg:
		if msg.err != nil {
			m.queryStatus = statusErr.Render("Query error: " + msg.err.Error())
		} else {
			m.queryHeader = msg.header
			m.queryRows = msg.rows
			m.queryStatus = statusOK.Render(fmt.Sprintf("%d rows", len(msg.rows)))
			m.resultScroll = 0
		}
		return m, nil

	case tea.KeyMsg:
		switch m.focus {
		case meterFocusList:
			return m.updateList(msg)
		case meterFocusDetail:
			return m.updateDetail(msg)
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *metersModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	if m.t.filtering {
		cmd := m.t.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "R":
		m.status = statusWIP.Render("Refreshing…")
		return m, m.load()

	case "enter":
		row := m.t.SelectedRow()
		idx := m.t.selectedRealIndex()
		if len(row) == 0 || idx < 0 || idx >= len(m.slugs) {
			return m, nil
		}
		slug := m.slugs[idx]
		m.focus = meterFocusDetail
		m.detailSlug = slug
		m.detailLoaded = false
		m.queryRows = nil
		m.queryHeader = ""
		m.queryStatus = ""
		m.resultScroll = 0
		// Reset and focus query form
		m.queryFocused = queryFieldSubject
		m.queryWinIdx = 0
		m.queryFormStatus = ""
		for i := range m.queryFields {
			m.queryFields[i].Reset()
			m.queryFields[i].Blur()
		}
		m.queryFields[0].Focus()
		return m, tea.Batch(m.loadDetail(slug), textinput.Blink)

	case "c":
		m.createFocused = createFieldSlug
		m.createAggIdx = 0
		m.createStatus = ""
		for i := range m.createFields {
			m.createFields[i].Reset()
			m.createFields[i].Blur()
		}
		m.createFields[0].Focus()
		return m, func() tea.Msg {
			return showFormMsg{
				view:   m.viewCreateForm,
				update: m.updateCreateForm,
			}
		}

	case "d":
		idx := m.t.selectedRealIndex()
		if idx < 0 || idx >= len(m.slugs) {
			return m, nil
		}
		slug := m.slugs[idx]
		return m, func() tea.Msg {
			return showConfirmMsg{
				text: fmt.Sprintf("Delete meter %s?", statusWIP.Render(slug)),
				fn: func() tea.Cmd {
					return func() tea.Msg {
						err := m.client.DeleteMeter(slug)
						return meterDeletedMsg{err}
					}
				},
			}
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

// ─── detail view (meter info + inline query form + results) ──────────────────

func (m *metersModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	key := msg.String()

	switch key {
	case "backspace":
		m.focus = meterFocusList
		return m, nil
	case "e":
		return m, m.openUpdateForm()
	case "enter":
		return m, m.submitQuery()
	case "tab", "down":
		return m, m.queryMoveFocus((m.queryFocused + 1) % queryFieldCount)
	case "shift+tab", "up":
		return m, m.queryMoveFocus((m.queryFocused + queryFieldCount - 1) % queryFieldCount)
	}

	// WindowSize cycle selector
	if m.queryFocused == queryFieldWindowSize {
		switch key {
		case "left", "h":
			m.queryWinIdx = (m.queryWinIdx + len(windowSizes) - 1) % len(windowSizes)
			return m, nil
		case "right", "l":
			m.queryWinIdx = (m.queryWinIdx + 1) % len(windowSizes)
			return m, nil
		}
		return m, nil
	}

	// Forward to active text input
	ti := queryTextFieldIdx(m.queryFocused)
	if ti >= 0 {
		var cmd tea.Cmd
		m.queryFields[ti], cmd = m.queryFields[ti].Update(msg)
		return m, cmd
	}

	return m, nil
}

func queryTextFieldIdx(logical int) int {
	switch logical {
	case queryFieldSubject:
		return 0
	case queryFieldFrom:
		return 1
	case queryFieldTo:
		return 2
	case queryFieldWindowTZ:
		return 3
	case queryFieldGroupBy:
		return 4
	default:
		return -1
	}
}

func (m *metersModel) queryMoveFocus(next int) tea.Cmd {
	if ti := queryTextFieldIdx(m.queryFocused); ti >= 0 {
		m.queryFields[ti].Blur()
	}
	m.queryFocused = next
	if ti := queryTextFieldIdx(next); ti >= 0 {
		m.queryFields[ti].Focus()
		return textinput.Blink
	}
	return nil
}

func (m *metersModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.focus {
	case meterFocusDetail:
		return m.viewDetail()
	default:
		tip := tipStyle.Render("Meters define how events are aggregated. Select a meter and press Enter to inspect or query it.")
		return tip + "\n" + m.t.View()
	}
}

func (m *metersModel) viewDetail() string {
	if !m.detailLoaded {
		return statusWIP.Render("Loading meter…")
	}

	var b strings.Builder
	info := m.detailInfo
	kvStyle := lipgloss.NewStyle().Foreground(colDimmed)
	valStyle := lipgloss.NewStyle().Foreground(colFg)

	// ─── header: meter summary ───
	b.WriteString(titleStyle.Render(info.Slug))
	if info.Name != "" && info.Name != info.Slug {
		b.WriteString("  " + valStyle.Render(info.Name))
	}
	b.WriteString("\n")

	summaryParts := []string{
		kvStyle.Render("event: ") + valStyle.Render(info.EventType),
		kvStyle.Render("agg: ") + valStyle.Render(info.Aggregation),
	}
	if info.ValueProperty != "" {
		summaryParts = append(summaryParts, kvStyle.Render("value: ")+valStyle.Render(info.ValueProperty))
	}
	if len(info.GroupBy) > 0 {
		gb := ""
		for k, v := range info.GroupBy {
			if gb != "" {
				gb += ", "
			}
			gb += k + "=" + v
		}
		summaryParts = append(summaryParts, kvStyle.Render("groupBy: ")+valStyle.Render(gb))
	}
	if info.Description != "" {
		summaryParts = append(summaryParts, kvStyle.Render("desc: ")+valStyle.Render(info.Description))
	}
	b.WriteString(strings.Join(summaryParts, "  ") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", m.width)) + "\n")

	// ─── inline query form ───
	formLabel := labelStyle.Width(14)
	for i := 0; i < queryFieldCount; i++ {
		marker := "  "
		if m.queryFocused == i {
			marker = focusStyle.Render("▸ ")
		}
		switch i {
		case queryFieldSubject:
			m.queryFields[0].Width = m.width - 18
			b.WriteString(marker + formLabel.Render("Subject:") + " " + m.queryFields[0].View() + "\n")
		case queryFieldFrom:
			m.queryFields[1].Width = 28
			m.queryFields[2].Width = 28
			// From and To on same line
			fromMarker := "  "
			toMarker := "  "
			if m.queryFocused == queryFieldFrom {
				fromMarker = focusStyle.Render("▸ ")
			}
			if m.queryFocused == queryFieldTo {
				toMarker = focusStyle.Render("▸ ")
			}
			b.WriteString(fromMarker + formLabel.Render("From:") + " " + m.queryFields[1].View())
			b.WriteString("  " + toMarker + formLabel.Render("To:") + " " + m.queryFields[2].View() + "\n")
		case queryFieldTo:
			continue // rendered with From
		case queryFieldWindowSize:
			b.WriteString(marker + formLabel.Render("Window:") + " " + renderWindowSizeSelector(m.queryWinIdx) + "\n")
		case queryFieldWindowTZ:
			m.queryFields[3].Width = 24
			m.queryFields[4].Width = m.width - 18 - 28 - 16
			tzMarker := "  "
			gbMarker := "  "
			if m.queryFocused == queryFieldWindowTZ {
				tzMarker = focusStyle.Render("▸ ")
			}
			if m.queryFocused == queryFieldGroupBy {
				gbMarker = focusStyle.Render("▸ ")
			}
			b.WriteString(tzMarker + formLabel.Render("Timezone:") + " " + m.queryFields[3].View())
			b.WriteString("  " + gbMarker + formLabel.Render("Group By:") + " " + m.queryFields[4].View() + "\n")
		case queryFieldGroupBy:
			continue // rendered with Timezone
		}
	}

	b.WriteString(lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", m.width)) + "\n")

	// ─── query results ───
	if m.queryFormStatus != "" {
		b.WriteString(m.queryFormStatus + "\n")
	}
	if m.queryStatus != "" {
		b.WriteString(m.queryStatus)
		if m.queryHeader != "" {
			b.WriteString("  " + descStyle.Render(m.queryHeader))
		}
		b.WriteString("\n")
	}

	if len(m.queryRows) > 0 {
		b.WriteString(headerStyle.Render("Window Start          Window End            Value         Subject       Group By") + "\n")
		// Show results with scroll
		visibleH := m.height - strings.Count(b.String(), "\n") - 2
		if visibleH < 1 {
			visibleH = 1
		}
		end := m.resultScroll + visibleH
		if end > len(m.queryRows) {
			end = len(m.queryRows)
		}
		start := m.resultScroll
		if start > len(m.queryRows) {
			start = len(m.queryRows)
		}
		for _, row := range m.queryRows[start:end] {
			b.WriteString(strings.Join(row, "  ") + "\n")
		}
	} else if m.queryStatus == "" {
		b.WriteString(descStyle.Render("Press Enter to query this meter") + "\n")
	}

	return b.String()
}

func (m *metersModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *metersModel) Status() string {
	switch m.focus {
	case meterFocusDetail:
		if m.queryStatus != "" {
			return m.queryStatus
		}
		return statusOK.Render("meter: " + m.detailSlug)
	default:
		return m.status
	}
}

func (m *metersModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case meterFocusDetail:
		return []KeyHint{
			{"↑↓/Tab", "fields"},
			{"←→", "window size"},
			{"Enter", "query"},
			{"e", "edit"},
			{"Bksp", "back"},
		}
	default:
		return []KeyHint{
			{"↑↓/jk", "navigate"},
			{"/", "search"},
			{"Enter", "detail"},
			{"c", "create"},
			{"d", "delete"},
			{"R", "refresh"},
			{"Esc", "nav mode"},
		}
	}
}

// ─── create form ─────────────────────────────────────────────────────────────

func createTextFieldIdx(logical int) int {
	switch logical {
	case createFieldSlug:
		return 0
	case createFieldName:
		return 1
	case createFieldDescription:
		return 2
	case createFieldEventType:
		return 3
	case createFieldValueProp:
		return 4
	case createFieldGroupBy:
		return 5
	case createFieldEventFrom:
		return 6
	default:
		return -1
	}
}

func (m *metersModel) updateCreateForm(msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		return false, m.createMoveFocus((m.createFocused + 1) % createFieldCount)
	case "shift+tab", "up":
		return false, m.createMoveFocus((m.createFocused + createFieldCount - 1) % createFieldCount)
	case "enter":
		cmd := m.submitCreate()
		if cmd == nil {
			return false, nil
		}
		return true, cmd
	}

	if m.createFocused == createFieldAggregation {
		switch key {
		case "left", "h":
			m.createAggIdx = (m.createAggIdx + len(aggregations) - 1) % len(aggregations)
			return false, nil
		case "right", "l":
			m.createAggIdx = (m.createAggIdx + 1) % len(aggregations)
			return false, nil
		}
		return false, nil
	}

	ti := createTextFieldIdx(m.createFocused)
	if ti >= 0 {
		var cmd tea.Cmd
		m.createFields[ti], cmd = m.createFields[ti].Update(msg)
		return false, cmd
	}

	return false, nil
}

func (m *metersModel) createMoveFocus(next int) tea.Cmd {
	if ti := createTextFieldIdx(m.createFocused); ti >= 0 {
		m.createFields[ti].Blur()
	}
	m.createFocused = next
	if ti := createTextFieldIdx(next); ti >= 0 {
		m.createFields[ti].Focus()
		return textinput.Blink
	}
	return nil
}

func (m *metersModel) submitCreate() tea.Cmd {
	slug := m.createFields[0].Value()
	name := m.createFields[1].Value()
	desc := m.createFields[2].Value()
	eventType := m.createFields[3].Value()
	agg := aggregations[m.createAggIdx]
	valueProp := m.createFields[4].Value()
	groupByStr := m.createFields[5].Value()
	eventFrom := m.createFields[6].Value()

	if slug == "" {
		m.createStatus = statusErr.Render("Slug is required")
		return nil
	}
	if eventType == "" {
		m.createStatus = statusErr.Render("Event type is required")
		return nil
	}
	if agg != "COUNT" && valueProp == "" {
		switch agg {
		case "SUM", "AVG", "MIN", "MAX", "UNIQUE_COUNT", "LATEST":
			m.createStatus = statusErr.Render(agg + " requires a value property")
			return nil
		}
	}

	body := map[string]any{
		"slug":        slug,
		"eventType":   eventType,
		"aggregation": agg,
	}
	if name != "" {
		body["name"] = name
	}
	if desc != "" {
		body["description"] = desc
	}
	if valueProp != "" {
		body["valueProperty"] = valueProp
	}
	if groupByStr != "" {
		gb := parseGroupBy(groupByStr)
		if len(gb) > 0 {
			body["groupBy"] = gb
		}
	}
	if eventFrom != "" {
		body["eventFrom"] = eventFrom
	}

	m.createStatus = statusWIP.Render("Creating…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.CreateMeter(raw)
		return meterCreatedMsg{err}
	}
}

func parseGroupBy(s string) map[string]string {
	gb := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			gb[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			gb[part] = "$." + part
		}
	}
	return gb
}

func (m *metersModel) createFieldHelp() (string, string) {
	switch m.createFocused {
	case createFieldSlug:
		return "Slug (required)",
			"Unique identifier for the meter.\nAlphanumeric + underscores only.\nUsed in API paths.\n\n  Example: tokens_total\n  Example: api_requests_total"
	case createFieldName:
		return "Display Name",
			"Human-readable name (1-256 chars).\nDefaults to the slug if omitted.\n\n  Example: Tokens Total"
	case createFieldDescription:
		return "Description",
			"Optional description (max 1024).\n\n  Example: AI Token Usage"
	case createFieldEventType:
		return "Event Type (required)",
			"The CloudEvents type to match.\nOnly events with this exact type\nare aggregated by the meter.\n\n  Example: prompt\n  Example: request"
	case createFieldAggregation:
		agg := aggregations[m.createAggIdx]
		base := "How matched events are combined.\n\n"
		switch agg {
		case "SUM":
			base += "Sums a numeric value property.\nRequires: valueProperty"
		case "COUNT":
			base += "Counts the number of events.\nNo value property needed."
		case "UNIQUE_COUNT":
			base += "Counts distinct values (string).\nRequires: valueProperty"
		case "AVG":
			base += "Averages the value property.\nRequires: valueProperty"
		case "MIN":
			base += "Minimum value in the window.\nRequires: valueProperty"
		case "MAX":
			base += "Maximum value in the window.\nRequires: valueProperty"
		case "LATEST":
			base += "Most recent value in period.\nRequires: valueProperty"
		}
		return "Aggregation: " + agg, base
	case createFieldValueProp:
		return "Value Property",
			"JSONPath to extract value from\nevent data.\n\nRequired for: SUM, AVG, MIN, MAX,\nUNIQUE_COUNT, LATEST.\nIgnored for COUNT.\n\n  Example: $.tokens\n  Example: $.duration_seconds"
	case createFieldGroupBy:
		return "Group By",
			"Named JSONPath expressions.\nFormat: key=$.path\nShorthand: key (→ key=$.key)\n\n  Example: model=$.model,type=$.type\n  Example: region,gpu_type"
	case createFieldEventFrom:
		return "Event From",
			"Only include events after this date.\nRFC 3339 format. Optional.\n\n  Example: 2024-01-01T00:00:00Z"
	}
	return "", ""
}

func (m *metersModel) viewCreateForm(width int) string {
	return m.viewTwoPaneForm(width, "Create Meter", m.createFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Slug:", createFieldSlug, true},
			{"Name:", createFieldName, false},
			{"Description:", createFieldDescription, false},
			{"Event Type:", createFieldEventType, true},
			{"Aggregation:", createFieldAggregation, true},
			{"Value Prop:", createFieldValueProp, false},
			{"Group By:", createFieldGroupBy, false},
			{"Event From:", createFieldEventFrom, false},
		}
		title, body := m.createFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		if r.logical == createFieldAggregation {
			return renderAggSelector(m.createAggIdx)
		}
		ti := createTextFieldIdx(r.logical)
		m.createFields[ti].Width = inputW
		return m.createFields[ti].View()
	}, m.createStatus)
}

// ─── update form ─────────────────────────────────────────────────────────────

func (m *metersModel) openUpdateForm() tea.Cmd {
	m.updateFocused = updateFieldName
	m.updateStatus = ""
	for i := range m.updateFields {
		m.updateFields[i].Reset()
		m.updateFields[i].Blur()
	}
	// Pre-fill from detail info
	if m.detailLoaded {
		m.updateFields[0].SetValue(m.detailInfo.Name)
		m.updateFields[1].SetValue(m.detailInfo.Description)
		gb := ""
		for k, v := range m.detailInfo.GroupBy {
			if gb != "" {
				gb += ","
			}
			gb += k + "=" + v
		}
		m.updateFields[2].SetValue(gb)
	}
	m.updateFields[0].Focus()
	return func() tea.Msg {
		return showFormMsg{
			view:   m.viewUpdateForm,
			update: m.updateUpdateForm,
		}
	}
}

func (m *metersModel) updateUpdateForm(msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		next := (m.updateFocused + 1) % updateFieldCount
		m.updateFields[m.updateFocused].Blur()
		m.updateFocused = next
		m.updateFields[next].Focus()
		return false, textinput.Blink
	case "shift+tab", "up":
		next := (m.updateFocused + updateFieldCount - 1) % updateFieldCount
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
	m.updateFields[m.updateFocused], cmd = m.updateFields[m.updateFocused].Update(msg)
	return false, cmd
}

func (m *metersModel) submitUpdate() tea.Cmd {
	name := m.updateFields[0].Value()
	desc := m.updateFields[1].Value()
	groupByStr := m.updateFields[2].Value()

	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if desc != "" {
		body["description"] = desc
	}
	if groupByStr != "" {
		gb := parseGroupBy(groupByStr)
		if len(gb) > 0 {
			body["groupBy"] = gb
		}
	}

	if len(body) == 0 {
		m.updateStatus = statusErr.Render("Nothing to update")
		return nil
	}

	slug := m.detailSlug
	m.updateStatus = statusWIP.Render("Updating…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.UpdateMeter(slug, raw)
		return meterUpdatedMsg{err}
	}
}

func (m *metersModel) updateFieldHelp() (string, string) {
	switch m.updateFocused {
	case updateFieldName:
		return "Display Name",
			"Human-readable name (1-256 chars).\n\nUpdating the name does not\naffect the slug or API paths."
	case updateFieldDescription:
		return "Description",
			"Optional description (max 1024).\n\nDescribe what this meter tracks."
	case updateFieldGroupBy:
		return "Group By",
			"Named JSONPath expressions.\nFormat: key=$.path\n\nThis replaces the current groupBy.\nLeave blank to keep existing.\n\n  Example: model=$.model,type=$.type"
	}
	return "", ""
}

func (m *metersModel) viewUpdateForm(width int) string {
	return m.viewTwoPaneForm(width, "Update Meter: "+m.detailSlug, m.updateFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Name:", updateFieldName, false},
			{"Description:", updateFieldDescription, false},
			{"Group By:", updateFieldGroupBy, false},
		}
		title, body := m.updateFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		m.updateFields[r.logical].Width = inputW
		return m.updateFields[r.logical].View()
	}, m.updateStatus)
}

// ─── query submit ────────────────────────────────────────────────────────────

func (m *metersModel) submitQuery() tea.Cmd {
	subject := m.queryFields[0].Value()
	from := m.queryFields[1].Value()
	to := m.queryFields[2].Value()
	windowTZ := m.queryFields[3].Value()
	groupBy := m.queryFields[4].Value()
	winSize := windowSizes[m.queryWinIdx]

	slug := m.detailSlug
	m.queryStatus = statusWIP.Render("Querying…")
	return func() tea.Msg {
		body := map[string]any{}
		if subject != "" {
			body["subject"] = []string{subject}
		}
		if from != "" {
			body["from"] = from
		}
		if to != "" {
			body["to"] = to
		}
		if winSize != "" {
			body["windowSize"] = winSize
		}
		if windowTZ != "" {
			body["windowTimeZone"] = windowTZ
		}
		if groupBy != "" {
			var groups []string
			for _, g := range strings.Split(groupBy, ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					groups = append(groups, g)
				}
			}
			if len(groups) > 0 {
				body["groupBy"] = groups
			}
		}

		reqBody, _ := json.Marshal(body)
		raw, err := m.client.QueryMeter(slug, reqBody)
		if err != nil {
			return meterQueryResultMsg{err: err}
		}

		var result struct {
			From       string `json:"from"`
			To         string `json:"to"`
			WindowSize string `json:"windowSize"`
			Data       []struct {
				WindowStart string            `json:"windowStart"`
				WindowEnd   string            `json:"windowEnd"`
				Value       json.Number       `json:"value"`
				Subject     *string           `json:"subject"`
				CustomerID  string            `json:"customerId"`
				GroupBy     map[string]string `json:"groupBy"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return meterQueryResultMsg{rows: [][]string{{string(raw)}}}
		}

		header := ""
		if result.From != "" || result.To != "" || result.WindowSize != "" {
			var parts []string
			if result.From != "" {
				parts = append(parts, "from: "+result.From)
			}
			if result.To != "" {
				parts = append(parts, "to: "+result.To)
			}
			if result.WindowSize != "" {
				parts = append(parts, "window: "+result.WindowSize)
			}
			header = strings.Join(parts, "  ")
		}

		rows := make([][]string, len(result.Data))
		for i, d := range result.Data {
			subj := ""
			if d.Subject != nil {
				subj = *d.Subject
			}
			gbStr := ""
			for k, v := range d.GroupBy {
				if gbStr != "" {
					gbStr += ", "
				}
				gbStr += k + "=" + v
			}
			rows[i] = []string{
				trunc(d.WindowStart, 22),
				trunc(d.WindowEnd, 22),
				d.Value.String(),
				subj,
				gbStr,
			}
		}
		return meterQueryResultMsg{header: header, rows: rows}
	}
}

// ─── shared two-pane form renderer (for create/update modals) ────────────────

type formRow struct {
	label    string
	logical  int
	required bool
}

var (
	helpTitleRender = lipgloss.NewStyle().Bold(true).Foreground(colPurple).Render
	helpBodyRender  = lipgloss.NewStyle().Foreground(colFg).Render
)

func renderAggSelector(activeIdx int) string {
	aggActive := lipgloss.NewStyle().Background(colAccent).Foreground(colBg).Padding(0, 1)
	aggInactive := lipgloss.NewStyle().Foreground(colMuted)
	var opts []string
	for i, a := range aggregations {
		if i == activeIdx {
			opts = append(opts, aggActive.Render(a))
		} else {
			opts = append(opts, aggInactive.Render(a))
		}
	}
	return strings.Join(opts, " ")
}

func renderWindowSizeSelector(activeIdx int) string {
	wsActive := lipgloss.NewStyle().Background(colAccent).Foreground(colBg).Padding(0, 1)
	wsInactive := lipgloss.NewStyle().Foreground(colMuted)
	var opts []string
	for i, w := range windowSizes {
		label := w
		if label == "" {
			label = "TOTAL"
		}
		if i == activeIdx {
			opts = append(opts, wsActive.Render(label))
		} else {
			opts = append(opts, wsInactive.Render(label))
		}
	}
	return strings.Join(opts, " ")
}

func (m *metersModel) viewTwoPaneForm(
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

func (m *metersModel) load() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListMeters()
		if err != nil {
			return metersLoadedMsg{err: err, at: time.Now()}
		}

		var meters []struct {
			ID            string            `json:"id"`
			Slug          string            `json:"slug"`
			Name          string            `json:"name"`
			Description   string            `json:"description"`
			EventType     string            `json:"eventType"`
			Aggregation   string            `json:"aggregation"`
			ValueProperty string            `json:"valueProperty"`
			GroupBy       map[string]string `json:"groupBy"`
			CreatedAt     string            `json:"createdAt"`
		}
		if err := json.Unmarshal(raw, &meters); err != nil {
			return metersLoadedMsg{err: fmt.Errorf("parse meters: %w", err), at: time.Now()}
		}

		rows := make([][]string, len(meters))
		slugs := make([]string, len(meters))
		for i, m := range meters {
			groupBy := ""
			for k, v := range m.GroupBy {
				if groupBy != "" {
					groupBy += ", "
				}
				groupBy += k + "=" + v
			}
			displayName := m.Name
			if displayName == "" {
				displayName = m.Slug
			}
			rows[i] = []string{
				m.Slug,
				displayName,
				m.EventType,
				m.Aggregation,
				m.ValueProperty,
				groupBy,
			}
			slugs[i] = m.Slug
			if slugs[i] == "" {
				slugs[i] = m.ID
			}
		}
		return metersLoadedMsg{rows: rows, slugs: slugs, at: time.Now()}
	}
}

func (m *metersModel) loadDetail(slug string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.GetMeter(slug)
		if err != nil {
			return meterDetailMsg{err: err}
		}
		var meter struct {
			Slug          string            `json:"slug"`
			Name          string            `json:"name"`
			Description   string            `json:"description"`
			EventType     string            `json:"eventType"`
			Aggregation   string            `json:"aggregation"`
			ValueProperty string            `json:"valueProperty"`
			GroupBy       map[string]string `json:"groupBy"`
		}
		if err := json.Unmarshal(raw, &meter); err != nil {
			return meterDetailMsg{err: fmt.Errorf("parse meter: %w", err)}
		}
		return meterDetailMsg{info: meterInfo{
			Slug:          meter.Slug,
			Name:          meter.Name,
			Description:   meter.Description,
			EventType:     meter.EventType,
			Aggregation:   meter.Aggregation,
			ValueProperty: meter.ValueProperty,
			GroupBy:       meter.GroupBy,
		}}
	}
}
