package openmeter_tui

import (
	"crypto/rand"
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

type eventsLoadedMsg struct {
	rows       [][]string
	rawEvents  []json.RawMessage
	nextCursor string // v2 only
	err        error
	at         time.Time
}

type eventEmittedMsg struct{ err error }

type meterTypesLoadedMsg struct {
	types []string
	err   error
}

// ─── simple mode fields (v1 API) ────────────────────────────────────────────

const (
	simpleID = iota
	simpleSubject
	simpleCustomerID
	simpleFrom
	simpleTo
	simpleLimit
	simpleCount
)

// ─── advanced mode fields (v2 API) ──────────────────────────────────────────

const (
	advID = iota
	advSubject
	advType
	advSource
	advCustomerID
	advFrom
	advTo
	advIngestedFrom
	advIngestedTo
	advLimit
	advCount
)

// ─── focus constants ─────────────────────────────────────────────────────────

const (
	focusFilter = 0 // left pane, filter fields (text inputs)
	focusEmit   = 1 // left pane, emit fields (text inputs)
	focusTable  = 2 // right pane, table (no text input)
)

// ─── emit field indices ─────────────────────────────────────────────────────

const (
	emitFieldType    = 0 // handled by SearchSelect, not textinput
	emitFieldSubject = 1
	emitFieldSource  = 2
	emitFieldData    = 3
	emitFieldCount   = 4
)

// ─── model ────────────────────────────────────────────────────────────────────

type eventViewMode int

const (
	eventModeList eventViewMode = iota
	eventModeDetail
)

type eventsModel struct {
	client *openmeter.Client
	t      tableModel
	status string
	width  int
	height int
	mode   eventViewMode

	// Mode toggle
	advanced bool

	// Simple filter (v1): id, subject, customerId, from, to, limit
	simpleFields  [simpleCount]textinput.Model
	simpleFocused int

	// Advanced filter (v2): id, subject, type, source, customerId, from, to, ingestedFrom, ingestedTo, limit
	advFields  [advCount]textinput.Model
	advFocused int

	// Pagination
	nextCursor string // v2 cursor

	// Raw events for detail
	rawEvents []json.RawMessage

	// Detail view
	detailLines  []string
	detailScroll int

	// Focus state: 0=filter, 1=emit, 2=table
	focus           int
	emitTypeSelect  SearchSelect
	emitFocused     int // 0=type, 1..3=subject/source/data
	emitFields      [emitFieldCount]textinput.Model
	meterEventTypes []string // fetched from meters for the type selector
}

func newEventsModel(client *openmeter.Client) *eventsModel {
	mkInput := func(placeholder string, charLimit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		ti.Prompt = ""
		return ti
	}

	m := &eventsModel{
		client: client,
		t: func() tableModel {
			t := newTableModel([]string{"ID", "Type", "Subject", "Source", "Time", "Ingested At"})
			t.SetFocused(true)
			return t
		}(),
		status: statusWIP.Render("Loading…"),
		simpleFields: [simpleCount]textinput.Model{
			mkInput("event-id", 200),
			mkInput("customer-1", 200),
			mkInput("01G65Z755A...", 36),
			mkInput("2024-01-01", 30),
			mkInput("2025-01-01", 30),
			mkInput("100", 3),
		},
		advFields: [advCount]textinput.Model{
			mkInput("event-id", 200),
			mkInput("customer-1", 200),
			mkInput("prompt", 200),
			mkInput("service-name", 200),
			mkInput("01G65Z755A...", 36),
			mkInput("2024-01-01", 30),
			mkInput("2025-01-01", 30),
			mkInput("2024-01-01", 30),
			mkInput("2025-01-01", 30),
			mkInput("100", 3),
		},
		emitFields: [emitFieldCount]textinput.Model{
			mkInput("", 0),                   // placeholder for type (handled by SearchSelect)
			mkInput("customer-1", 200),       // subject
			mkInput("my-service", 200),       // source
			mkInput(`{"tokens": 500}`, 2000), // data (JSON)
		},
	}
	m.simpleFields[0].Focus()
	m.buildEmitForm()
	return m
}

// ─── emit form ───────────────────────────────────────────────────────────────

func (m *eventsModel) buildEmitForm() {
	m.emitFocused = 0

	// Build SearchSelect for type field
	opts := m.buildTypeOptions()
	m.emitTypeSelect = NewSearchSelect("select event type…", opts)

	// Reset text fields
	for i := emitFieldSubject; i < emitFieldCount; i++ {
		m.emitFields[i].SetValue("")
		m.emitFields[i].Blur()
	}
}

func (m *eventsModel) buildTypeOptions() []SearchSelectOption {
	seen := map[string]bool{}
	var opts []SearchSelectOption
	for _, t := range m.meterEventTypes {
		if !seen[t] {
			opts = append(opts, SearchSelectOption{Label: t, Value: t})
			seen[t] = true
		}
	}
	return opts
}

func (m *eventsModel) loadMeterTypes() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		raw, err := client.ListMeters()
		if err != nil {
			return meterTypesLoadedMsg{err: err}
		}
		var meters []struct {
			EventType string `json:"eventType"`
		}
		if err := json.Unmarshal(raw, &meters); err != nil {
			return meterTypesLoadedMsg{err: fmt.Errorf("parse meters: %w", err)}
		}
		types := make([]string, 0, len(meters))
		for _, m := range meters {
			if m.EventType != "" {
				types = append(types, m.EventType)
			}
		}
		return meterTypesLoadedMsg{types: types}
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *eventsModel) Name() string { return "Events" }

