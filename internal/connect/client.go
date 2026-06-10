package connect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"time"

	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ANSI helpers — duplicated from cmd/push.go to keep the connect package
// self-contained (no import cycle back into cmd).
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cRed   = "\033[31m"
	cGreen = "\033[32m"
	cCyan  = "\033[36m"
)

// Config holds connection parameters.
type Config struct {
	ServerAddr string
	Token      string
	DeviceID   string
	CLIVersion string
	Verbose    bool
}

// verbf prints an indented detail line only when verbose mode is enabled.
func (c *Config) verbf(format string, args ...any) {
	if c.Verbose {
		fmt.Printf("   "+cDim+format+cReset+"\n", args...)
	}
}

// Run connects to the server, registers, and runs the heartbeat + command loop.
// Blocks until ctx is cancelled or the connection drops.
func Run(ctx context.Context, cfg Config) error {
	fmt.Printf("%s→%s Connecting to %s%s%s\n", cCyan, cReset, cBold, cfg.ServerAddr, cReset)
	cfg.verbf("device=%s  cli_version=%s", cfg.DeviceID, cfg.CLIVersion)

	// Dial via QUIC — TLS is handled by QUIC, so gRPC layer is insecure
	var verbose func(string, ...any)
	if cfg.Verbose {
		verbose = cfg.verbf
	}
	cc, err := grpc.NewClient(
		"passthrough:///"+cfg.ServerAddr,
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return dialQUIC(ctx, addr, verbose)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// connectv1 messages are JSON-encoded; opt into the "json" codec.
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")),
	)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer cc.Close() //nolint:errcheck

	// Attach bearer token as metadata
	md := metadata.Pairs("authorization", "Bearer "+cfg.Token)
	streamCtx := metadata.NewOutgoingContext(ctx, md)
	cfg.verbf("opening bidirectional gRPC stream")

	client := connectv1.NewConnectServiceClient(cc)
	stream, err := client.Connect(streamCtx)
	if err != nil {
		return fmt.Errorf("connect stream: %w", err)
	}
	cfg.verbf("gRPC stream opened")

	// Register device
	hostname, _ := os.Hostname()
	fmt.Printf("%s→%s Registering device %s%s%s\n", cCyan, cReset, cBold, hostname, cReset)
	cfg.verbf("os=%s  arch=%s  device_id=%s", runtime.GOOS, runtime.GOARCH, cfg.DeviceID)
	if err := stream.Send(&connectv1.ClientMessage{
		Register: &connectv1.RegisterDevice{
			DeviceID:   cfg.DeviceID,
			Hostname:   hostname,
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
			CLIVersion: cfg.CLIVersion,
		},
	}); err != nil {
		return fmt.Errorf("register: %w", err)
	}

	// Wait for ack
	cfg.verbf("waiting for registration ack")
	ack, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("register ack: %w", err)
	}
	if ack.RegisterAck != nil && !ack.RegisterAck.Accepted {
		return fmt.Errorf("registration rejected: %s", ack.RegisterAck.Message)
	}

	fmt.Printf("%s✓%s Connected as %s%s%s\n", cGreen, cReset, cBold, cfg.DeviceID, cReset)
	fmt.Printf("%s→%s %sWaiting for commands. Press Ctrl+C to disconnect%s\n", cCyan, cReset, cDim, cReset)

	startTime := time.Now()

	// Start heartbeat in background
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				cfg.verbf("heartbeat stopped")
				return
			case <-ticker.C:
				uptime := uint64(time.Since(startTime).Seconds())
				cfg.verbf("heartbeat sent (uptime=%ds)", uptime)
				if err := stream.Send(&connectv1.ClientMessage{
					Heartbeat: &connectv1.Heartbeat{
						TimestampUnix: time.Now().Unix(),
						UptimeSeconds: uptime,
					},
				}); err != nil {
					cfg.verbf("heartbeat failed: %v", err)
				}
			}
		}
	}()

	// Receive loop — handle server commands
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				cfg.verbf("stream closed (eof=%v ctx_err=%v)", errors.Is(err, io.EOF), ctx.Err())
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch {
		case msg.Command != nil:
			fmt.Printf("%s→%s Executing: %s\n", cCyan, cReset, msg.Command.Command)
			cfg.verbf("command_id=%s  shell=%q  workdir=%q  timeout=%ds  env_keys=%d",
				msg.Command.CommandID, msg.Command.Shell, msg.Command.WorkingDir, msg.Command.TimeoutSeconds, len(msg.Command.Env))
			result := execShellCommand(msg.Command)
			if result.ExitCode == 0 {
				fmt.Printf("  %s✓%s %sdone%s\n", cGreen, cReset, cDim, cReset)
			} else {
				fmt.Printf("  %s✗%s %sexit code %d%s\n", cRed, cReset, cDim, result.ExitCode, cReset)
			}
			cfg.verbf("stdout_len=%d  stderr_len=%d", len(result.Stdout), len(result.Stderr))
			if err := stream.Send(&connectv1.ClientMessage{CommandResult: result}); err != nil {
				return fmt.Errorf("send result: %w", err)
			}

		case msg.Disconnect != nil:
			fmt.Printf("%s→%s Server requested disconnect: %s\n", cCyan, cReset, msg.Disconnect.Reason)
			return nil
		}
	}
}
