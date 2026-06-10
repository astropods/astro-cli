// Code generated manually to match proto/connect/v1/connect.proto.
// Uses JSON-over-gRPC encoding registered under the "json" content-subtype.
// Clients must opt in via grpc.CallContentSubtype("json"); see
// apps/astro-cli/internal/connect/client.go.

package connectv1

import (
	"encoding/json"

	"google.golang.org/grpc/encoding"
)

func init() {
	// Register JSON codec under name "json" so it doesn't shadow gRPC's
	// default proto codec. Idempotent with admin/v1's registration.
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                               { return "json" }

// --- Client -> Server ---

type ClientMessage struct {
	Register      *RegisterDevice `json:"register,omitempty"`
	Heartbeat     *Heartbeat      `json:"heartbeat,omitempty"`
	CommandResult *CommandResult  `json:"command_result,omitempty"`
}

type RegisterDevice struct {
	DeviceID   string `json:"device_id,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	OS         string `json:"os,omitempty"`
	Arch       string `json:"arch,omitempty"`
	CLIVersion string `json:"cli_version,omitempty"`
}

type Heartbeat struct {
	TimestampUnix int64  `json:"timestamp_unix,omitempty"`
	UptimeSeconds uint64 `json:"uptime_seconds,omitempty"`
}

type CommandResult struct {
	CommandID string `json:"command_id,omitempty"`
	ExitCode  int32  `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

// --- Server -> Client ---

type ServerMessage struct {
	RegisterAck *RegisterDeviceResponse `json:"register_ack,omitempty"`
	Command     *ShellCommand           `json:"command,omitempty"`
	Disconnect  *Disconnect             `json:"disconnect,omitempty"`
}

type RegisterDeviceResponse struct {
	Accepted bool   `json:"accepted,omitempty"`
	Message  string `json:"message,omitempty"`
}

type ShellCommand struct {
	CommandID      string            `json:"command_id,omitempty"`
	Shell          string            `json:"shell,omitempty"`
	Command        string            `json:"command,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds uint32            `json:"timeout_seconds,omitempty"`
}

type Disconnect struct {
	Reason string `json:"reason,omitempty"`
}
