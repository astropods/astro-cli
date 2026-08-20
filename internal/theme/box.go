package theme

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	// minBoxWidth is the narrowest box we render, including borders. Below this
	// the content is unreadable anyway, so we stop shrinking and let the
	// terminal wrap.
	minBoxWidth = 24

	// boxFrameWidth is the space the frame itself takes: one border column and
	// two padding columns on each side.
	boxFrameWidth = 6
)

// boxStyle is the shared frame for the CLI summary boxes.
func boxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(Primary).
		Padding(0, 2)
}

// Box renders lines inside a bordered frame. When the natural width exceeds the
// terminal, the content wraps inside the frame instead of the frame overflowing.
func Box(lines []string) string {
	content := strings.Join(lines, "\n")
	style := boxStyle()
	rendered := style.Render(content)

	avail := TerminalWidth()
	if avail <= 0 || lipgloss.Width(rendered) <= avail {
		return rendered
	}

	if avail < minBoxWidth {
		avail = minBoxWidth
	}
	// Width sets the content block; the border adds a column on each side.
	return style.Width(avail - 2).Render(content)
}

// BoxContentWidth returns the widest line a box holds without wrapping, or 0
// when the terminal width is unknown. Callers use it to move content that does
// not fit, such as a URL that must stay on one line, outside the box.
func BoxContentWidth() int {
	w := TerminalWidth()
	if w <= 0 {
		return 0
	}
	if w < minBoxWidth {
		w = minBoxWidth
	}
	return w - boxFrameWidth
}

// TerminalWidth returns the width of the terminal on stdout, or 0 when the
// width is unknown. COLUMNS overrides the detected size.
func TerminalWidth() int {
	if env := os.Getenv("COLUMNS"); env != "" {
		if w, err := strconv.Atoi(env); err == nil && w > 0 {
			return w
		}
	}

	fd := int(os.Stdout.Fd()) //nolint:gosec
	if !term.IsTerminal(fd) {
		return 0
	}
	w, _, err := term.GetSize(fd)
	if err != nil || w <= 0 {
		return 0
	}
	return w
}
