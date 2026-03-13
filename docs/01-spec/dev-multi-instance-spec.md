# Multi-Instance Local Dev Spec

**Version:** 1.0
**Date:** 2026-03-06
**Status:** Draft

## Abstract

This spec defines automatic host-port allocation and instance isolation for `ast dev`, enabling multiple concurrent agent instances from the same or different directories without manual configuration. Each invocation gets a unique instance ID, its own compose file and Docker project name, and auto-assigned host ports. Subcommands (`logs`, `stop`, `trigger`, `ports`) resolve the target instance automatically when unambiguous or require explicit selection when multiple are active.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Problem

The existing `ast dev` writes a single `.ast/docker-compose.yml` with hardcoded host-published ports. Running a second instance from the same directory overwrites that file and collides on port bindings, Docker project names, and network/volume names. Running from a different directory avoids the file collision but still conflicts on shared ports (3000, 3100, 9090, etc.). There is no mechanism to discover which ports a running instance is using.

---

## 2. Goals

1. **Zero-config multi-instance** — `ast dev` MUST run multiple instances concurrently (same directory, different directories) without user-specified port offsets.
2. **Automatic port allocation** — each published port MUST be probed and replaced with the first available host port when occupied.
3. **Instance isolation** — each invocation MUST use a unique Docker Compose project name and compose file so container, network, and volume namespaces do not collide.
4. **Subcommand ergonomics** — `logs`, `stop`, `trigger` MUST continue to work without extra flags when a single instance is active. When multiple are active, they MUST require explicit instance selection and provide actionable guidance.
5. **Port visibility** — a new `ast dev ports` command MUST display all active instances and their port mappings, with optional service-name and instance-ID filtering.

## 3. Non-Goals

1. **Remote instance discovery** — only local instances (same machine, same working directory) are tracked.
2. **Persistent data sharing** — instances do not share Docker volumes. Each instance gets isolated storage via compose project-scoped naming.
3. **Automatic instance cleanup daemon** — stale metadata is cleaned up lazily when subcommands enumerate instances, not by a background process.

---

## 4. Instance Model

### 4.1 Instance ID

Each `ast dev` invocation MUST generate a unique instance ID. The ID is an 8-character lowercase hex string produced from 4 bytes of `crypto/rand`.

### 4.2 Metadata

Instance metadata MUST be persisted as a JSON file at `.ast/instances/<instanceID>.json` with the following schema:

| Field         | Type              | Description                                         |
|---------------|-------------------|-----------------------------------------------------|
| `id`          | string            | Instance ID (8 hex chars).                          |
| `agentName`   | string            | Agent name from `astropods.yml`.                    |
| `projectName` | string            | Docker Compose project name (`agentName-instanceID`). |
| `composePath` | string            | Absolute path to the instance's compose file.       |
| `workingDir`  | string            | Absolute working directory at invocation time.      |
| `ports`       | array\<PortMapping\> | Allocated port mappings (Section 5.3).           |
| `createdAt`   | ISO 8601 string   | Timestamp of instance creation.                     |

### 4.3 Lifecycle

- **Created** when `ast dev` successfully starts containers.
- **Removed** when `ast dev stop` completes, when the `--local` mode Ctrl+C handler finishes cleanup, or lazily when a subcommand discovers the instance's Docker project has no running containers.
- The compose file (`.ast/docker-compose-<instanceID>.yml`) is deleted alongside the metadata file.

### 4.4 Active Instance Detection

An instance is **active** if its compose file exists on disk AND `docker compose -f <composePath> ps -q` returns at least one container ID. Stale instances (metadata exists but no containers running) MUST be cleaned up by removing the metadata file when discovered during enumeration.

---

## 5. Port Allocation

### 5.1 Preferred Ports

The compose builder generates a project with default published ports (e.g. playground → 3000, messaging HTTP → 3100, gRPC → 9090). These are treated as **preferred** values by the allocator.

### 5.2 Allocation Algorithm

For each service port mapping in the compose project:

1. Parse the `Published` field as the preferred host port. If empty, fall back to the `Target` port.
2. Starting from the preferred port, probe sequentially upward (port, port+1, ..., port+999).
3. Skip ports that are in the run's in-memory reserved set (prevents duplicate assignment within one invocation).
4. For each candidate, call `net.Listen("tcp", ":<port>")`. If the listen succeeds, the port is available — close the listener and record the port.
5. If no port is found within 1000 candidates, fail with a clear error naming the service and preferred port.
6. Write the resolved port back to the compose project's `Published` field before marshaling.

### 5.3 Port Mapping Schema

Each entry in the instance's `ports` array:

| Field           | Type    | Description                              |
|-----------------|---------|------------------------------------------|
| `service`       | string  | Compose service name (e.g. `playground`).|
| `targetPort`    | integer | Container-internal port.                 |
| `publishedPort` | integer | Host-published port (allocated).         |

### 5.4 TOCTOU Handling

Port probing is inherently racy — a port may become occupied between the probe and `docker compose up`. If `docker compose up` fails with a bind-conflict error (message contains "Bind for" AND "port is already allocated"), the CLI MUST:

