package telemetry

import (
	"runtime"
	"time"

	"github.com/amplitude/analytics-go/amplitude"
)

// AmplitudeAPIKey is set at build time via ldflags.
// Override via: go build -ldflags "-X github.com/postman/astro/apps/astro-cli/internal/telemetry.AmplitudeAPIKey=..."
var AmplitudeAPIKey = ""

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
		props["error_type"] = "command_error"
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

// Shutdown flushes pending events. Call before process exit.
func (c *Client) Shutdown() {
	if c == nil {
		return
	}
	c.amp.Shutdown()
}
