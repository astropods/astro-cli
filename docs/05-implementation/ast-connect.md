# `ast connect` — Persistent Device Connection via QUIC+gRPC

## Overview

`ast connect` establishes a long-lived gRPC-over-QUIC bidirectional streaming connection from the CLI to astro-server. The connected device registers itself, reports health, and receives server-pushed shell commands for execution. Can run as a daemon.

## Architecture

```
+---------------+   gRPC/QUIC (TLS)  +------------------+
|  ast connect  | <-----------------> |  astro-server    |
|  (CLI/daemon) |   bidi stream       |  ConnectService  |
|               |   Bearer JWT auth   |  port 9092/udp   |
+---+-----------+                     +------------------+
    |                                        |
    v                                 +------+------+
+---+-----------+                     |  PostgreSQL  |
| local shell   |                     |  devices tbl |
+---------------+                     +-------------+
```

## Design Decisions

### 1. Separate gRPC service on its own port

ConnectService is user-facing (JWT auth), AdminService is operator-facing (mTLS). Different trust models, different ports.

- AdminService: port 9091, mTLS, operator tooling (astro-queen)
- ConnectService: port 9092, JWT Bearer, CLI clients

### 2. QUIC transport

gRPC over QUIC using `quic-go` (already an indirect dep in astro-server v0.54.0). Benefits over TCP:
- 0-RTT connection establishment — faster reconnects after sleep/wake
- Connection migration — survives network changes (WiFi to ethernet, laptop undock)
- Multiplexed streams without head-of-line blocking
- Built-in TLS 1.3 (QUIC mandates it, no separate TLS handshake)
- Better on lossy/unstable networks

Server listens on UDP port 9092. Client dials via `quic-go` transport, gRPC runs on top using `grpc.NewClient` with a custom QUIC dialer. The `quic-go` library provides `quic.Transport` (server) and `quic.Dial` (client) — we wrap these to satisfy gRPC's `net.Listener` and `net.Conn` interfaces respectively.

### 3. Auth via Bearer JWT in gRPC metadata

CLI reuses existing `auth.TokenManager` — no new auth flow. Token sent as gRPC metadata (`authorization: Bearer <token>`). Server-side stream interceptor validates using existing `auth.JWTValidator`, extracts user_id + org_id from claims. Client auto-refreshes token before expiry.

### 4. Single org connection

Device connects once with the user's active org (or personal account). No multi-org multiplexing — switch org requires reconnect.

## Proto Definition

New file: `packages/astro-proto/proto/connect/v1/connect.proto`

```proto
service ConnectService {
  rpc Connect(stream ClientMessage) returns (stream ServerMessage);
}

// --- Client -> Server ---

message ClientMessage {
  oneof payload {
    RegisterDevice register = 1;
    Heartbeat heartbeat = 2;
    CommandResult command_result = 3;
  }
}

message RegisterDevice {
  string device_id = 1;       // stable UUID from ~/.ast/device_id
  string hostname = 2;
  string os = 3;
  string arch = 4;
  string cli_version = 5;
}

message Heartbeat {
  int64 timestamp_unix = 1;
  uint64 uptime_seconds = 2;
}

message CommandResult {
  string command_id = 1;
  int32 exit_code = 2;
  string stdout = 3;
  string stderr = 4;
}

// --- Server -> Client ---

message ServerMessage {
  oneof payload {
    RegisterDeviceResponse register_ack = 1;
    ShellCommand command = 2;
    Disconnect disconnect = 3;
  }
}

message RegisterDeviceResponse {
  bool accepted = 1;
  string message = 2;
}

message ShellCommand {
  string command_id = 1;       // for correlating CommandResult
  string shell = 2;            // e.g. "/bin/sh", "/bin/bash" (default: user's shell)
  string command = 3;          // the command string to execute
  string working_dir = 4;      // optional cwd
  map<string, string> env = 5; // additional env vars
  uint32 timeout_seconds = 6;  // 0 = no timeout
}

message Disconnect {
  string reason = 1;
}
```

Uses JSON-over-gRPC codec (matching existing admin proto pattern).

## Database Schema

```sql
CREATE TABLE connected_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    hostname TEXT,
    os TEXT,
    arch TEXT,
    cli_version TEXT,
    status TEXT NOT NULL DEFAULT 'connected',  -- connected | disconnected
    last_heartbeat_at TIMESTAMPTZ,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disconnected_at TIMESTAMPTZ,
    UNIQUE(account_id, device_id)
);

CREATE INDEX idx_connected_devices_account_status
    ON connected_devices(account_id, status);
```

## Connection Lifecycle

