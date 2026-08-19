package metering

import (
	"math"
	"testing"
)

func TestParseCPU(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"100m", 0.1},
		{"250m", 0.25},
		{"1", 1},
		{"2", 2},
		{"1.5", 1.5},
		{"", 0},
		{"0m", 0},
	}
	for _, tt := range tests {
		got := parseCPU(tt.input)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("parseCPU(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1Gi", 1},
		{"256Mi", 0.25},
		{"512Mi", 0.5},
		{"2Gi", 2},
		{"128Mi", 0.125},
		{"1G", 1},
		{"500M", 0.5},
		{"1Ti", 1024},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMemory(tt.input)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("parseMemory(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
