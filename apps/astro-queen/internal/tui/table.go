package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── interactive table ──────────────────────────────────────────────────────

type tableModel struct {
	headers  []string
	colTypes []string // optional: aligned with headers (e.g. "uuid", "jsonb", "timestamp")
	rows     [][]string
	selected int
	offset   int
	height   int // visible data rows
	width    int
	focused  bool

	// filter / search
	filtering bool   // whether filter input is active
	filter    string // current filter text
	filtered  []int  // indices into rows that match (nil = show all)
}

func newTableModel(headers []string) tableModel {
	return tableModel{
		headers: headers,
		height:  10,
	}
}

func (m *tableModel) SetRows(rows [][]string) {
	m.rows = rows
	m.applyFilter()
	m.clampSelected()
	m.clampOffset()
}

func (m *tableModel) SetSize(w, h int) {
	m.width = w
	// Subtract 1 for the header row.
	m.height = h - 1
	if m.height < 1 {
		m.height = 1
	}
	m.clampOffset()
}

func (m *tableModel) SetFocused(focused bool) {
	m.focused = focused
}

// rowCount returns the number of visible rows (filtered or total).
func (m *tableModel) rowCount() int {
	if m.filtered != nil {
		return len(m.filtered)
	}
	return len(m.rows)
}

func (m *tableModel) SelectedRow() []string {
	rc := m.rowCount()
	if rc == 0 || m.selected < 0 || m.selected >= rc {
		return nil
	}
	idx := m.selected
	if m.filtered != nil {
		idx = m.filtered[m.selected]
	}
	return m.rows[idx]
}

// selectedRealIndex returns the real index into m.rows for the current selection.
func (m *tableModel) selectedRealIndex() int {
	rc := m.rowCount()
	if rc == 0 || m.selected < 0 || m.selected >= rc {
		return -1
	}
	if m.filtered != nil {
		return m.filtered[m.selected]
	}
	return m.selected
}

func (m *tableModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}

		switch msg.String() {
		case "/":
			m.filtering = true
			return nil
		}

		// Navigation operates over rowCount().
		rc := m.rowCount()
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				if m.selected < m.offset {
					m.offset = m.selected
				}
			}
		case "down", "j":
			if m.selected < rc-1 {
				m.selected++
				if m.selected >= m.offset+m.visibleHeight() {
					m.offset = m.selected - m.visibleHeight() + 1
				}
			}
		case "pgup":
			m.selected -= m.visibleHeight()
			if m.selected < 0 {
				m.selected = 0
			}
			m.clampOffset()
		case "pgdown":
			m.selected += m.visibleHeight()
			if m.selected >= rc {
				m.selected = max(0, rc-1)
			}
			m.clampOffset()
		case "home":
			m.selected = 0
			m.offset = 0
		case "end":
			m.selected = max(0, rc-1)
			m.clampOffset()
		}
	case tea.MouseMsg:
		rc := m.rowCount()
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
			if m.selected > 0 {
				m.selected--
				if m.selected < m.offset {
					m.offset = m.selected
				}
			}
		} else if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown {
			if m.selected < rc-1 {
				m.selected++
				if m.selected >= m.offset+m.visibleHeight() {
					m.offset = m.selected - m.visibleHeight() + 1
				}
			}
		}
	}
	return nil
}

func (m *tableModel) updateFiltering(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	switch key {
	case "enter":
		m.filtering = false
		return nil
	case "esc":
		m.filtering = false
		m.filter = ""
		m.filtered = nil
		m.clampSelected()
		m.clampOffset()
		return nil
	case "backspace":
		if len(m.filter) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.applyFilter()
			m.selected = 0
			m.offset = 0
		}
		return nil
	}

	// Accept printable runes (single char key strings from bubbletea).
	if len(key) == 1 || (len(msg.Runes) > 0 && key != "ctrl+c") {
		for _, r := range msg.Runes {
			m.filter += string(r)
		}
		if len(msg.Runes) == 0 {
			m.filter += key
		}
		m.applyFilter()
		m.selected = 0
		m.offset = 0
		return nil
	}

	return nil
}

func (m *tableModel) applyFilter() {
	if m.filter == "" {
		m.filtered = nil
		return
	}
	lower := strings.ToLower(m.filter)
	var matched []int
	for i, row := range m.rows {
		for _, col := range row {
			if strings.Contains(strings.ToLower(col), lower) {
				matched = append(matched, i)
				break
			}
		}
	}
	m.filtered = matched
}

func (m *tableModel) clampSelected() {
	rc := m.rowCount()
	if rc == 0 {
		m.selected = 0
	} else if m.selected >= rc {
		m.selected = rc - 1
	}
}

// visibleHeight returns data rows available for display (accounts for filter bar).
func (m *tableModel) visibleHeight() int {
	h := m.height
	if m.filter != "" || m.filtering {
		h-- // filter bar takes 1 row
	}
	if h < 1 {
		h = 1
	}
	return h
}

