package connectgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"
	"github.com/google/uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/devicestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// deviceSession tracks a connected device's stream and pending command responses.
type deviceSession struct {
	stream  connectv1.ConnectService_ConnectServer
	mu      sync.Mutex
	pending map[string]chan *connectv1.CommandResult // command_id -> result channel
}

type Server struct {
	connectv1.UnimplementedConnectServiceServer
	log          *logger.Logger
	deviceStore  *devicestore.Store
	accountStore *account.AccountStore
	reaperOnce   sync.Once

	mu       sync.RWMutex
	sessions map[string]*deviceSession // device_id -> session
}

func New(log *logger.Logger, deviceStore *devicestore.Store, accountStore *account.AccountStore) *Server {
	return &Server{
		log:          log,
		deviceStore:  deviceStore,
		accountStore: accountStore,
		sessions:     make(map[string]*deviceSession),
	}
}

// SendCommand sends a shell command to a connected device and waits for the result.
func (s *Server) SendCommand(ctx context.Context, deviceID string, cmd *connectv1.ShellCommand) (*connectv1.CommandResult, error) {
	s.mu.RLock()
	sess, ok := s.sessions[deviceID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("device %q is not connected", deviceID)
	}

	// Assign command ID
	if cmd.CommandID == "" {
		cmd.CommandID = uuid.NewString()[:8]
	}

	// Register pending result channel
	resultCh := make(chan *connectv1.CommandResult, 1)
	sess.mu.Lock()
	sess.pending[cmd.CommandID] = resultCh
	sess.mu.Unlock()

	defer func() {
		sess.mu.Lock()
		delete(sess.pending, cmd.CommandID)
		sess.mu.Unlock()
	}()

	// Send command to device
	sess.mu.Lock()
	err := sess.stream.Send(&connectv1.ServerMessage{Command: cmd})
	sess.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// Wait for result with timeout
	timeout := 30 * time.Second
	if cmd.TimeoutSeconds > 0 {
		timeout = time.Duration(cmd.TimeoutSeconds)*time.Second + 5*time.Second
	}

	select {
	case result := <-resultCh:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("command timed out after %s", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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

	if userID == "" {
		return status.Error(codes.Unauthenticated, "missing user identity")
	}

	// Resolve the user's org from the JWT claim, falling back to their personal account.
	orgID := OrgIDFromContext(ctx)
	if orgID == "" {
		accounts, err := s.accountStore.GetAccountsForUser(userID)
		if err != nil {
			s.log.Error("failed to resolve accounts for user", "error", err, "user_id", userID)
			return status.Error(codes.Internal, "failed to resolve account")
		}
		for _, a := range accounts {
			if a.Type == "personal" {
				orgID = a.ID
				break
			}
		}
		if orgID == "" {
			return status.Error(codes.PermissionDenied, "no account found — run 'ast login' and create an account first")
		}
	}

	var deviceID string
	var registered bool
	var sess *deviceSession

	defer func() {
		if registered && deviceID != "" {
			s.mu.Lock()
			delete(s.sessions, deviceID)
			s.mu.Unlock()
			s.disconnectDevice(orgID, deviceID)
		}
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
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

			// Register session for command dispatch
			sess = &deviceSession{
				stream:  stream,
				pending: make(map[string]chan *connectv1.CommandResult),
			}
			s.mu.Lock()
			s.sessions[deviceID] = sess
			s.mu.Unlock()

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
			if sess != nil {
				sess.mu.Lock()
				ch, ok := sess.pending[msg.CommandResult.CommandID]
				sess.mu.Unlock()
				if ok {
					ch <- msg.CommandResult
				}
			}
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
