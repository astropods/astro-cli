package connectgrpc

import (
	"context"
	"io"
	"sync"
	"time"

	connectv1 "github.com/postman/astro/packages/astro-proto/connect/v1"

	"github.com/postman/astro/apps/astro-server/internal/devicestore"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

type Server struct {
	connectv1.UnimplementedConnectServiceServer
	log         *logger.Logger
	deviceStore *devicestore.Store
	reaperOnce  sync.Once
}

func New(log *logger.Logger, deviceStore *devicestore.Store) *Server {
	return &Server{log: log, deviceStore: deviceStore}
}

// StartReaper starts a background goroutine that marks stale devices as disconnected.
func (s *Server) StartReaper(ctx context.Context) {
	s.reaperOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					n, err := s.deviceStore.ReapStale(ctx, 2*time.Minute)
					if err != nil {
						s.log.Error("device reaper failed", "error", err)
					} else if n > 0 {
						s.log.Info("reaped stale devices", "count", n)
					}
				}
			}
		}()
	})
}

func (s *Server) Connect(stream connectv1.ConnectService_ConnectServer) error {
	ctx := stream.Context()
	userID := UserIDFromContext(ctx)
	orgID := OrgIDFromContext(ctx)

	if userID == "" || orgID == "" {
		return io.EOF
	}

	var deviceID string
	var registered bool

	for {
		msg, err := stream.Recv()
		if err != nil {
			// Client disconnected — mark device offline
			if registered {
				s.disconnectDevice(orgID, deviceID)
			}
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch {
		case msg.Register != nil:
			deviceID = msg.Register.DeviceID
			_, dbErr := s.deviceStore.Upsert(ctx, orgID, userID, &devicestore.Device{
				DeviceID:   msg.Register.DeviceID,
				Hostname:   msg.Register.Hostname,
				OS:         msg.Register.OS,
				Arch:       msg.Register.Arch,
				CLIVersion: msg.Register.CLIVersion,
			})
			if dbErr != nil {
				s.log.Error("device registration failed", "error", dbErr, "device_id", deviceID)
				_ = stream.Send(&connectv1.ServerMessage{
					RegisterAck: &connectv1.RegisterDeviceResponse{
						Accepted: false,
						Message:  "registration failed",
					},
				})
				continue
			}
			registered = true
			s.log.Info("device connected",
				"device_id", deviceID,
				"hostname", msg.Register.Hostname,
				"user_id", userID,
				"org_id", orgID,
			)
			_ = stream.Send(&connectv1.ServerMessage{
				RegisterAck: &connectv1.RegisterDeviceResponse{
					Accepted: true,
					Message:  "registered",
				},
			})

		case msg.Heartbeat != nil:
			if registered {
				_ = s.deviceStore.Heartbeat(ctx, orgID, deviceID)
			}

		case msg.CommandResult != nil:
			s.log.Info("command result",
				"command_id", msg.CommandResult.CommandID,
				"exit_code", msg.CommandResult.ExitCode,
				"device_id", deviceID,
			)
		}
	}
}

func (s *Server) disconnectDevice(orgID, deviceID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.deviceStore.Disconnect(ctx, orgID, deviceID); err != nil {
		s.log.Error("failed to mark device disconnected", "error", err, "device_id", deviceID)
	} else {
		s.log.Info("device disconnected", "device_id", deviceID)
	}
}
