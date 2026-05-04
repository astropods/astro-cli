package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"sub-second", 250 * time.Millisecond, "0.2s"},
		{"a few seconds", 4500 * time.Millisecond, "4.5s"},
		{"just under a minute", 59*time.Second + 900*time.Millisecond, "59.9s"},
		{"exactly one minute", time.Minute, "1m 00s"},
		{"one minute and change", 78 * time.Second, "1m 18s"},
		{"long build", 6*time.Minute + 48*time.Second, "6m 48s"},
		{"zero", 0, "0.0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatElapsed(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHeartbeatInterval(t *testing.T) {
	assert.Equal(t, 15*time.Second, heartbeatInterval(false))
	assert.Equal(t, 5*time.Second, heartbeatInterval(true))
}

func TestTruncateVertexName(t *testing.T) {
	short := "[builder 1/3] FROM node:20"
	assert.Equal(t, short, truncateVertexName(short, 40))

	long := "[frontend-builder 6/6] RUN --mount=type=cache,target=/root/.bun bun install && bun run build"
	got := truncateVertexName(long, 40)
	assert.Equal(t, 40, len([]rune(got)))
	assert.Contains(t, got, "frontend-builder")
}
