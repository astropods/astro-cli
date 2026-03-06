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

	connectv1 "github.com/postman/astro/packages/astro-proto/connect/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config holds connection parameters.
type Config struct {
	ServerAddr string
	Token      string
	DeviceID   string
	CLIVersion string
}

// Run connects to the server, registers, and runs the heartbeat + command loop.
// Blocks until ctx is cancelled or the connection drops.
func Run(ctx context.Context, cfg Config) error {
	// Dial via QUIC — TLS is handled by QUIC, so gRPC layer is insecure
	cc, err := grpc.NewClient(
		"passthrough:///"+cfg.ServerAddr,
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return dialQUIC(ctx, addr)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer cc.Close() //nolint:errcheck

	// Attach bearer token as metadata
	md := metadata.Pairs("authorization", "Bearer "+cfg.Token)
	streamCtx := metadata.NewOutgoingContext(ctx, md)

	client := connectv1.NewConnectServiceClient(cc)
	stream, err := client.Connect(streamCtx)
	if err != nil {
		return fmt.Errorf("connect stream: %w", err)
	}

	// Register device
	hostname, _ := os.Hostname()
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
	ack, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("register ack: %w", err)
	}
	if ack.RegisterAck != nil && !ack.RegisterAck.Accepted {
		return fmt.Errorf("registration rejected: %s", ack.RegisterAck.Message)
	}

	fmt.Printf("Connected as device %s\n", cfg.DeviceID)

	startTime := time.Now()

	// Start heartbeat in background
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = stream.Send(&connectv1.ClientMessage{
					Heartbeat: &connectv1.Heartbeat{
						TimestampUnix: time.Now().Unix(),
						UptimeSeconds: uint64(time.Since(startTime).Seconds()),
					},
				})
			}
		}
	}()

	// Receive loop — handle server commands
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch {
		case msg.Command != nil:
			fmt.Printf("Executing command %s: %s\n", msg.Command.CommandID, msg.Command.Command)
			result := execShellCommand(msg.Command)
			if err := stream.Send(&connectv1.ClientMessage{CommandResult: result}); err != nil {
				return fmt.Errorf("send result: %w", err)
			}

		case msg.Disconnect != nil:
			fmt.Printf("Server requested disconnect: %s\n", msg.Disconnect.Reason)
			return nil
		}
	}
}