func (m *eventsModel) Init() tea.Cmd {
	return tea.Batch(m.load(""), textinput.Blink, m.loadMeterTypes())
}

func (m *eventsModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case eventsLoadedMsg:
		if msg.err != nil {
			m.status = statusOK.Render("—")
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.t.SetRows(msg.rows)
		m.rawEvents = msg.rawEvents
		m.nextCursor = msg.nextCursor
		m.status = statusOK.Render(fmt.Sprintf(
			"%d events  —  %s", len(msg.rows), msg.at.Format("15:04:05"),
		))
		return m, nil

	case meterTypesLoadedMsg:
		if msg.err == nil && len(msg.types) > 0 {
			m.meterEventTypes = msg.types
			opts := m.buildTypeOptions()
			m.emitTypeSelect.SetOptions(opts)
		}
		return m, nil

	case eventEmittedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.status = statusGood.Render("Event emitted!")
		m.buildEmitForm()
		return m, m.load("")

	case tea.KeyMsg:
		switch m.mode {
		case eventModeList:
			return m.updateList(msg)
		case eventModeDetail:
			return m.updateDetail(msg)
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *eventsModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch m.focus {
	case focusTable:
		return m.updateTablePane(msg)
	case focusEmit:
		return m.updateEmitPane(msg)
	default:
		return m.updateFilterPane(msg)
	}
}

func (m *eventsModel) updateFilterPane(msg tea.KeyMsg) (Tab, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		m.status = statusWIP.Render("Searching…")
		m.nextCursor = ""
		return m, m.load("")

	case "ctrl+e":
		m.focus = focusEmit
		m.blurFilterFields()
		return m, textinput.Blink

	case "ctrl+a":
		m.advanced = !m.advanced
		if m.advanced {
			m.advFields[0].Focus()
			m.advFocused = 0
			for i := range m.simpleFields {
				m.simpleFields[i].Blur()
			}
		} else {
			m.simpleFields[0].Focus()
			m.simpleFocused = 0
			for i := range m.advFields {
				m.advFields[i].Blur()
			}
		}
		return m, textinput.Blink

	case "ctrl+d":
		return m, m.openDetail()

	case "right":
		m.switchToTable()
		return m, nil

	case "tab", "down":
		return m, m.moveFocus(1)

	case "shift+tab", "up":
		return m, m.moveFocus(-1)
	}

	// Forward to active text input
	if m.advanced {
		var cmd tea.Cmd
		m.advFields[m.advFocused], cmd = m.advFields[m.advFocused].Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.simpleFields[m.simpleFocused], cmd = m.simpleFields[m.simpleFocused].Update(msg)
	return m, cmd
}

func (m *eventsModel) updateEmitPane(msg tea.KeyMsg) (Tab, tea.Cmd) {
	// If the SearchSelect modal is open, forward everything to it.
	if m.emitTypeSelect.IsOpen() {
		ss, cmd := m.emitTypeSelect.Update(msg)
		m.emitTypeSelect = ss
		return m, cmd
	}

	key := msg.String()

	switch key {
	case "enter":
		emitCmd := m.submitEmit()
		m.buildEmitForm()
		return m, emitCmd

	case "esc":
		m.blurEmitFields()
		m.focus = focusFilter
		m.focusFilterField()
		return m, textinput.Blink

	case "ctrl+e":
		m.blurEmitFields()
		m.focus = focusFilter
		m.focusFilterField()
		return m, textinput.Blink

	case "right":
		m.blurEmitFields()
		m.switchToTable()
		return m, nil
	}

	// Focus on SearchSelect (type field).
	if m.emitFocused == emitFieldType {
		switch key {
		case "tab", "down":
			m.emitFocused = emitFieldSubject
			m.emitFields[emitFieldSubject].Focus()
			return m, textinput.Blink
		default:
			ss, cmd := m.emitTypeSelect.Update(msg)
			m.emitTypeSelect = ss
			// If the modal just opened, show it as a form overlay.
			if ss.IsOpen() {
				return m, tea.Batch(cmd, func() tea.Msg {
					return showFormMsg{
						maxWidth: 50,
						view: func(width int) string {
							return m.emitTypeSelect.ModalView()
						},
						update: func(msg tea.Msg) (bool, tea.Cmd) {
							ss, cmd := m.emitTypeSelect.Update(msg)
							m.emitTypeSelect = ss
							return !ss.IsOpen(), cmd
						},
					}
				})
			}
			return m, cmd
		}
	}

	// Text field navigation.
	switch key {
	case "tab", "down":
		m.emitFields[m.emitFocused].Blur()
		m.emitFocused++
		if m.emitFocused >= emitFieldCount {
			m.emitFocused = emitFieldCount - 1
		}
		m.emitFields[m.emitFocused].Focus()
		return m, textinput.Blink

	case "shift+tab", "up":
		m.emitFields[m.emitFocused].Blur()
		m.emitFocused--
		if m.emitFocused < emitFieldType {
			m.emitFocused = emitFieldType
		}
		if m.emitFocused == emitFieldType {
			// Back to SearchSelect row — no textinput to focus.
			return m, nil
		}
		m.emitFields[m.emitFocused].Focus()
		return m, textinput.Blink
	}

	// Forward to active text input.
	var cmd tea.Cmd
	m.emitFields[m.emitFocused], cmd = m.emitFields[m.emitFocused].Update(msg)
	return m, cmd
}

func (m *eventsModel) updateTablePane(msg tea.KeyMsg) (Tab, tea.Cmd) {
	// If the table is in filtering mode, forward to table.
	if m.t.filtering {
		cmd := m.t.Update(msg)
		return m, cmd
	}

	key := msg.String()
	switch key {
	case "enter", "d":
		return m, m.openDetail()

	case "e":
		m.focus = focusEmit
		m.t.SetFocused(false)
		return m, textinput.Blink

	case "left", "h":
		m.focus = focusFilter
		m.t.SetFocused(false)
		m.focusFilterField()
		return m, textinput.Blink

	case "R":
		m.status = statusWIP.Render("Refreshing…")
		m.nextCursor = ""
		return m, m.load("")

	case "alt+n":
		if m.advanced && m.nextCursor != "" {
			m.status = statusWIP.Render("Next page…")
			return m, m.load(m.nextCursor)
		}
	}

	// Forward navigation keys (j/k/up/down/pgup/pgdown/home/end//) to table
	cmd := m.t.Update(msg)
	return m, cmd
}

func (m *eventsModel) blurEmitFields() {
	for i := emitFieldSubject; i < emitFieldCount; i++ {
		m.emitFields[i].Blur()
	}
}

func (m *eventsModel) blurFilterFields() {
	for i := range m.simpleFields {
		m.simpleFields[i].Blur()
	}
	for i := range m.advFields {
		m.advFields[i].Blur()
	}
}

func (m *eventsModel) focusFilterField() {
	if m.advanced {
		m.advFields[m.advFocused].Focus()
	} else {
		m.simpleFields[m.simpleFocused].Focus()
	}
}

func (m *eventsModel) switchToTable() {
	m.focus = focusTable
	m.blurFilterFields()
	m.blurEmitFields()
	m.t.SetFocused(true)
}

func (m *eventsModel) openDetail() tea.Cmd {
	idx := m.t.selectedRealIndex()
	if idx >= 0 && idx < len(m.rawEvents) {
		m.mode = eventModeDetail
		m.detailScroll = 0
		var pretty json.RawMessage
		if err := json.Unmarshal(m.rawEvents[idx], &pretty); err == nil {
			indented, _ := json.MarshalIndent(pretty, "", "  ")
			m.detailLines = strings.Split(string(indented), "\n")
		} else {
			m.detailLines = strings.Split(string(m.rawEvents[idx]), "\n")
		}
	}
	return nil
}

func (m *eventsModel) moveFocus(dir int) tea.Cmd {
	if m.advanced {
		m.advFields[m.advFocused].Blur()
		m.advFocused = (m.advFocused + dir + advCount) % advCount
		m.advFields[m.advFocused].Focus()
	} else {
		m.simpleFields[m.simpleFocused].Blur()
		m.simpleFocused = (m.simpleFocused + dir + simpleCount) % simpleCount
		m.simpleFields[m.simpleFocused].Focus()
	}
	return textinput.Blink
}

func (m *eventsModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "backspace", "esc":
		m.mode = eventModeList
		return m, textinput.Blink
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

// ─── View ────────────────────────────────────────────────────────────────────

func (m *eventsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.mode {
	case eventModeDetail:
		return m.viewDetail()
	default:
		return m.viewList()
	}
}

func (m *eventsModel) viewList() string {
	leftW := 48
	gap := 1
	rightW := m.width - leftW - gap
	if rightW < 30 {
		rightW = 30
		leftW = m.width - rightW - gap
	}

	boxFor := func(focused bool, w int) lipgloss.Style {
		col := colBorder
		if focused {
			col = colAccent
		}
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(col).
			Width(w).
			Padding(0, 1)
	}

	// ── left pane ──
	innerW := leftW - 4 // account for border + padding
	filterSection := m.viewFilterSection(innerW)
	emitSection := m.viewEmitSection(innerW)

	filterFocused := m.focus == focusFilter
	emitFocused := m.focus == focusEmit

	filterSection = borderLabel(
		boxFor(filterFocused, leftW-2).Render(filterSection),
		"Filter", boolColor(filterFocused),
	)
	emitLabel := "Emit"
	if !emitFocused {
		emitLabel = "C-e Emit"
	}
	emitSection = borderLabel(
		boxFor(emitFocused, leftW-2).Render(emitSection),
		emitLabel, boolColor(emitFocused),
	)

	leftContent := filterSection + "\n" + emitSection

	leftPane := lipgloss.NewStyle().
		Width(leftW).
		Height(m.height).
		Render(leftContent)

	// ── right pane (table) ──
	tableFocused := m.focus == focusTable
	m.t.SetFocused(tableFocused)

	tableBoxOverhead := 2 // border top + bottom
	tableH := m.height - tableBoxOverhead
	if tableH < 3 {
		tableH = 3
	}
	tableInnerW := rightW - 4 // border + padding
	m.t.SetSize(tableInnerW, tableH)

	tableBox := boxFor(tableFocused, rightW-2).Render(m.t.View())
	tableLabel := "Table"
	if !tableFocused {
		tableLabel = "→ Table"
	}
	rightPane := borderLabel(tableBox, tableLabel, boolColor(tableFocused))

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
}

func boolColor(active bool) lipgloss.Color {
	if active {
		return colAccent
	}
	return colBorder
}

func (m *eventsModel) viewFilterSection(w int) string {
	fieldW := w - 4
	if fieldW < 20 {
		fieldW = 20
	}
	inputW := fieldW - 12 // label width + marker
	if inputW < 8 {
		inputW = 8
	}

	for i := range m.simpleFields {
		m.simpleFields[i].Width = inputW
	}
	for i := range m.advFields {
		m.advFields[i].Width = inputW
	}

	active := m.focus == focusFilter
	var b strings.Builder

	modeLabel := descStyle.Render("Simple (v1)")
	if m.advanced {
		modeLabel = descStyle.Render("Advanced (v2)")
	}
	b.WriteString(modeLabel + "\n")

	if m.advanced {
		fields := []struct {
			label string
			idx   int
		}{
			{"ID:", advID},
			{"Subject:", advSubject},
			{"Type:", advType},
			{"Source:", advSource},
			{"Customer:", advCustomerID},
			{"From:", advFrom},
			{"To:", advTo},
			{"Ing From:", advIngestedFrom},
			{"Ing To:", advIngestedTo},
			{"Limit:", advLimit},
		}
		for _, f := range fields {
			focused := active && m.advFocused == f.idx
			b.WriteString(formField(f.label, m.advFields[f.idx].View(), focused, fieldW) + "\n")
		}
	} else {
		fields := []struct {
			label string
			idx   int
		}{
			{"ID:", simpleID},
			{"Subject:", simpleSubject},
			{"Customer:", simpleCustomerID},
			{"From:", simpleFrom},
			{"To:", simpleTo},
			{"Limit:", simpleLimit},
		}
		for _, f := range fields {
			focused := active && m.simpleFocused == f.idx
			b.WriteString(formField(f.label, m.simpleFields[f.idx].View(), focused, fieldW) + "\n")
		}
	}

	return b.String()
}

func (m *eventsModel) viewEmitSection(w int) string {
	fieldW := w - 4
	if fieldW < 20 {
		fieldW = 20
	}
	inputW := fieldW - 12
	if inputW < 8 {
		inputW = 8
	}

	for i := emitFieldSubject; i < emitFieldCount; i++ {
		m.emitFields[i].Width = inputW
	}

	active := m.focus == focusEmit
	var b strings.Builder

	fields := []struct {
		label string
		idx   int
	}{
		{"Type:", emitFieldType},
		{"Subject:", emitFieldSubject},
		{"Source:", emitFieldSource},
		{"Data:", emitFieldData},
	}
	for _, f := range fields {
		focused := active && m.emitFocused == f.idx
		var content string
		if f.idx == emitFieldType {
			content = m.emitTypeSelect.View()
		} else {
			content = m.emitFields[f.idx].View()
		}
		b.WriteString(formField(f.label, content, focused, fieldW) + "\n")
	}

	return b.String()
}

func (m *eventsModel) viewDetail() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Event Detail") + "\n\n")

	visibleH := m.height - 4
	if visibleH < 1 {
		visibleH = 1
	}
	end := m.detailScroll + visibleH
	if end > len(m.detailLines) {
		end = len(m.detailLines)
	}
	start := m.detailScroll
	if start > len(m.detailLines) {
		start = len(m.detailLines)
	}

	for _, line := range m.detailLines[start:end] {
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + descStyle.Render("↑↓/jk scroll  •  backspace back"))
	return b.String()
}

func (m *eventsModel) SetSize(w, h int) {
	m.width, m.height = w, h
}

func (m *eventsModel) Tip() string {
	switch m.focus {
	case focusEmit:
		return tipStyle.Render("Fill required fields and confirm to emit a CloudEvent")
	case focusTable:
		return tipStyle.Render("Navigate events • Enter or d to view detail")
	default:
		if m.advanced {
			return tipStyle.Render("v2 filter supports %, time ranges, and cursor pagination")
		}
		return tipStyle.Render("Enter to search • C-e to switch to emit form")
	}
}

func (m *eventsModel) Status() string { return m.status }

func (m *eventsModel) Hints() []KeyHint {
	switch m.mode {
	case eventModeDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"Bksp", "back"},
		}
	default:
		switch m.focus {
		case focusFilter:
			hints := []KeyHint{
				{"C-e", "emit"},
				{"C-a", "mode"},
				{"C-d", "detail"},
				{"→", "table"},
				{"Enter", "search"},
			}
			if m.advanced && m.nextCursor != "" {
				hints = append(hints, KeyHint{"⌥n", "next page"})
			}
			return hints

		case focusEmit:
			return []KeyHint{
				{"/", "type"},
				{"Enter", "emit"},
				{"Esc", "back"},
				{"→", "table"},
			}

		case focusTable:
			hints := []KeyHint{
				{"Enter", "detail"},
				{"e", "emit"},
				{"←", "filter"},
				{"R", "refresh"},
				{"/", "search"},
			}
			if m.advanced && m.nextCursor != "" {
				hints = append(hints, KeyHint{"⌥n", "next page"})
			}
			return hints
		}
		return nil
	}
}

