package cmd

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func successPipeline() *PushPipeline {
	p := NewPushPipeline(context.Background(), PushPipelineConfig{
		Account:   "astro-preview",
		AgentName: "credit-eater",
	})
	p.tag = "09182293"
	p.visibility = VisibilityPublic
	return p
}

// boxRows returns the rendered frame rows of a success box.
func boxRows(out string) []string {
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		if strings.ContainsAny(line, "╔║╚") {
			rows = append(rows, line)
		}
	}
	return rows
}

func TestPrintSuccessBoxFitsTerminal(t *testing.T) {
	for _, columns := range []int{120, 68, 60, 40, 24} {
		t.Run(strconv.Itoa(columns)+" columns", func(t *testing.T) {
			t.Setenv("COLUMNS", strconv.Itoa(columns))

			rows := boxRows(captureStdout(t, successPipeline().PrintSuccess))
			require.NotEmpty(t, rows, "output must contain a box")

			width := lipgloss.Width(rows[0])
			assert.LessOrEqual(t, width, columns, "box must fit the terminal")
			for _, row := range rows {
				assert.Equal(t, width, lipgloss.Width(row), "every row must match the frame width: %q", row)
			}
		})
	}
}

func TestPrintSuccessURLPlacement(t *testing.T) {
	agentURL := strings.TrimSuffix(buildinfo.DefaultServerURL, "/") + "/astro-preview/credit-eater"

	tests := []struct {
		name     string
		columns  string
		inBox    bool
		wantLine string
	}{
		{name: "inside the box when it fits", columns: "200", inBox: true},
		{name: "below the box when it does not fit", columns: "60", inBox: false, wantLine: agentURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COLUMNS", tt.columns)

			out := captureStdout(t, successPipeline().PrintSuccess)

			if tt.inBox {
				assert.Contains(t, out, "View online → "+agentURL)
				return
			}

			assert.NotContains(t, out, "View online → "+agentURL, "the box must not hold the URL")
			var standalone bool
			for _, line := range strings.Split(out, "\n") {
				if strings.TrimSpace(line) == tt.wantLine {
					standalone = true
				}
			}
			assert.True(t, standalone, "the URL must sit unbroken on its own line, got:\n%s", out)
		})
	}
}
