package telemetry

import (
	"context"
	"runtime"
	"time"

	"github.com/amplitude/analytics-go/amplitude"
)

// AmplitudeAPIKey is set at build time via ldflags.
// Override via: go build -ldflags "-X github.com/astropods/astro/apps/astro-cli/internal/telemetry.AmplitudeAPIKey=..."
var AmplitudeAPIKey = ""

// noopLogger suppresses all Amplitude SDK log output.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...interface{}) {}
func (noopLogger) Infof(string, ...interface{})  {}
func (noopLogger) Warnf(string, ...interface{})  {}
func (noopLogger) Errorf(string, ...interface{}) {}

// Client sends telemetry events directly to Amplitude via the official Go SDK.
// All methods are nil-safe (no-ops when client is nil).
type Client struct {
	amp      amplitude.Client
	userID   string
	deviceID string
	version  string
}

// NewClient creates a telemetry client. Returns nil if API key is empty.
func NewClient(userID, deviceID, cliVersion string) *Client {
	if AmplitudeAPIKey == "" {
		return nil
	}

	config := amplitude.NewConfig(AmplitudeAPIKey)
	config.FlushQueueSize = 1 // flush immediately — CLI is short-lived
	config.FlushInterval = 100 * time.Millisecond
	config.Logger = noopLogger{}

	return &Client{
		amp:      amplitude.NewClient(config),
		userID:   userID,
		deviceID: deviceID,
		version:  cliVersion,
	}
}

// TrackCommand sends a cli.command_executed event with timing and status info.
func (c *Client) TrackCommand(command string, duration time.Duration, cmdErr error) {
	if c == nil {
		return
	}

	props := map[string]interface{}{
		"command":     command,
		"duration_ms": duration.Milliseconds(),
		"success":     cmdErr == nil,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	}
	if cmdErr != nil {
		props["error_type"] = classifyError(cmdErr)
	}

	c.amp.Track(amplitude.Event{
		EventType: "cli.command_executed",
		UserID:    c.userID,
		DeviceID:  c.deviceID,
		EventOptions: amplitude.EventOptions{
			Platform:   "cli",
			OSName:     runtime.GOOS,
			AppVersion: c.version,
		},
		EventProperties: props,
	})
}

// Shutdown flushes pending events with a timeout so we never block the CLI exit.
func (c *Client) Shutdown() {
	if c == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		c.amp.Shutdown()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Cobra user errors (bad flags, missing args, unknown commands)
	for _, prefix := range []string{"unknown command", "unknown flag", "required flag", "accepts ", "invalid argument"} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			return "user_error"
		}
	}
	return "command_error"
}
