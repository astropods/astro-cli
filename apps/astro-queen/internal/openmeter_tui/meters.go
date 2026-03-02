package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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

	// Create form (huh)
	huhCreateForm     *huh.Form
	createFormFocused int
	formSlug          string
	formName          string
	formDesc          string
	formEventType     string
	formAgg           string
	formValueProp     string
	formGroupBy       string
	formEventFrom     string

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

	mkInput := func(placeholder string, charLimit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		ti.Prompt = ""
		return ti
	}

	return &metersModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
		updateFields: [3]textinput.Model{
			mkInput("display name", 256),
			mkInput("description", 1024),
			mkInput("model=$.model,type=$.type", 500),
		},
		queryFields: [5]textinput.Model{
			mkInput("subject-1", 200),
			mkInput("2024-01-01T00:00:00Z", 30),
			mkInput("2025-01-01T00:00:00Z", 30),
			mkInput("UTC", 50),
			mkInput("model,type", 200),
		},
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *metersModel) Name() string { return "Meters" }

func (m *metersModel) Init() tea.Cmd { return m.load() }

func (m *metersModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case metersLoadedMsg:
		if msg.err != nil {
			m.status = statusOK.Render("—")
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
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
			m.queryStatus = statusOK.Render("—")
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
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
		return m, m.openMeterCreateForm()

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
		return m.viewList()
	}
}

func (m *metersModel) viewList() string {
	boxW := m.width - 2 // border
	innerW := boxW - 2  // padding
	m.t.SetSize(innerW, m.height-2)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Width(boxW).
		Padding(0, 1).
		Render(m.t.View())
	return borderLabel(box, "Meters", colAccent)
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
	fieldW := 37 // input(24) + marker(1) + label(10) + pad(2)

	m.queryFields[0].Width = 24
	m.queryFields[1].Width = 24
	m.queryFields[2].Width = 24
	m.queryFields[3].Width = 20
	m.queryFields[4].Width = 24

	qRow1 := formLine(
		formField("Subject:", m.queryFields[0].View(), m.queryFocused == queryFieldSubject, fieldW),
		formField("Window:", renderWindowSizeSelector(m.queryWinIdx), m.queryFocused == queryFieldWindowSize, 0),
	)
	qRow2 := formLine(
		formField("From:", m.queryFields[1].View(), m.queryFocused == queryFieldFrom, fieldW),
		formField("To:", m.queryFields[2].View(), m.queryFocused == queryFieldTo, fieldW),
	)
	qRow3 := formLine(
		formField("Timezone:", m.queryFields[3].View(), m.queryFocused == queryFieldWindowTZ, 33),
		formField("Group By:", m.queryFields[4].View(), m.queryFocused == queryFieldGroupBy, fieldW),
	)

	b.WriteString(qRow1 + "\n")
	b.WriteString(qRow2 + "\n")
	b.WriteString(qRow3 + "\n")

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

func (m *metersModel) Tip() string {
	switch m.focus {
	case meterFocusDetail:
		return ""
	default:
		return "Meters define how events are aggregated. Select a meter and press Enter to inspect or query it."
	}
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

func (m *metersModel) Hints() []KeyHint {
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
		}
	}
}

// ─── create form (huh) ───────────────────────────────────────────────────────

const meterCreateFieldCount = 9

