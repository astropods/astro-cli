package openmeter_tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchSelectOption is a single item in the search-select list.
type SearchSelectOption struct {
	Label string
	Value string
}

// SearchSelect renders as a single-line field by default and opens a
// searchable list modal when the user presses `/`.
type SearchSelect struct {
	placeholder string
	options     []SearchSelectOption
	value       string // current selected value
	label       string // current selected label
	open        bool
	filter      string
	cursor      int // index into filtered results
	offset      int // scroll offset
	height      int // max visible items
	width       int
	input       textinput.Model
}

// NewSearchSelect creates a SearchSelect with the given placeholder and options.
func NewSearchSelect(placeholder string, opts []SearchSelectOption) SearchSelect {
	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.Prompt = ""
	ti.CharLimit = 200
	return SearchSelect{
		placeholder: placeholder,
		options:     opts,
		height:      10,
		width:       36,
		input:       ti,
	}
}

// WithHeight sets the max visible items in the modal list.
func (s SearchSelect) WithHeight(h int) SearchSelect { s.height = h; return s }

// WithWidth sets the width used for rendering.
func (s SearchSelect) WithWidth(w int) SearchSelect { s.width = w; return s }

// Value returns the currently selected value.
func (s SearchSelect) Value() string { return s.value }

// Label returns the currently selected label.
func (s SearchSelect) Label() string { return s.label }

// IsOpen returns true when the search modal is active.
func (s SearchSelect) IsOpen() bool { return s.open }

// SetValue sets the current selection.
func (s *SearchSelect) SetValue(v string) {
	s.value = v
	s.label = v
	for _, o := range s.options {
		if o.Value == v {
			s.label = o.Label
			break
		}
	}
}

// SetOptions replaces the option list, preserving the current selection if still valid.
func (s *SearchSelect) SetOptions(opts []SearchSelectOption) {
	s.options = opts
	if s.value != "" {
		found := false
		for _, o := range opts {
			if o.Value == s.value {
				s.label = o.Label
				found = true
				break
			}
		}
		if !found {
			// keep the value as-is (custom value)
		}
	}
}

// Init returns the blink command for the embedded text input.
func (s SearchSelect) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles key messages. Returns a copy.
func (s SearchSelect) Update(msg tea.Msg) (SearchSelect, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// Forward non-key messages (e.g. blink) to text input when open.
		if s.open {
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
		return s, nil
	}

	if !s.open {
		if key.String() == "/" {
			s.open = true
			s.filter = ""
			s.cursor = 0
			s.offset = 0
			s.input.SetValue("")
			s.input.Focus()
			return s, textinput.Blink
		}
		return s, nil
	}

	// Modal is open.
	switch key.String() {
	case "esc":
		s.open = false
		s.input.Blur()
		return s, nil

	case "enter":
		filtered := s.filtered()
		if s.cursor < len(filtered) {
			s.value = filtered[s.cursor].Value
			s.label = filtered[s.cursor].Label
		} else if strings.TrimSpace(s.filter) != "" {
			// Use filter text as custom value.
			s.value = strings.TrimSpace(s.filter)
			s.label = s.value
		}
		s.open = false
		s.input.Blur()
		return s, nil

	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
			if s.cursor < s.offset {
				s.offset = s.cursor
			}
		}
		return s, nil

	case "down", "j":
		filtered := s.filtered()
		if s.cursor < len(filtered)-1 {
			s.cursor++
			if s.cursor >= s.offset+s.height {
				s.offset = s.cursor - s.height + 1
			}
		}
		return s, nil
	}

	// Forward to text input for typing/backspace.
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	newFilter := s.input.Value()
	if newFilter != s.filter {
		s.filter = newFilter
		s.cursor = 0
		s.offset = 0
	}
	return s, cmd
}

// View renders the inline (collapsed) representation.
func (s SearchSelect) View() string {
	if s.label != "" {
		return lipgloss.NewStyle().Foreground(colGreen).Render(s.label) +
			"  " + dimStyle.Render("/ search")
	}
	return dimStyle.Render(s.placeholder) + "  " + dimStyle.Render("/ search")
}

// ModalView renders the modal body content. The parent wraps this in an overlay.
func (s SearchSelect) ModalView() string {
	w := s.width
	if w < 30 {
		w = 30
	}

	var b strings.Builder

	// Search row
	b.WriteString(labelStyle.Render("Search: ") + s.input.View() + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", w)) + "\n")

	filtered := s.filtered()
	visible := s.height
	if visible > len(filtered) {
		visible = len(filtered)
	}

	if len(filtered) == 0 {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	} else {
		end := s.offset + visible
		if end > len(filtered) {
			end = len(filtered)
		}
		for i := s.offset; i < end; i++ {
			opt := filtered[i]
			if i == s.cursor {
				line := focusStyle.Render("▸ ") + lipgloss.NewStyle().Foreground(colAccent).Render(opt.Label)
				b.WriteString(line + "\n")
			} else {
				b.WriteString("  " + opt.Label + "\n")
			}
		}
	}

	// Footer
	b.WriteString(lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", w)) + "\n")
	matchInfo := fmt.Sprintf("%d of %d matches", len(filtered), len(s.options))
	footer := dimStyle.Render(matchInfo + "  •  ↑↓ navigate  Enter select  Esc cancel")
	b.WriteString(footer)

	return b.String()
}

// filtered returns options matching the current filter.
func (s SearchSelect) filtered() []SearchSelectOption {
	if s.filter == "" {
		return s.options
	}
	lower := strings.ToLower(s.filter)
	var out []SearchSelectOption
	for _, o := range s.options {
		if strings.Contains(strings.ToLower(o.Label), lower) {
			out = append(out, o)
		}
	}
	return out
}