// ─── emit submission ─────────────────────────────────────────────────────────

func (m *eventsModel) submitEmit() tea.Cmd {
	evType := strings.TrimSpace(m.emitTypeSelect.Value())
	subject := strings.TrimSpace(m.emitFields[emitFieldSubject].Value())
	source := strings.TrimSpace(m.emitFields[emitFieldSource].Value())
	data := strings.TrimSpace(m.emitFields[emitFieldData].Value())

	if evType == "" || subject == "" || source == "" {
		return func() tea.Msg {
			return eventEmittedMsg{err: fmt.Errorf("type, subject, and source are required")}
		}
	}

	// Build CloudEvents JSON
	ev := map[string]any{
		"specversion": "1.0",
		"id":          newUUID(),
		"type":        evType,
		"source":      source,
		"subject":     subject,
		"time":        time.Now().UTC().Format(time.RFC3339),
	}
	if data != "" {
		ev["data"] = json.RawMessage(data)
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return func() tea.Msg {
			return eventEmittedMsg{err: fmt.Errorf("marshal: %w", err)}
		}
	}

	client := m.client
	return func() tea.Msg {
		err := client.IngestEvent(json.RawMessage(body))
		return eventEmittedMsg{err: err}
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *eventsModel) load(cursor string) tea.Cmd {
	if m.advanced {
		return m.loadV2(cursor)
	}
	return m.loadV1()
}

// loadV1 uses GET /api/v1/events with plain query params. Returns a plain array.
func (m *eventsModel) loadV1() tea.Cmd {
	id := strings.TrimSpace(m.simpleFields[simpleID].Value())
	subject := strings.TrimSpace(m.simpleFields[simpleSubject].Value())
	customerID := strings.TrimSpace(m.simpleFields[simpleCustomerID].Value())
	from := strings.TrimSpace(m.simpleFields[simpleFrom].Value())
	to := strings.TrimSpace(m.simpleFields[simpleTo].Value())
	limit := strings.TrimSpace(m.simpleFields[simpleLimit].Value())

	return func() tea.Msg {
		params := url.Values{}
		if limit != "" {
			params.Set("limit", limit)
		} else {
			params.Set("limit", "100")
		}
		if id != "" {
			params.Set("id", id)
		}
		if subject != "" {
			params.Set("subject", subject)
		}
		if customerID != "" {
			params.Add("customerId", customerID)
		}
		if t := normalizeTime(from); t != "" {
			params.Set("from", t)
		}
		if t := normalizeTime(to); t != "" {
			params.Set("to", t)
		}

		raw, err := m.client.ListEvents(params.Encode())
		if err != nil {
			return eventsLoadedMsg{err: err, at: time.Now()}
		}

		// v1 returns a plain array of IngestedEvent
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return eventsLoadedMsg{err: fmt.Errorf("parse: %w", err), at: time.Now()}
		}

		rows, rawEvents := parseEventItems(items)
		return eventsLoadedMsg{rows: rows, rawEvents: rawEvents, at: time.Now()}
	}
}