1. Re-run the allocation algorithm (Section 5.2) against the current compose project.
2. Rewrite the compose file and instance metadata with new port mappings.
3. Retry `docker compose up` exactly once. If it fails again, surface the error normally.

---

## 6. Compose Isolation

### 6.1 Project Name

The Docker Compose project name MUST be `<agentName>-<instanceID>` when an instance ID is set. This ensures unique container prefixes, network names, and volume names across concurrent instances.

### 6.2 Compose File

Each instance writes its compose file to `.ast/docker-compose-<instanceID>.yml`. The old fixed path `.ast/docker-compose.yml` is no longer used.

### 6.3 Network

The compose project's network MUST NOT have an explicit `Name` field. When omitted, Docker Compose auto-generates a network name of the form `<projectName>_<networkKey>`, which is inherently unique per project.

### 6.4 Volumes

Named volumes retain their existing explicit names (e.g. `knowledge-docs-data`). Because each instance uses a unique project name, compose treats same-named volumes from different projects as shared — this preserves persistent data (knowledge stores) across restarts of the same agent while producing a harmless warning when two instances share a volume.

---

## 7. Subcommand Behavior

### 7.1 Instance Resolution

All subcommands that target a running instance (`logs`, `stop`, `trigger`, `ports`) MUST resolve the target instance using the following rules:

1. If `--instance <id>` is provided, match by exact ID against active instances. If no match, error with guidance to run `ast dev ports`.
2. If `--instance` is omitted and exactly one active instance exists, use it (preserves single-instance UX).
3. If `--instance` is omitted and multiple active instances exist, error with a list of active instance IDs and guidance to pass `--instance <id>` or run `ast dev ports`.
4. If no active instances exist, error with guidance to run `ast dev`.

### 7.2 `ast dev ports`

| Usage | Behavior |
|-------|----------|
| `ast dev ports` | Print all active instances with all port mappings. |
| `ast dev ports <service>` | Filter port mappings by service name across all active instances. |
| `ast dev ports --instance <id>` | Print port mappings for only the specified instance. |
| `ast dev ports --instance <id> <service>` | Filter by both instance and service. |

Output includes: agent name, instance ID, creation time, and for each port: service name, target port, published port, and a `(remapped)` indicator when the published port differs from the target.

### 7.3 `ast dev stop`

After `docker compose down` completes, `stop` MUST remove the instance metadata file and the compose file.

### 7.4 `ast dev logs` / `ast dev trigger`

These commands resolve the instance per Section 7.1 and pass the instance's `composePath` to `docker compose`. No other behavioral changes.

---

## 8. Local Mode (`--local`)

When running in `--local` mode, the local agent process MUST use port mappings from the instance's `ports` array for:

- `GRPC_SERVER_ADDR` — resolved from the `astro-messaging` service's published port for target 9090.
- Browser open URL — resolved from the `playground` service's published port for target 80.

On Ctrl+C shutdown, the `--local` cleanup handler MUST remove the instance metadata and compose file after running `docker compose down`.

---

## 9. Ready Block

The post-start summary box MUST display:

- The instance ID after the agent name (e.g. `✨ my-agent is ready  (abc12345)`).
- URLs using resolved published ports (not hardcoded defaults).
- `ast dev ports` as a listed command alongside `logs` and `stop`.

---

## 10. Builder Changes

The compose builder (`BuildOptions`) accepts an optional `InstanceID` string. When set:

- `project.Name` becomes `<agentName>-<instanceID>`.
- The `astro-dev` network has no explicit `Name` (auto-generated from project).

Port allocation happens AFTER the builder returns and BEFORE the compose file is written. The builder continues to emit preferred published ports; the allocator in `cmd/dev.go` resolves them to available host ports.

---

## 11. Validation Rules

1. Instance IDs MUST be exactly 8 lowercase hex characters.
2. The port search window MUST NOT exceed 1000 candidates per preferred port.
3. Allocated ports MUST be in the range 1–65535.
4. No two port mappings within a single instance MAY share the same published port.
5. `--instance` matching MUST be exact — the provided value MUST equal a full instance ID.

---

## 12. Files

| File | Change |
|------|--------|
| `apps/astro-cli/cmd/instance.go` | New — instance model, persistence, port allocator |
| `apps/astro-cli/cmd/dev.go` | Instance lifecycle, auto-allocation wiring, `dev ports` command, instance-aware subcommands, ready block changes |
| `apps/astro-cli/internal/compose/builder.go` | `InstanceID` in `BuildOptions`, unique project name, auto-generated network name |
| `apps/astro-cli/cmd/dev_test.go` | Tests for allocator, instance resolution, port lookup |
| `apps/astro-cli/internal/compose/builder_test.go` | Tests for instance-scoped project naming |

---

## 13. Migration

- The old fixed compose path `.ast/docker-compose.yml` is no longer written. Existing users who have a running `ast dev` session from a previous CLI version will need to run `docker compose -f .ast/docker-compose.yml down` manually or let `docker compose down` handle it on next `ast dev stop`.
- On first run of the new CLI, the `.ast/instances.db` database is created automatically. No manual setup required.
- Single-instance workflows (`ast dev` → `ast dev logs` → `ast dev stop`) are fully backward-compatible. The same-directory guard defaults to restart, matching current behavior.
