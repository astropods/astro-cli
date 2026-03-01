package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"net/url"
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

type meterQueryResultMsg struct {
	header string
	rows   [][]string
	err    error
}

type meterSubjectsMsg struct {
	rows [][]string
	err  error
}

// ─── aggregation & windowSize enums (from OpenMeter OpenAPI spec) ────────────

var aggregations = []string{"SUM", "COUNT", "UNIQUE_COUNT", "AVG", "MIN", "MAX", "LATEST"}
var windowSizes = []string{"", "MINUTE", "HOUR", "DAY", "MONTH"}

// ─── model ────────────────────────────────────────────────────────────────────

type meterFocus int

const (
	meterFocusList meterFocus = iota
	meterFocusDetail
	meterFocusQuery
)

// create form field indices (logical fields, not all are text inputs)
const (
	createFieldSlug = iota
	createFieldName
	createFieldDescription
	createFieldEventType
	createFieldAggregation // cycle selector
	createFieldValueProp
	createFieldGroupBy
	createFieldEventFrom
	createFieldCount // sentinel
)

type metersModel struct {
	client *openmeter.Client
	t      tableModel
	slugs  []string
	status string
	width  int
	height int
	focus  meterFocus

	// Create form — 7 text inputs (slug, name, description, eventType, valueProp, groupBy, eventFrom)
	createFields  [7]textinput.Model
	createFocused int
	createAggIdx  int
	createStatus  string

	// Detail sub-view
	detailJSON   string
	detailSlug   string
	detailScroll int

	// Query sub-view
	queryHeader string
	queryRows   [][]string
	queryStatus string
}

