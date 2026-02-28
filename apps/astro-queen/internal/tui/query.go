package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type queryResultMsg struct {
	resp *adminv1.QueryDatabaseResponse
	err  error
}

type schemaLoadedMsg struct {
	tables map[string]map[string]string // table_name → (column_name → data_type)
}

// ─── focus constants ──────────────────────────────────────────────────────────

type queryFocus int

const (
	focusInput queryFocus = iota
	focusTable
	focusDetail
)

// ─── model ────────────────────────────────────────────────────────────────────

type queryModel struct {
	client       adminv1.AdminServiceClient
	input        textinput.Model
	t            tableModel
	focus        queryFocus
	status       string
	width        int
	height       int
	detailScroll int // scroll offset in detail view

	// Schema cache: table_name → (column_name → data_type).
	schemaCache map[string]map[string]string
	// Column types for the current result set (aligned with t.headers).
	colTypes  []string
	hydrating bool // true while prefetching schema
}

func newQueryModel(client adminv1.AdminServiceClient) *queryModel {
	ti := textinput.New()
	ti.Placeholder = "SELECT * FROM deployments LIMIT 10"
	ti.CharLimit = 500
	ti.Width = 60

	t := newTableModel(nil)

	return &queryModel{
		client:      client,
		input:       ti,
		t:           t,
		focus:       focusInput,
		status:      descStyle.Render("Type a query and press Enter to run it."),
		schemaCache: make(map[string]map[string]string),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *queryModel) Name() string { return "Query" }

func (m *queryModel) Init() tea.Cmd {
	m.focus = focusInput
	m.input.Focus()
	m.hydrating = true
	return tea.Batch(textinput.Blink, m.prefetchSchema())
}

// prefetchSchema loads column types for all tables on init via GetSchema RPC.
func (m *queryModel) prefetchSchema() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetSchema(context.Background(), &adminv1.GetSchemaRequest{})
		if err != nil {
			return schemaLoadedMsg{} // silently ignore — schema is optional
		}
		tables := make(map[string]map[string]string)
		for _, col := range resp.Columns {
			if tables[col.TableName] == nil {
				tables[col.TableName] = make(map[string]string)
			}
			tables[col.TableName][col.ColumnName] = col.DataType
		}
		return schemaLoadedMsg{tables: tables}
	}
}

