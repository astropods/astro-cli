package theme

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBox(t *testing.T) {
	wide := []string{
		"✓ Pushed successfully!",
		"",
		"  credit-eater  tag 09182293",
		"  View online → https://astropod.ai/astro-preview/credit-eater",
	}

	tests := []struct {
		name      string
		columns   int
		lines     []string
		wantWidth int
	}{
		{name: "fits naturally", columns: 120, lines: wide, wantWidth: 68},
		{name: "exact fit", columns: 68, lines: wide, wantWidth: 68},
		{name: "wraps to terminal", columns: 50, lines: wide, wantWidth: 50},
		{name: "wraps to narrow terminal", columns: 36, lines: wide, wantWidth: 36},
		{name: "stops at the minimum width", columns: 10, lines: wide, wantWidth: minBoxWidth},
		{name: "short content keeps its own width", columns: 200, lines: []string{"short"}, wantWidth: 11},
		{
			name:      "keeps the frame square around an unbreakable token",
			columns:   40,
			lines:     []string{"https://astropod.ai/matt/verylongblueprintnamewithnobreaks"},
			wantWidth: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", strconv.Itoa(tt.columns))

			box := Box(tt.lines)

			rows := strings.Split(box, "\n")
			require.NotEmpty(t, rows)
			for _, row := range rows {
				assert.Equal(t, tt.wantWidth, lipgloss.Width(row), "every row must match the frame width: %q", row)
			}
		})
	}
}

func TestBoxContentWidth(t *testing.T) {
	tests := []struct {
		name    string
		columns string
		want    int
	}{
		{name: "subtracts the frame", columns: "80", want: 74},
		{name: "clamps to the minimum box", columns: "10", want: minBoxWidth - boxFrameWidth},
		{name: "unknown width falls back to the default", columns: "not-a-number", want: defaultTerminalWidth - boxFrameWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", tt.columns)

			assert.Equal(t, tt.want, BoxContentWidth())
		})
	}
}

func TestResolveTerminalWidth(t *testing.T) {
	tests := []struct {
		name    string
		tty     int
		columns string
		want    int
	}{
		{name: "live terminal size wins over a stale COLUMNS", tty: 50, columns: "200", want: 50},
		{name: "COLUMNS answers when stdout is not a terminal", tty: 0, columns: "120", want: 120},
		{name: "neither known", tty: 0, columns: "", want: defaultTerminalWidth},
		{name: "COLUMNS is not a number", tty: 0, columns: "wide", want: defaultTerminalWidth},
		{name: "COLUMNS is zero", tty: 0, columns: "0", want: defaultTerminalWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveTerminalWidth(tt.tty, tt.columns))
		})
	}
}
