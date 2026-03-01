package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	}
	m.simpleFields[0].Focus()
	return m
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *eventsModel) Name() string { return "Events" }

func (m *eventsModel) Init() tea.Cmd {
	return tea.Batch(m.load(""), textinput.Blink)
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
	key := msg.String()

	switch key {
	case "enter":
		m.status = statusWIP.Render("Searching…")
		m.nextCursor = ""
		return m, m.load("")

	case "ctrl+a":
		// Toggle simple/advanced
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

	case "tab", "down":
		return m, m.moveFocus(1)

	case "shift+tab", "up":
		return m, m.moveFocus(-1)

	case "ctrl+n":
		if m.advanced && m.nextCursor != "" {
			m.status = statusWIP.Render("Next page…")
			return m, m.load(m.nextCursor)
		}

	case "ctrl+d":
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
			return m, nil
		}
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
	fieldW := 37
	timeW := 37

	for i := range m.simpleFields {
		m.simpleFields[i].Width = 24
	}
	for i := range m.advFields {
		m.advFields[i].Width = 24
	}

	var b strings.Builder

	// Mode indicator
	modeLabel := descStyle.Render("Simple (v1)")
	if m.advanced {
		modeLabel = descStyle.Render("Advanced (v2)")
	}
	b.WriteString(titleStyle.Render("Events") + "  " + modeLabel + "  " + dimStyle.Render("C-a toggle") + "\n")

	if m.advanced {
		// Advanced: 3 rows
		row1 := formLine(
			formField("ID:", m.advFields[advID].View(), m.advFocused == advID, fieldW),
			formField("Subject:", m.advFields[advSubject].View(), m.advFocused == advSubject, fieldW),
			formField("Type:", m.advFields[advType].View(), m.advFocused == advType, fieldW),
		)
		row2 := formLine(
			formField("Source:", m.advFields[advSource].View(), m.advFocused == advSource, fieldW),
			formField("Customer:", m.advFields[advCustomerID].View(), m.advFocused == advCustomerID, fieldW),
		)
		row3 := formLine(
			formField("From:", m.advFields[advFrom].View(), m.advFocused == advFrom, timeW),
			formField("To:", m.advFields[advTo].View(), m.advFocused == advTo, timeW),
			formField("Ing From:", m.advFields[advIngestedFrom].View(), m.advFocused == advIngestedFrom, timeW),
		)
		m.advFields[advLimit].Width = 4
		row4 := formLine(
			formField("Ing To:", m.advFields[advIngestedTo].View(), m.advFocused == advIngestedTo, timeW),
			formField("Limit:", m.advFields[advLimit].View(), m.advFocused == advLimit, 18),
		)
		b.WriteString(row1 + "\n")
		b.WriteString(row2 + "\n")
		b.WriteString(row3 + "\n")
		b.WriteString(row4 + "\n")
	} else {
		// Simple: 2 rows
		row1 := formLine(
			formField("ID:", m.simpleFields[simpleID].View(), m.simpleFocused == simpleID, fieldW),
			formField("Subject:", m.simpleFields[simpleSubject].View(), m.simpleFocused == simpleSubject, fieldW),
			formField("Customer:", m.simpleFields[simpleCustomerID].View(), m.simpleFocused == simpleCustomerID, fieldW),
		)
		m.simpleFields[simpleLimit].Width = 4
		row2 := formLine(
			formField("From:", m.simpleFields[simpleFrom].View(), m.simpleFocused == simpleFrom, timeW),
			formField("To:", m.simpleFields[simpleTo].View(), m.simpleFocused == simpleTo, timeW),
			formField("Limit:", m.simpleFields[simpleLimit].View(), m.simpleFocused == simpleLimit, 18),
		)
		b.WriteString(row1 + "\n")
		b.WriteString(row2 + "\n")
	}

	b.WriteString(formSeparator(m.width) + "\n")

	if m.status != "" {
		b.WriteString(m.status + "\n")
	}

	formLines := strings.Count(b.String(), "\n")
	tableH := m.height - formLines
	if tableH < 3 {
		tableH = 3
	}
	m.t.SetSize(m.width, tableH)
	b.WriteString(m.t.View())

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

func (m *eventsModel) Status() string { return m.status }

func (m *eventsModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.mode {
	case eventModeDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"Bksp", "back"},
		}
	default:
		hints := []KeyHint{
			{"↑↓/Tab", "fields"},
			{"Enter", "search"},
			{"C-a", "simple/adv"},
			{"C-d", "detail"},
		}
		if m.advanced && m.nextCursor != "" {
			hints = append(hints, KeyHint{"C-n", "next page"})
		}
		hints = append(hints, KeyHint{"Esc", "nav mode"})
		return hints
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
