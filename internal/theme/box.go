package theme

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	minBoxWidth          = 24
	boxBorderCols        = 2
	boxFrameWidth        = 6
	defaultTerminalWidth = 80
)

func boxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(Primary).
		Padding(0, 2)
}

func Box(lines []string) string {
	content := strings.Join(lines, "\n")
	style := boxStyle()
	rendered := style.Render(content)

	avail := TerminalWidth()
	if lipgloss.Width(rendered) <= avail {
		return rendered
	}

	if avail < minBoxWidth {
		avail = minBoxWidth
	}
	return style.Width(avail - boxBorderCols).Render(content)
}

func BoxContentWidth() int {
	w := TerminalWidth()
	if w < minBoxWidth {
		w = minBoxWidth
	}
	return w - boxFrameWidth
}

func TerminalWidth() int {
	return resolveTerminalWidth(ttyWidth(), os.Getenv("COLUMNS"))
}

// An exported COLUMNS goes stale when a pane is split or resized, so the live
// terminal size wins.
func resolveTerminalWidth(tty int, columns string) int {
	if tty > 0 {
		return tty
	}
	if w, err := strconv.Atoi(columns); err == nil && w > 0 {
		return w
	}
	return defaultTerminalWidth
}

func ttyWidth() int {
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
