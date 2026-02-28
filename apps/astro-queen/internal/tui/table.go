package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── interactive table ──────────────────────────────────────────────────────

type tableModel struct {
	headers  []string
	rows     [][]string
	selected int
	offset   int
	height   int // visible data rows
	width    int
	focused  bool
}

func newTableModel(headers []string) tableModel {
	return tableModel{
		headers: headers,
		height:  10,
	}
}

func (m *tableModel) SetRows(rows [][]string) {
	m.rows = rows
	if m.selected >= len(rows) {
		m.selected = max(0, len(rows)-1)
	}
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

func (m *tableModel) SelectedRow() []string {
	if len(m.rows) == 0 || m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	return m.rows[m.selected]
}

func (m *tableModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				if m.selected < m.offset {
					m.offset = m.selected
				}
			}
		case "down", "j":
			if m.selected < len(m.rows)-1 {
				m.selected++
				if m.selected >= m.offset+m.height {
					m.offset = m.selected - m.height + 1
				}
			}
		case "pgup":
			m.selected -= m.height
			if m.selected < 0 {
				m.selected = 0
			}
			m.clampOffset()
		case "pgdown":
			m.selected += m.height
			if m.selected >= len(m.rows) {
				m.selected = max(0, len(m.rows)-1)
			}
			m.clampOffset()
		case "home":
			m.selected = 0
			m.offset = 0
		case "end":
			m.selected = max(0, len(m.rows)-1)
			m.clampOffset()
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
			if m.selected > 0 {
				m.selected--
				if m.selected < m.offset {
					m.offset = m.selected
				}
			}
		} else if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown {
			if m.selected < len(m.rows)-1 {
				m.selected++
				if m.selected >= m.offset+m.height {
					m.offset = m.selected - m.height + 1
				}
			}
		}
	}
	return nil
}

func (m *tableModel) View() string {
	if m.width == 0 || len(m.headers) == 0 {
		return ""
	}

	colWidths := m.columnWidths()
	visibleRows := m.visibleRows()

	var b strings.Builder

	// Header row.
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	for i, h := range m.headers {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(headerStyle.Width(colWidths[i]).MaxWidth(colWidths[i]).Render(h))
	}

	// Data rows.
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	selectedIdx := m.selected - m.offset

	for ri, row := range visibleRows {
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
				b.WriteString("  ")
			}
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			b.WriteString(style.Width(colWidths[ci]).MaxWidth(colWidths[ci]).Render(val))
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

// columnWidths sizes columns to fit their content, then distributes leftover space.
func (m *tableModel) columnWidths() []int {
	n := len(m.headers)
	gap := 2 * (n - 1) // 2-char gap between columns
	avail := m.width - gap
	if avail < n {
		avail = n
	}

	// Measure natural width of each column (max of header + all visible data).
	natural := make([]int, n)
	for i, h := range m.headers {
		natural[i] = len(h)
	}
	for _, row := range m.rows {
		for i := range natural {
			if i < len(row) && len(row[i]) > natural[i] {
				natural[i] = len(row[i])
			}
		}
	}

	// Start with natural widths, capped at available space.
	widths := make([]int, n)
	total := 0
	for i, w := range natural {
		widths[i] = w
		total += w
	}

	if total <= avail {
		// All columns fit — distribute leftover space proportionally.
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
		// Columns don't fit — shrink proportionally but keep a minimum of 4.
		for i := range widths {
			widths[i] = max(4, natural[i]*avail/total)
		}
		// Correct rounding errors.
		sum := 0
		for _, w := range widths {
			sum += w
		}
		for sum > avail && sum > 0 {
			// Shrink the widest column.
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
	if len(m.rows) == 0 {
		return nil
	}
	end := m.offset + m.height
	if end > len(m.rows) {
		end = len(m.rows)
	}
	return m.rows[m.offset:end]
}

func (m *tableModel) clampOffset() {
	if len(m.rows) == 0 {
		m.offset = 0
		return
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+m.height {
		m.offset = m.selected - m.height + 1
	}
	maxOffset := len(m.rows) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}
