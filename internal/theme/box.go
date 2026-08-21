package theme

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	minBoxWidth   = 24
	boxBorderCols = 2
	boxFrameWidth = 6
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
	if avail <= 0 || lipgloss.Width(rendered) <= avail {
		return rendered
	}

	if avail < minBoxWidth {
		avail = minBoxWidth
	}
	return style.Width(avail - boxBorderCols).Render(content)
}

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