func (m *queryModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case schemaLoadedMsg:
		m.hydrating = false
		for tbl, types := range msg.tables {
			m.schemaCache[tbl] = types
		}
		if len(m.t.headers) > 0 {
			m.colTypes = m.resolveColTypes()
			m.t.colTypes = m.colTypes
		}
		return m, nil

	case queryResultMsg:
		if msg.err != nil {
			m.status = statusErr.Render("Error: " + msg.err.Error())
			return m, nil
		}
		resp := msg.resp
		rows := make([][]string, len(resp.Rows))
		for i, row := range resp.Rows {
			rows[i] = row.Values
		}
		m.t.headers = resp.Columns
		m.t.SetRows(rows)
		m.t.SetFocused(true)
		m.colTypes = m.resolveColTypes()
		m.t.colTypes = m.colTypes
		m.status = statusOK.Render(fmt.Sprintf("%d rows, %d columns", len(rows), len(resp.Columns)))
		m.focus = focusTable
		m.input.Blur()
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		// Detail view: scrollable key-value list for the selected row.
		if m.focus == focusDetail {
			switch key {
			case "enter", "backspace":
				m.focus = focusTable
				m.t.SetFocused(true)
				return m, nil
			case "up", "k":
				if m.detailScroll > 0 {
					m.detailScroll--
				}
				return m, nil
			case "down", "j":
				m.detailScroll++
				return m, nil
			case "home":
				m.detailScroll = 0
				return m, nil
			}
			return m, nil
		}

		switch key {
		case "tab":
			if m.focus == focusInput {
				m.focus = focusTable
				m.input.Blur()
				m.t.SetFocused(true)
				return m, nil
			}
			m.focus = focusInput
			m.input.Focus()
			m.t.SetFocused(false)
			return m, textinput.Blink
		case "i":
			if m.focus == focusTable {
				m.focus = focusInput
				m.input.Focus()
				m.t.SetFocused(false)
				return m, textinput.Blink
			}
		case "enter":
			if m.focus == focusInput {
				m.status = statusWIP.Render("Running…")
				return m, m.runQuery()
			}
			if m.focus == focusTable && m.t.SelectedRow() != nil {
				m.focus = focusDetail
				m.detailScroll = 0
				m.t.SetFocused(false)
				return m, nil
			}
		}

		// When input is focused, forward all other keys to the text input.
		if m.focus == focusInput {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	// Table navigation.
	if m.focus == focusTable {
		cmd := m.t.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *queryModel) View(w, h int) string {
	// Detail view replaces everything.
	if m.focus == focusDetail {
		return m.renderDetail(w, h)
	}

	var inputHeader string
	if m.focus == focusInput {
		inputHeader = activeTabStyle.Render("SQL Query") +
			descStyle.Render("  Enter to run  •  Tab to results")
	} else {
		inputHeader = inactiveTabStyle.Render("SQL Query") +
			descStyle.Render("  i to edit  •  Tab to switch")
	}
	inputHeader = barBg.Width(w).Render(inputHeader)

	inputRow := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(w).
		Render(m.input.View())

	var resultsHeader string
	if m.focus == focusTable {
		resultsHeader = barBg.Width(w).Render(
			activeTabStyle.Render("Results") + "  " + m.status,
		)
	} else {
		resultsHeader = barBg.Width(w).Render(
			inactiveTabStyle.Render("Results") + "  " + m.status,
		)
	}

	var tableSection string
	if m.focus == focusTable {
		tableSection = m.t.View()
	} else {
		tableSection = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(m.t.View())
	}

	return strings.Join([]string{
		inputHeader,
		inputRow,
		resultsHeader,
		tableSection,
	}, "\n")
}

func (m *queryModel) renderDetail(w, h int) string {
	row := m.t.SelectedRow()
	if row == nil {
		return ""
	}

	headerLine := activeTabStyle.Render("Row Detail") + "  " +
		descStyle.Render(fmt.Sprintf("row %d  •  Enter/Backspace to go back  •  ↑↓ to scroll", m.t.selected+1))
	header := barBg.Width(w).Render(headerLine)

	// Build key-value lines with type-aware rendering.
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	uuidStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	boolTrue := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	boolFalse := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	var lines []string
	for i, col := range m.t.headers {
		val := ""
		if i < len(row) {
			val = row[i]
		}

		// Get column type.
		colType := "text"
		if i < len(m.colTypes) {
			colType = m.colTypes[i]
		}

		typeTag := dimStyle.Render(" (" + colType + ")")
		label := labelStyle.Render(col+":") + typeTag

		// Render based on type.
		switch colType {
		case "jsonb", "json":
			trimmed := strings.TrimSpace(val)
			var raw json.RawMessage
			if json.Unmarshal([]byte(trimmed), &raw) == nil {
				pretty, err := json.MarshalIndent(raw, "", "  ")
				if err == nil {
					highlighted := highlightJSON(string(pretty))
					lines = append(lines, "  "+label)
					for _, jl := range strings.Split(highlighted, "\n") {
						lines = append(lines, "    "+jl)
					}
					continue
				}
			}
		case "uuid":
			lines = append(lines, "  "+label+" "+uuidStyle.Render(val))
			continue
		case "timestamp", "timestamp with time zone", "timestamp without time zone", "timestamptz":
			lines = append(lines, "  "+label+" "+tsStyle.Render(val))
			continue
		case "boolean":
			if val == "true" {
				lines = append(lines, "  "+label+" "+boolTrue.Render("true"))
			} else {
				lines = append(lines, "  "+label+" "+boolFalse.Render("false"))
			}
			continue
		}

		// Default: wrap long lines.
		maxValWidth := w - len(col) - 4
		if maxValWidth < 20 {
			maxValWidth = 20
		}
		if len(val) <= maxValWidth {
			lines = append(lines, "  "+label+" "+valStyle.Render(val))
		} else {
			lines = append(lines, "  "+label)
			for len(val) > 0 {
				end := min(len(val), w-6)
				lines = append(lines, "    "+valStyle.Render(val[:end]))
				val = val[end:]
			}
		}
	}

	// Apply scroll offset and visible height.
	bodyHeight := h - 1 // header takes 1 line
	if m.detailScroll > len(lines)-bodyHeight {
		m.detailScroll = max(0, len(lines)-bodyHeight)
	}
	start := m.detailScroll
	end := min(start+bodyHeight, len(lines))
	visible := lines[start:end]

	return header + "\n" + strings.Join(visible, "\n")
}

func (m *queryModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.input.Width = w - 10
	// Reserve 3 lines for input header, input row, results header.
	tableH := h - 3
	if tableH < 4 {
		tableH = 4
	}
	m.t.SetSize(w, tableH)
}

func (m *queryModel) Status() string {
	if m.hydrating {
		return m.status + "  " + statusWIP.Render("⟳ hydrating schema…")
	}
	return m.status
}

func (m *queryModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case focusInput:
		return []KeyHint{
			{"Enter", "run query"},
			{"Tab", "results"},
			{"Esc", "nav mode"},
		}
	case focusDetail:
		return []KeyHint{
			{"↑↓/jk", "scroll"},
			{"Enter/⌫", "back to table"},
			{"Esc", "nav mode"},
		}
	default:
		return []KeyHint{
			{"Enter", "view row"},
			{"i", "edit query"},
			{"↑↓/jk", "navigate"},
			{"Tab", "input"},
			{"Esc", "nav mode"},
		}
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

// highlightJSON uses chroma to syntax-highlight a JSON string for the terminal.
func highlightJSON(src string) string {
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, src, "json", "terminal256", "monokai"); err != nil {
		return src
	}
	// chroma adds a trailing newline — trim it.
	return strings.TrimRight(buf.String(), "\n")
}

func (m *queryModel) runQuery() tea.Cmd {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		return nil
	}
	return func() tea.Msg {
		resp, err := m.client.QueryDatabase(context.Background(), &adminv1.QueryDatabaseRequest{Query: q})
		if err != nil {
			return queryResultMsg{err: err}
		}
		return queryResultMsg{resp: resp}
	}
}

// resolveColTypes builds colTypes from the schema cache, falling back to value inference.
func (m *queryModel) resolveColTypes() []string {
	types := make([]string, len(m.t.headers))

	// Try to find types from cache.
	for i, col := range m.t.headers {
		for _, schema := range m.schemaCache {
			if t, ok := schema[col]; ok {
				types[i] = t
				break
			}
		}
	}

	// For text/varchar/unknown, infer from first non-empty value.
	for i, t := range types {
		if isTextType(t) {
			types[i] = inferTypeFromValues(m.t.rows, i)
		}
	}

	return types
}

func isTextType(t string) bool {
	switch t {
	case "", "text", "character varying", "varchar", "USER-DEFINED":
		return true
	}
	return false
}

func inferTypeFromValues(rows [][]string, col int) string {
	for _, row := range rows {
		if col >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[col])
		if v == "" || v == "NULL" {
			continue
		}
		// UUID: 8-4-4-4-12 hex pattern.
		if len(v) == 36 && v[8] == '-' && v[13] == '-' && v[18] == '-' && v[23] == '-' {
			return "uuid"
		}
		// JSON object or array.
		if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
			var raw json.RawMessage
			if json.Unmarshal([]byte(v), &raw) == nil {
				return "jsonb"
			}
		}
		// Timestamp-like: starts with 4 digits and contains T or space + time.
		if len(v) >= 19 && v[4] == '-' && v[7] == '-' {
			return "timestamp"
		}
		// Boolean.
		if v == "true" || v == "false" {
			return "boolean"
		}
		return "text"
	}
	return "text"
}