func newMetersModel(client *openmeter.Client) *metersModel {
	t := newTableModel([]string{"Slug", "Name", "Event Type", "Aggregation", "Value Property", "Group By"})
	t.SetFocused(true)

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

	return &metersModel{
		client:       client,
		t:            t,
		status:       statusWIP.Render("Loading…"),
		createFields: [7]textinput.Model{slugInput, nameInput, descInput, eventTypeInput, valuePropInput, groupByInput, eventFromInput},
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

	case meterQueryResultMsg:
		if msg.err != nil {
			m.queryStatus = statusErr.Render("Query error: " + msg.err.Error())
		} else {
			m.queryHeader = msg.header
			m.queryRows = msg.rows
			m.queryStatus = statusOK.Render(fmt.Sprintf("%d rows", len(msg.rows)))
		}
		return m, nil

	case meterSubjectsMsg:
		if msg.err != nil {
			m.queryStatus = statusErr.Render("Error: " + msg.err.Error())
		} else {
			m.queryStatus = statusOK.Render(fmt.Sprintf("%d subjects", len(msg.rows)))
			m.queryRows = msg.rows
			m.queryHeader = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch m.focus {
		case meterFocusList:
			return m.updateList(msg)
		case meterFocusDetail:
			return m.updateDetail(msg)
		case meterFocusQuery:
			return m.updateQuery(msg)
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
		m.detailScroll = 0
		m.queryRows = nil
		m.queryHeader = ""
		m.queryStatus = ""
		return m, m.loadDetail(slug)

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

// ─── create form ─────────────────────────────────────────────────────────────

// textFieldIdx maps logical field index → textinput array index, or -1 for aggregation.
func textFieldIdx(logical int) int {
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
		return -1 // aggregation
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

	ti := textFieldIdx(m.createFocused)
	if ti >= 0 {
		var cmd tea.Cmd
		m.createFields[ti], cmd = m.createFields[ti].Update(msg)
		return false, cmd
	}

	return false, nil
}

func (m *metersModel) createMoveFocus(next int) tea.Cmd {
	if ti := textFieldIdx(m.createFocused); ti >= 0 {
		m.createFields[ti].Blur()
	}
	m.createFocused = next
	if ti := textFieldIdx(next); ti >= 0 {
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

// parseGroupBy parses "key=$.path,key2=$.path2" or "key1,key2" (shorthand: key=$.<key>).
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

// ─── field help (right pane of create form) ──────────────────────────────────

func (m *metersModel) fieldHelp() (string, string) {
	switch m.createFocused {
	case createFieldSlug:
		return "Slug (required)",
			"Unique identifier for the meter.\n" +
				"Alphanumeric + underscores only.\n" +
				"Used in API paths.\n\n" +
				"  Example: tokens_total\n" +
				"  Example: api_requests_total\n" +
				"  Example: gpu_execution_time"
	case createFieldName:
		return "Display Name",
			"Human-readable name (1-256 chars).\n" +
				"Defaults to the slug if omitted.\n\n" +
				"  Example: Tokens Total\n" +
				"  Example: API Requests"
	case createFieldDescription:
		return "Description",
			"Optional description (max 1024).\n\n" +
				"  Example: AI Token Usage\n" +
				"  Example: Total API calls"
	case createFieldEventType:
		return "Event Type (required)",
			"The CloudEvents type to match.\n" +
				"Only events with this exact type\n" +
				"are aggregated by the meter.\n\n" +
				"  Example: prompt\n" +
				"  Example: request\n" +
				"  Example: gpu_time"
	case createFieldAggregation:
		agg := aggregations[m.createAggIdx]
		base := "How matched events are combined.\n\n"
		switch agg {
		case "SUM":
			base += "Sums a numeric value property.\n" +
				"Use for: tokens, bytes, duration.\n" +
				"Requires: valueProperty"
		case "COUNT":
			base += "Counts the number of events.\n" +
				"No value property needed."
		case "UNIQUE_COUNT":
			base += "Counts distinct values of the\n" +
				"value property (must be string).\n" +
				"Requires: valueProperty"
		case "AVG":
			base += "Averages the value property.\n" +
				"Requires: valueProperty"
		case "MIN":
			base += "Minimum value in the window.\n" +
				"Requires: valueProperty"
		case "MAX":
			base += "Maximum value in the window.\n" +
				"Requires: valueProperty"
		case "LATEST":
			base += "Most recent value in the period.\n" +
				"Useful for resource tracking.\n" +
				"Requires: valueProperty"
		}
		return "Aggregation: " + agg, base
	case createFieldValueProp:
		return "Value Property",
			"JSONPath to extract the value from\n" +
				"ingested event data.\n\n" +
				"Required for: SUM, AVG, MIN, MAX,\n" +
				"UNIQUE_COUNT, LATEST.\n" +
				"Ignored for COUNT.\n\n" +
				"SUM/AVG/MIN/MAX: must be number.\n" +
				"UNIQUE_COUNT: must be string.\n\n" +
				"  Example: $.tokens\n" +
				"  Example: $.duration_seconds\n" +
				"  Example: $.session_id"
	case createFieldGroupBy:
		return "Group By",
			"Named JSONPath expressions to group\n" +
				"results by. Format: key=$.path\n\n" +
				"Shorthand: key (expands to key=$.key)\n\n" +
				"  Example: model=$.model,type=$.type\n" +
				"  Example: method=$.method,route=$.route\n" +
				"  Example: region,gpu_type"
	case createFieldEventFrom:
		return "Event From",
			"Only include events after this date.\n" +
				"RFC 3339 format. Optional.\n\n" +
				"Useful to skip old historical events\n" +
				"when creating a new meter.\n\n" +
				"  Example: 2024-01-01T00:00:00Z\n" +
				"  Example: 2025-06-15T12:00:00Z"
	}
	return "", ""
}

func (m *metersModel) viewCreateForm(width int) string {
	rightW := 40
	borderW := 1                          // right border on left pane acts as divider
	leftW := width - rightW - borderW - 2 // 2 for padding
	if leftW < 35 {
		leftW = 35
		rightW = width - leftW - borderW - 2
	}

	requiredLabel := labelStyle.Bold(true).Width(16)
	optionalLabel := labelStyle.Width(16)
	aggActive := lipgloss.NewStyle().Background(colAccent).Foreground(colBg).Padding(0, 1)
	aggInactive := lipgloss.NewStyle().Foreground(colMuted)
	helpTitleStyle := lipgloss.NewStyle().Bold(true).Foreground(colPurple)
	helpBodyStyle := lipgloss.NewStyle().Foreground(colFg)

	inputW := leftW - 20
	if inputW < 10 {
		inputW = 10
	}
	for i := range m.createFields {
		m.createFields[i].Width = inputW
	}

	type formRow struct {
		label    string
		logical  int
		required bool
	}
	formRows := []formRow{
		{"Slug:", createFieldSlug, true},
		{"Name:", createFieldName, false},
		{"Description:", createFieldDescription, false},
		{"Event Type:", createFieldEventType, true},
		{"Aggregation:", createFieldAggregation, true},
		{"Value Prop:", createFieldValueProp, false},
		{"Group By:", createFieldGroupBy, false},
		{"Event From:", createFieldEventFrom, false},
	}

	var leftLines []string
	leftLines = append(leftLines, titleStyle.Render("Create Meter"))
	leftLines = append(leftLines, "")

	for _, r := range formRows {
		marker := "  "
		if m.createFocused == r.logical {
			marker = focusStyle.Render("▸ ")
		}
		lbl := optionalLabel
		if r.required {
			lbl = requiredLabel
		}

		if r.logical == createFieldAggregation {
			var opts []string
			for i, a := range aggregations {
				if i == m.createAggIdx {
					opts = append(opts, aggActive.Render(a))
				} else {
					opts = append(opts, aggInactive.Render(a))
				}
			}
			leftLines = append(leftLines, marker+lbl.Render(r.label)+" "+strings.Join(opts, " "))
		} else {
			ti := textFieldIdx(r.logical)
			leftLines = append(leftLines, marker+lbl.Render(r.label)+" "+m.createFields[ti].View())
		}
	}

	leftLines = append(leftLines, "")
	if m.createStatus != "" {
		leftLines = append(leftLines, m.createStatus)
	}
	leftLines = append(leftLines, descStyle.Render("↑↓/Tab • ←→ agg • Enter submit • Esc cancel"))

	// Left pane: fixed width with a right border as divider
	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		PaddingRight(1).
		Render(strings.Join(leftLines, "\n"))

	// Right pane: help content
	helpTitleStr, helpBodyStr := m.fieldHelp()
	rightContent := helpTitleStyle.Render(helpTitleStr) + "\n\n" + helpBodyStyle.Render(helpBodyStr)
	rightPane := lipgloss.NewStyle().
		Width(rightW).
		PaddingLeft(1).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

// ─── detail / query sub-views ────────────────────────────────────────────────

func (m *metersModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "backspace", "esc":
		m.focus = meterFocusList
		return m, nil
	case "q":
		return m, func() tea.Msg {
			return showInputMsg{
				title:       fmt.Sprintf("Query %s — subject filter (blank=all), or subject,from,to,windowSize", statusWIP.Render(m.detailSlug)),
				placeholder: "subject1 or subject1,2024-01-01T00:00:00Z,2024-02-01T00:00:00Z,DAY",
				fn: func(val string) tea.Cmd {
					return m.queryMeter(m.detailSlug, val)
				},
			}
		}
	case "s":
		return m, m.loadSubjects(m.detailSlug)
	case "up", "k":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "down", "j":
		m.detailScroll++
	case "home", "g":
		m.detailScroll = 0
	}
	return m, nil
}

func (m *metersModel) updateQuery(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "backspace", "esc":
		m.focus = meterFocusDetail
		m.queryRows = nil
		m.queryHeader = ""
		return m, nil
	}
	return m, nil
}

func (m *metersModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.focus {
	case meterFocusDetail:
		return m.viewDetail()
	case meterFocusQuery:
		return m.viewQuery()
	default:
		tip := tipStyle.Render("Meters define how events are aggregated. Select a meter and press Enter to inspect or query it.")
		return tip + "\n" + m.t.View()
	}
}

func (m *metersModel) viewDetail() string {
	if m.detailJSON == "" {
		return statusWIP.Render("Loading meter detail…")
	}

	lines := strings.Split(m.detailJSON, "\n")

	var b strings.Builder
	b.WriteString(titleStyle.Render("Meter: "+m.detailSlug) + "\n\n")

	end := m.detailScroll + m.height - 4
	if end > len(lines) {
		end = len(lines)
	}
	start := m.detailScroll
	if start > len(lines) {
		start = len(lines)
	}

	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + descStyle.Render("q query  •  s subjects  •  backspace back"))
	return b.String()
}

func (m *metersModel) viewQuery() string {
	if m.queryStatus == "" && len(m.queryRows) == 0 {
		return statusWIP.Render("Running query…")
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Query Results: "+m.detailSlug) + "\n")

	if m.queryHeader != "" {
		b.WriteString(descStyle.Render(m.queryHeader) + "\n")
	}
	b.WriteString("\n")

	if m.queryStatus != "" {
		b.WriteString(m.queryStatus + "\n\n")
	}

	// Header for data rows
	if len(m.queryRows) > 0 {
		b.WriteString(headerStyle.Render("Window Start          Window End            Value         Subject       Group By") + "\n")
	}
	for _, row := range m.queryRows {
		b.WriteString(strings.Join(row, "  ") + "\n")
	}

	b.WriteString("\n" + descStyle.Render("backspace back"))
	return b.String()
}

func (m *metersModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *metersModel) Status() string {
	switch m.focus {
	case meterFocusDetail:
		return statusOK.Render("meter: " + m.detailSlug)
	case meterFocusQuery:
		if m.queryStatus != "" {
			return m.queryStatus
		}
		return statusWIP.Render("querying…")
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
			{"↑↓/jk", "scroll"},
			{"q", "query"},
			{"s", "subjects"},
			{"Bksp", "back"},
		}
	case meterFocusQuery:
		return []KeyHint{
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
			return showErrMsg{err.Error()}
		}
		var pretty json.RawMessage
		if err := json.Unmarshal(raw, &pretty); err != nil {
			m.detailJSON = string(raw)
		} else {
			indented, _ := json.MarshalIndent(pretty, "", "  ")
			m.detailJSON = string(indented)
		}
		return nil
	}
}

// queryMeter parses user input as "subject" or "subject,from,to,windowSize" and queries.
func (m *metersModel) queryMeter(slug, input string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		parts := strings.Split(input, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}

		if len(parts) >= 1 && parts[0] != "" {
			params.Add("subject", parts[0])
		}
		if len(parts) >= 2 && parts[1] != "" {
			params.Set("from", parts[1])
		}
		if len(parts) >= 3 && parts[2] != "" {
			params.Set("to", parts[2])
		}
		if len(parts) >= 4 && parts[3] != "" {
			params.Set("windowSize", strings.ToUpper(parts[3]))
		}

		raw, err := m.client.QueryMeter(slug, params.Encode())
		if err != nil {
			return meterQueryResultMsg{err: err}
		}

		// Parse MeterQueryResult per OpenMeter spec
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
			m.focus = meterFocusQuery
			return meterQueryResultMsg{rows: [][]string{{string(raw)}}}
		}

		// Build header summary
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
		m.focus = meterFocusQuery
		return meterQueryResultMsg{header: header, rows: rows}
	}
}

func (m *metersModel) loadSubjects(slug string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListMeterSubjects(slug, "")
		if err != nil {
			return meterSubjectsMsg{err: err}
		}

		var subjects []string
		if err := json.Unmarshal(raw, &subjects); err != nil {
			return meterSubjectsMsg{err: fmt.Errorf("parse: %w", err)}
		}

		rows := make([][]string, len(subjects))
		for i, s := range subjects {
			rows[i] = []string{s}
		}
		m.focus = meterFocusQuery
		m.queryStatus = statusOK.Render(fmt.Sprintf("%d subjects", len(subjects)))
		return meterSubjectsMsg{rows: rows}
	}
}