// fitCell truncates or pads a string to exactly width characters.
// For UUID columns, shows only the first 8 characters.
func fitCell(s string, width int, colType string) string {
	// Strip newlines — JSON values can contain them.
	s = strings.ReplaceAll(s, "\n", " ")

	// UUID: show short form (first 8 chars + "…").
	if colType == "uuid" && len(s) == 36 && s[8] == '-' {
		s = s[:8] + "…"
	}

	r := []rune(s)
	if len(r) > width {
		if width > 1 {
			return string(r[:width-1]) + "…"
		}
		return string(r[:width])
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}

var filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

func (m *tableModel) View() string {
	if m.width == 0 || len(m.headers) == 0 {
		return ""
	}

	colWidths := m.columnWidths()

	var b strings.Builder

	// Header row.
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	for i, h := range m.headers {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(headerStyle.Render(fitCell(h, colWidths[i], "")))
	}

	// Filter bar (between header and data).
	if m.filter != "" || m.filtering {
		b.WriteByte('\n')
		cursor := ""
		if m.filtering {
			cursor = "█"
		}
		rc := m.rowCount()
		bar := fmt.Sprintf("/ %s%s  (%d matches)", m.filter, cursor, rc)
		b.WriteString(filterStyle.Render(bar))
	}

	// Data rows.
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("220")).Bold(true)

	visRows := m.visibleRows()
	selectedIdx := m.selected - m.offset

	for ri, row := range visRows {
		b.WriteByte('\n')
		var style lipgloss.Style
		switch {
		case ri == selectedIdx && m.focused:
			style = selectedStyle
		case ri%2 == 0:
			style = dimStyle
		default:
			style = normalStyle
		}

		for ci := range m.headers {
			if ci > 0 {
				b.WriteString(style.Render("  "))
			}
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			ct := ""
			if ci < len(m.colTypes) {
				ct = m.colTypes[ci]
			}
			b.WriteString(style.Render(fitCell(val, colWidths[ci], ct)))
		}

		// For selected row, fill remaining width with background.
		if ri == selectedIdx && m.focused {
			rendered := b.String()
			lastNewline := strings.LastIndexByte(rendered, '\n')
			var lineLen int
			if lastNewline >= 0 {
				lineLen = lipgloss.Width(rendered[lastNewline+1:])
			} else {
				lineLen = lipgloss.Width(rendered)
			}
			if pad := m.width - lineLen; pad > 0 {
				b.WriteString(style.Render(strings.Repeat(" ", pad)))
			}
		}
	}

	return b.String()
}

const maxColWidth = 30

// columnWidths sizes columns to fit their content (capped at maxColWidth),
// then distributes leftover space.
func (m *tableModel) columnWidths() []int {
	n := len(m.headers)
	gap := 2 * (n - 1) // 2-char gap between columns
	avail := m.width - gap
	if avail < n {
		avail = n
	}

	// Measure natural width of each column (max of header + all data), capped.
	natural := make([]int, n)
	for i, h := range m.headers {
		natural[i] = min(len(h), maxColWidth)
	}
	for _, row := range m.rows {
		for i := range natural {
			if i < len(row) {
				w := min(len(row[i]), maxColWidth)
				if w > natural[i] {
					natural[i] = w
				}
			}
		}
	}

	// Start with natural widths.
	widths := make([]int, n)
	total := 0
	for i, w := range natural {
		widths[i] = w
		total += w
	}

	if total <= avail {
		// All columns fit — distribute leftover evenly.
		leftover := avail - total
		for leftover > 0 {
			for i := range widths {
				if leftover <= 0 {
					break
				}
				widths[i]++
				leftover--
			}
		}
	} else {
		// Columns don't fit — shrink proportionally, minimum 4.
		for i := range widths {
			widths[i] = max(4, natural[i]*avail/total)
		}
		sum := 0
		for _, w := range widths {
			sum += w
		}
		for sum > avail && sum > 0 {
			widest := 0
			for i, w := range widths {
				if w > widths[widest] {
					widest = i
				}
			}
			widths[widest]--
			sum--
		}
	}

	return widths
}

func (m *tableModel) visibleRows() [][]string {
	rc := m.rowCount()
	if rc == 0 {
		return nil
	}
	vh := m.visibleHeight()
	end := m.offset + vh
	if end > rc {
		end = rc
	}
	start := m.offset
	if start > rc {
		start = rc
	}

	if m.filtered != nil {
		result := make([][]string, 0, end-start)
		for i := start; i < end; i++ {
			result = append(result, m.rows[m.filtered[i]])
		}
		return result
	}
	return m.rows[start:end]
}

func (m *tableModel) clampOffset() {
	rc := m.rowCount()
	if rc == 0 {
		m.offset = 0
		return
	}
	vh := m.visibleHeight()
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+vh {
		m.offset = m.selected - vh + 1
	}
	maxOffset := rc - vh
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}