func (m *metersModel) buildMeterCreateForm() *huh.Form {
	m.formSlug = ""
	m.formName = ""
	m.formDesc = ""
	m.formEventType = ""
	m.formAgg = "SUM"
	m.formValueProp = ""
	m.formGroupBy = ""
	m.formEventFrom = ""

	aggOpts := make([]huh.Option[string], len(aggregations))
	for i, a := range aggregations {
		aggOpts[i] = huh.NewOption(a, a)
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("slug").
				Title("Slug *").
				Description("Unique meter identifier (alphanumeric + underscores).").
				Placeholder("tokens_total").
				Value(&m.formSlug),

			huh.NewInput().
				Key("name").
				Title("Name").
				Description("Human-readable display name.").
				Placeholder("Tokens Total").
				Value(&m.formName),

			huh.NewInput().
				Key("desc").
				Title("Description").
				Description("Optional description (max 1024).").
				Placeholder("AI Token Usage").
				Value(&m.formDesc),

			huh.NewInput().
				Key("eventType").
				Title("Event Type *").
				Description("CloudEvents type to match.").
				Placeholder("prompt").
				Value(&m.formEventType),

			huh.NewSelect[string]().
				Key("agg").
				Title("Aggregation *").
				Description("How matched events are combined.").
				Options(aggOpts...).
				Height(8).
				Value(&m.formAgg),

			huh.NewInput().
				Key("valueProp").
				Title("Value Property").
				Description("JSONPath to extract value. Required for non-COUNT aggregations.").
				Placeholder("$.tokens").
				Value(&m.formValueProp),

			huh.NewText().
				Key("groupBy").
				Title("Group By").
				Description("One entry per line: key=$.path").
				Placeholder("model=$.model\ntype=$.type").
				Lines(3).
				CharLimit(500).
				Value(&m.formGroupBy),

			huh.NewInput().
				Key("eventFrom").
				Title("Event From").
				Description("Only include events after this date (RFC 3339).").
				Placeholder("2024-01-01T00:00:00Z").
				Value(&m.formEventFrom),

			huh.NewConfirm().
				Key("confirm").
				Title("Create this meter?").
				Affirmative("Create").
				Negative("Cancel"),
		),
	).WithTheme(huhTheme).WithWidth(80)
}

func (m *metersModel) openMeterCreateForm() tea.Cmd {
	m.createFormFocused = 0
	m.huhCreateForm = m.buildMeterCreateForm()
	initCmd := m.huhCreateForm.Init()

	return tea.Batch(
		initCmd,
		func() tea.Msg {
			return showFormMsg{
				view:   m.viewMeterCreateForm,
				update: m.updateMeterCreateForm,
			}
		},
	)
}

func (m *metersModel) viewMeterCreateForm(width int) string {
	if m.huhCreateForm == nil {
		return ""
	}

	rightW := 38
	borderW := 1
	leftW := width - rightW - borderW - 2
	if leftW < 40 {
		leftW = 40
		rightW = width - leftW - borderW - 2
	}
	if rightW < 10 {
		rightW = 0
	}

	m.huhCreateForm = m.huhCreateForm.WithWidth(leftW - 2)
	formView := m.huhCreateForm.View()

	if rightW < 10 {
		return formView
	}

	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		PaddingRight(1).
		Render(formView)

	rightPane := lipgloss.NewStyle().
		Width(rightW).
		PaddingLeft(1).
		Render(m.meterCreateFieldHelp())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m *metersModel) updateMeterCreateForm(msg tea.Msg) (bool, tea.Cmd) {
	if m.huhCreateForm == nil {
		return true, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyTab:
			if m.createFormFocused < meterCreateFieldCount-1 {
				m.createFormFocused++
			}
		case tea.KeyShiftTab:
			if m.createFormFocused > 0 {
				m.createFormFocused--
			}
		case tea.KeyEnter:
			// Enter advances on input/select fields but not textarea (6) or confirm (8).
			if m.createFormFocused != 6 && m.createFormFocused != 8 && m.createFormFocused < meterCreateFieldCount-1 {
				m.createFormFocused++
			}
		}
	}

	form, cmd := m.huhCreateForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.huhCreateForm = f
	}

	if m.huhCreateForm.State == huh.StateCompleted {
		confirm := m.huhCreateForm.GetBool("confirm")
		m.huhCreateForm = nil
		if !confirm {
			return true, nil
		}
		return true, m.submitMeterCreate()
	}

	if m.huhCreateForm.State == huh.StateAborted {
		m.huhCreateForm = nil
		return true, nil
	}

	return false, cmd
}