1. CLI loads token via `TokenManager.GetValidAccessToken()`
2. QUIC dial to server's connect port (UDP 9092), TLS 1.3 built into QUIC handshake
3. gRPC `Connect()` bidi stream opens over QUIC with `authorization` metadata
4. Server validates JWT, extracts user_id + org_id
5. Client sends `RegisterDevice` — server upserts `connected_devices`, responds with ack
6. Heartbeat loop (30s) — client sends `Heartbeat`, server updates `last_heartbeat_at`
7. Server can push `ShellCommand` at any time — client executes via `os/exec` and sends `CommandResult`
8. Server-side reaper marks stale devices (no heartbeat >2min) as `disconnected`
9. Graceful shutdown — client catches SIGINT/SIGTERM, stream closes, server marks `disconnected`

## Shell Command Execution (Client-side)

When the client receives a `ShellCommand`:

1. Spawn `os/exec.Command(shell, "-c", command)` with optional `working_dir` and `env`
2. Capture stdout and stderr separately
3. If `timeout_seconds > 0`, wrap with `context.WithTimeout` and kill on expiry
4. Send `CommandResult` back with `command_id`, `exit_code`, `stdout`, `stderr`

This keeps the client dead simple — any shell command, including `docker`, `orbctl`, `kubectl`, config management, etc. The server decides what to run; the client just executes.

## Components

### Server-side

| # | Component | Location |
|---|-----------|----------|
| 1 | Proto definition | `packages/astro-proto/proto/connect/v1/connect.proto` |
| 2 | Generated Go code | `packages/astro-proto/connect/v1/` |
| 3 | DB migration | `apps/astro-server/migrations/` |
| 4 | Device store | `apps/astro-server/internal/devicestore/` |
| 5 | JWT stream interceptor | `apps/astro-server/internal/connectgrpc/auth.go` |
| 6 | ConnectService impl | `apps/astro-server/internal/connectgrpc/server.go` |
| 7 | QUIC listener + gRPC wiring | `apps/astro-server/internal/connectgrpc/quic.go` |
| 8 | Wire into main.go | Start alongside HTTP + admin gRPC |

### CLI-side

| # | Component | Location |
|---|-----------|----------|
| 9 | `ast connect` command | `apps/astro-cli/cmd/connect.go` |
| 10 | Device ID persistence | `apps/astro-cli/internal/device/id.go` |
| 11 | QUIC dialer + gRPC client | `apps/astro-cli/internal/connect/client.go` |
| 12 | Shell command executor | `apps/astro-cli/internal/connect/exec.go` |
| 13 | Graceful shutdown | Signal handling in connect command |

### Daemon mode

| # | Component | Location |
|---|-----------|----------|
| 14 | `--daemon` flag | Re-exec with `--foreground`, detach via `Setsid` |
| 15 | PID file | `~/.ast/connect.pid` |
| 16 | `--status` flag | Check PID file + process liveness |
| 17 | `--stop` flag | Send SIGTERM to daemon PID |
| 18 | `install-service` subcommand | macOS: launchd plist, Linux: systemd user unit |

## Daemon Details

### Foreground vs background

- `ast connect` — foreground (default), logs to stdout
- `ast connect --daemon` — re-execs with `--foreground`, detached via `SysProcAttr{Setsid: true}`. PID to `~/.ast/connect.pid`, logs to `~/.ast/connect.log`
- `ast connect --status` — reads PID file, checks process alive
- `ast connect --stop` — SIGTERM to daemon PID

### OS service integration

`ast connect install-service`:

**macOS** — `~/Library/LaunchAgents/com.postman.ast-connect.plist`
- `KeepAlive: true`, starts on login, runs `ast connect --foreground`

**Linux** — `~/.config/systemd/user/ast-connect.service`
- `Restart=always`, `systemctl --user enable ast-connect`

## Config

### Server env vars

- `FLEET_GRPC_PORT` — default `9092` (UDP, QUIC)
- `FLEET_TLS_CERT_PATH` — TLS cert path (provided by platform via `fleet-tls` K8s secret at `/etc/fleet-tls/tls.crt`)
- `FLEET_TLS_KEY_PATH` — TLS key path (provided by platform via `fleet-tls` K8s secret at `/etc/fleet-tls/tls.key`)

### CLI flags

```
ast connect                       # foreground (default)
ast connect --daemon              # background
ast connect --status              # check daemon status
ast connect --stop                # stop daemon
ast connect --server <host:port>  # override server address
ast connect install-service       # install as OS service
```

## Implementation Order

1. Proto definition + codegen
2. DB migration + devicestore
3. QUIC listener (server) + QUIC dialer (client) wrapping gRPC
4. Server ConnectService + JWT interceptor + wire into main.go
5. CLI `ast connect` foreground (register + heartbeat + shell exec loop)
6. Daemon mode + PID management
7. `install-service` subcommand
