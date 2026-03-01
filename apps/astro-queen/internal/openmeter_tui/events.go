package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/postman/astro/apps/astro-queen/internal/openmeter"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type eventIngestedMsg struct {
	err error
	at  time.Time
	ev  ingestedEvent
}

type ingestedEvent struct {
	Type    string
	Subject string
	Data    string
	At      time.Time
}

// ─── model ────────────────────────────────────────────────────────────────────

type eventField int

const (
	eventFieldType eventField = iota
	eventFieldSubject
	eventFieldData
)

type eventsModel struct {
	client  *openmeter.Client
	fields  [3]textinput.Model // type, subject, data
	focused eventField
	status  string
	recent  []ingestedEvent
	width   int
	height  int
}

func newEventsModel(client *openmeter.Client) *eventsModel {
	typeInput := textinput.New()
	typeInput.Placeholder = "event type (e.g. api_call)"
	typeInput.CharLimit = 200
	typeInput.Focus()

	subjectInput := textinput.New()
	subjectInput.Placeholder = "subject (e.g. user-123)"
	subjectInput.CharLimit = 200

	dataInput := textinput.New()
	dataInput.Placeholder = `data JSON (e.g. {"method":"GET","path":"/api"})`
	dataInput.CharLimit = 1000

	return &eventsModel{
		client:  client,
		fields:  [3]textinput.Model{typeInput, subjectInput, dataInput},
		focused: eventFieldType,
		status:  descStyle.Render("Fill in fields and press Enter to ingest"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *eventsModel) Name() string { return "Events" }

func (m *eventsModel) Init() tea.Cmd { return textinput.Blink }

func (m *eventsModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case eventIngestedMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Ingest error: " + msg.err.Error())
		} else {
			m.status = statusGood.Render(fmt.Sprintf("✓ Ingested at %s", msg.at.Format("15:04:05")))
			m.recent = append([]ingestedEvent{msg.ev}, m.recent...)
			if len(m.recent) > 20 {
				m.recent = m.recent[:20]
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.fields[m.focused].Blur()
			m.focused = (m.focused + 1) % 3
			m.fields[m.focused].Focus()
			return m, textinput.Blink

		case "shift+tab":
			m.fields[m.focused].Blur()
			m.focused = (m.focused + 2) % 3 // -1 mod 3
			m.fields[m.focused].Focus()
			return m, textinput.Blink

		case "enter":
			return m, m.ingest()
		}
	}

	var cmd tea.Cmd
	m.fields[m.focused], cmd = m.fields[m.focused].Update(msg)
	return m, cmd
}

func (m *eventsModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}

	evLabel := labelStyle.Width(10)

	var b strings.Builder
	b.WriteString(tipStyle.Render("Ingest CloudEvents into OpenMeter. Type and subject are required; data is optional JSON payload.") + "\n\n")

	labels := []string{"Type:", "Subject:", "Data:"}
	for i, f := range m.fields {
		marker := "  "
		if eventField(i) == m.focused {
			marker = focusStyle.Render("▸ ")
		}
		b.WriteString(marker + evLabel.Render(labels[i]) + " " + f.View() + "\n")
	}

	b.WriteString("\n" + m.status + "\n")

	if len(m.recent) > 0 {
		b.WriteString("\n" + titleStyle.Render("Recent ingested events") + "\n")
		for i, ev := range m.recent {
			ts := ev.At.Format("15:04:05")
			line := fmt.Sprintf("  %s  type=%s  subject=%s  data=%s", ts, ev.Type, ev.Subject, trunc(ev.Data, 40))
			if i%2 == 0 {
				b.WriteString(dimStyle.Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}

	return b.String()
}

func (m *eventsModel) SetSize(w, h int) {
	m.width, m.height = w, h
	for i := range m.fields {
		m.fields[i].Width = w - 14
	}
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
	return []KeyHint{
		{"Tab", "next field"},
		{"Enter", "ingest"},
		{"Esc", "nav mode"},
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *eventsModel) ingest() tea.Cmd {
	evType := m.fields[eventFieldType].Value()
	subject := m.fields[eventFieldSubject].Value()
	data := m.fields[eventFieldData].Value()

	if evType == "" {
		m.status = statusErr.Render("Type is required")
		return nil
	}

	ev := ingestedEvent{
		Type:    evType,
		Subject: subject,
		Data:    data,
	}

	return func() tea.Msg {
		// Build CloudEvents-compatible event payload.
		event := map[string]interface{}{
			"specversion": "1.0",
			"type":        evType,
			"source":      "astro-queen",
			"id":          fmt.Sprintf("queen-%d", time.Now().UnixNano()),
			"time":        time.Now().UTC().Format(time.RFC3339),
			"subject":     subject,
		}
		if data != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err == nil {
				event["data"] = parsed
			} else {
				event["data"] = data
			}
		}

		body, _ := json.Marshal(event)
		err := m.client.IngestEvents(body)
		now := time.Now()
		ev.At = now
		return eventIngestedMsg{err: err, at: now, ev: ev}
	}
}