func (m *metersModel) meterCreateFieldHelp() string {
	var title, body string
	switch m.createFormFocused {
	case 0:
		title = "Slug (required)"
		body = "Unique identifier for the meter.\nAlphanumeric + underscores only.\nUsed in API paths.\n\n  Example: tokens_total\n  Example: api_requests_total"
	case 1:
		title = "Display Name"
		body = "Human-readable name (1-256 chars).\nDefaults to the slug if omitted.\n\n  Example: Tokens Total"
	case 2:
		title = "Description"
		body = "Optional description (max 1024).\n\n  Example: AI Token Usage"
	case 3:
		title = "Event Type (required)"
		body = "The CloudEvents type to match.\nOnly events with this exact type\nare aggregated by the meter.\n\n  Example: prompt\n  Example: request"
	case 4:
		agg := m.formAgg
		if agg == "" {
			agg = "SUM"
		}
		body = "How matched events are combined.\n\n"
		switch agg {
		case "SUM":
			body += "Sums a numeric value property.\nRequires: valueProperty"
		case "COUNT":
			body += "Counts the number of events.\nNo value property needed."
		case "UNIQUE_COUNT":
			body += "Counts distinct values (string).\nRequires: valueProperty"
		case "AVG":
			body += "Averages the value property.\nRequires: valueProperty"
		case "MIN":
			body += "Minimum value in the window.\nRequires: valueProperty"
		case "MAX":
			body += "Maximum value in the window.\nRequires: valueProperty"
		case "LATEST":
			body += "Most recent value in period.\nRequires: valueProperty"
		}
		title = "Aggregation: " + agg
	case 5:
		title = "Value Property"
		body = "JSONPath to extract value from\nevent data.\n\nRequired for: SUM, AVG, MIN, MAX,\nUNIQUE_COUNT, LATEST.\nIgnored for COUNT.\n\n  Example: $.tokens\n  Example: $.duration_seconds"
	case 6:
		title = "Group By"
		body = "Named JSONPath expressions.\nOne entry per line.\nFormat: key=$.path\nShorthand: key (→ key=$.key)\n\n  model=$.model\n  type=$.type\n  region"
	case 7:
		title = "Event From"
		body = "Only include events after this date.\nRFC 3339 format. Optional.\n\n  Example: 2024-01-01T00:00:00Z"
	case 8:
		title = "Request Body"
		body = m.buildMeterCreatePreview()
	}
	return helpTitleRender(title) + "\n\n" + helpBodyRender(body)
}

func (m *metersModel) buildMeterCreatePreview() string {
	slug := strings.TrimSpace(m.formSlug)
	eventType := strings.TrimSpace(m.formEventType)
	agg := m.formAgg
	if agg == "" {
		agg = "SUM"
	}

	body := map[string]any{
		"slug":        slug,
		"eventType":   eventType,
		"aggregation": agg,
	}
	if n := strings.TrimSpace(m.formName); n != "" {
		body["name"] = n
	}
	if d := strings.TrimSpace(m.formDesc); d != "" {
		body["description"] = d
	}
	if vp := strings.TrimSpace(m.formValueProp); vp != "" {
		body["valueProperty"] = vp
	}
	if gb := strings.TrimSpace(m.formGroupBy); gb != "" {
		gb = strings.ReplaceAll(gb, "\n", ",")
		parsed := parseGroupBy(gb)
		if len(parsed) > 0 {
			body["groupBy"] = parsed
		}
	}
	if ef := strings.TrimSpace(m.formEventFrom); ef != "" {
		body["eventFrom"] = ef
	}

	b, _ := json.MarshalIndent(body, "", "  ")
	return string(b)
}

func (m *metersModel) submitMeterCreate() tea.Cmd {
	slug := strings.TrimSpace(m.formSlug)
	name := strings.TrimSpace(m.formName)
	desc := strings.TrimSpace(m.formDesc)
	eventType := strings.TrimSpace(m.formEventType)
	agg := m.formAgg
	valueProp := strings.TrimSpace(m.formValueProp)
	groupByStr := strings.TrimSpace(m.formGroupBy)
	eventFrom := strings.TrimSpace(m.formEventFrom)

	if slug == "" {
		return func() tea.Msg { return showErrMsg{"Slug is required"} }
	}
	if eventType == "" {
		return func() tea.Msg { return showErrMsg{"Event type is required"} }
	}
	if agg != "COUNT" && valueProp == "" {
		switch agg {
		case "SUM", "AVG", "MIN", "MAX", "UNIQUE_COUNT", "LATEST":
			return func() tea.Msg { return showErrMsg{agg + " requires a value property"} }
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
		groupByStr = strings.ReplaceAll(groupByStr, "\n", ",")
		gb := parseGroupBy(groupByStr)
		if len(gb) > 0 {
			body["groupBy"] = gb
		}
	}
	if eventFrom != "" {
		body["eventFrom"] = eventFrom
	}

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

func (m *metersModel) updateUpdateForm(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	key := keyMsg.String()

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
	m.updateFields[m.updateFocused], cmd = m.updateFields[m.updateFocused].Update(keyMsg)
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