// loadV2 uses GET /api/v2/events with filter JSON + cursor pagination.
func (m *eventsModel) loadV2(cursor string) tea.Cmd {
	id := strings.TrimSpace(m.advFields[advID].Value())
	subject := strings.TrimSpace(m.advFields[advSubject].Value())
	evType := strings.TrimSpace(m.advFields[advType].Value())
	source := strings.TrimSpace(m.advFields[advSource].Value())
	customerID := strings.TrimSpace(m.advFields[advCustomerID].Value())
	from := strings.TrimSpace(m.advFields[advFrom].Value())
	to := strings.TrimSpace(m.advFields[advTo].Value())
	ingFrom := strings.TrimSpace(m.advFields[advIngestedFrom].Value())
	ingTo := strings.TrimSpace(m.advFields[advIngestedTo].Value())
	limit := strings.TrimSpace(m.advFields[advLimit].Value())

	return func() tea.Msg {
		params := url.Values{}
		if limit != "" {
			params.Set("limit", limit)
		} else {
			params.Set("limit", "100")
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}

		filter := map[string]any{}
		if id != "" {
			filter["id"] = map[string]string{"$eq": id}
		}
		if subject != "" {
			filter["subject"] = map[string]string{"$ilike": "%" + subject + "%"}
		}
		if evType != "" {
			filter["type"] = map[string]string{"$ilike": "%" + evType + "%"}
		}
		if source != "" {
			filter["source"] = map[string]string{"$ilike": "%" + source + "%"}
		}
		if customerID != "" {
			filter["customerId"] = map[string]any{"$in": []string{customerID}}
		}
		if timeF := buildTimeFilter(from, to); len(timeF) > 0 {
			filter["time"] = timeF
		}
		if ingF := buildTimeFilter(ingFrom, ingTo); len(ingF) > 0 {
			filter["ingestedAt"] = ingF
		}

		if len(filter) > 0 {
			filterJSON, _ := json.Marshal(filter)
			params.Set("filter", string(filterJSON))
		}

		raw, err := m.client.ListEventsV2(params.Encode())
		if err != nil {
			return eventsLoadedMsg{err: err, at: time.Now()}
		}

		var resp struct {
			Items      []json.RawMessage `json:"items"`
			NextCursor string            `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return eventsLoadedMsg{err: fmt.Errorf("parse: %w", err), at: time.Now()}
		}

		rows, rawEvents := parseEventItems(resp.Items)
		return eventsLoadedMsg{rows: rows, rawEvents: rawEvents, nextCursor: resp.NextCursor, at: time.Now()}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func parseEventItems(items []json.RawMessage) ([][]string, []json.RawMessage) {
	type ingestedEvent struct {
		Event struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Subject string `json:"subject"`
			Source  string `json:"source"`
			Time    string `json:"time"`
		} `json:"event"`
		IngestedAt string `json:"ingestedAt"`
	}

	rows := make([][]string, 0, len(items))
	rawEvents := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		var ev ingestedEvent
		if err := json.Unmarshal(item, &ev); err != nil {
			continue
		}
		rows = append(rows, []string{
			trunc(ev.Event.ID, 12),
			ev.Event.Type,
			ev.Event.Subject,
			ev.Event.Source,
			ev.Event.Time,
			ev.IngestedAt,
		})
		rawEvents = append(rawEvents, item)
	}
	return rows, rawEvents
}

func buildTimeFilter(from, to string) map[string]string {
	f := map[string]string{}
	if t := normalizeTime(from); t != "" {
		f["$gte"] = t
	}
	if t := normalizeTime(to); t != "" {
		f["$lte"] = t
	}
	return f
}

func normalizeTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s + "T00:00:00Z"
	}
	return ""
}

// newUUID generates a v4 UUID without external dependencies.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
