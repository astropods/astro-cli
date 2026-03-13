# Multi-Instance Local Dev Spec

**Version:** 2.0
**Date:** 2026-03-12
**Status:** Draft

## Abstract

This spec defines automatic host-port allocation and instance isolation for `ast dev`, enabling multiple concurrent agent instances from the same or different directories without manual configuration. Each invocation gets a unique instance ID, its own Docker project, and auto-assigned host ports from a reserved range. Instance metadata is stored in an embedded SQLite database. Subcommands (`logs`, `stop`, `trigger`, `ports`) resolve the target instance automatically when unambiguous or require explicit selection when multiple are active.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Problem

The existing `ast dev` writes a single `.ast/docker-compose.yml` with hardcoded host-published ports. Running a second instance from the same directory overwrites that file and collides on port bindings, Docker project names, and network/volume names. Running from a different directory avoids the file collision but still conflicts on shared ports (3000, 3100, 9090, etc.). There is no mechanism to discover which ports a running instance is using, and no guard against accidentally spawning duplicate instances from the same directory.

---

## 2. Goals

1. **Zero-config multi-instance** — `ast dev` MUST run multiple instances concurrently (same directory, different directories) without user-specified port offsets.
2. **Automatic port allocation** — each published port MUST be allocated from a reserved range using block-based assignment.
3. **Instance isolation** — each invocation MUST use a unique Docker Compose project name so container, network, and volume namespaces do not collide.
4. **Same-directory safety** — when an active instance already exists from the same working directory, `ast dev` MUST prompt the user before creating an additional instance (Section 4.6).
5. **Subcommand ergonomics** — `logs`, `stop`, `trigger` MUST continue to work without extra flags when a single instance is active. When multiple are active, they MUST require explicit instance selection and provide actionable guidance.
6. **Port visibility** — a new `ast dev ports` command MUST display all active instances and their port mappings, with optional service-name and instance-ID filtering.

## 3. Non-Goals

1. **Remote instance discovery** — only local instances (same machine) are tracked.
2. **Persistent data sharing** — volume sharing across instances is configurable (Section 6.3) but defaults to isolated.
3. **Automatic instance cleanup daemon** — stale metadata is cleaned up lazily when subcommands enumerate instances, not by a background process.

---

## 4. Instance Model

### 4.1 Instance ID

Each `ast dev` invocation MUST generate a unique instance ID. The ID is an 8-character lowercase hex string produced from 4 bytes of `crypto/rand`.

### 4.2 Metadata Store

Instance metadata MUST be persisted in an embedded SQLite database at `.ast/instances.db`. SQLite is preferred over per-instance JSON files because it provides atomic writes, structured queries, and resilience against partial-write corruption.

The database MUST contain an `instances` table with the following schema:

| Column        | Type    | Description                                                     |
|---------------|---------|-----------------------------------------------------------------|
| `id`          | TEXT PK | Instance ID (8 hex chars).                                      |
| `agent_name`  | TEXT    | Agent name from `astropods.yml`.                                |
| `project_name`| TEXT    | Docker Compose project name (`agentName-instanceID`).           |
| `working_dir` | TEXT    | Absolute working directory at invocation time.                  |
| `created_at`  | TEXT    | ISO 8601 timestamp of instance creation.                        |

And a `port_mappings` table:

| Column          | Type    | Description                                        |
|-----------------|---------|----------------------------------------------------|
| `instance_id`   | TEXT FK | References `instances.id`.                         |
| `service`       | TEXT    | Compose service name (e.g. `playground`).          |
| `target_port`   | INTEGER | Container-internal port.                           |
| `published_port` | INTEGER | Host-published port (allocated).                  |

On `ast dev stop` or cleanup, the instance row and its port mappings MUST be deleted from the database. Stale rows (where the Docker project has no running containers) MUST be cleaned up lazily during enumeration.

### 4.3 Docker SDK

Container orchestration SHOULD use the Docker SDK (Go client) directly rather than generating and shelling out to `docker-compose` YAML files. This eliminates an entire class of file-management bugs (stale compose files, partial writes, path resolution) and gives the CLI direct control over container lifecycle, port bindings, and health checks through the Docker API.

If the initial implementation retains compose file generation as a transitional step, each instance MUST write its compose file to `.ast/docker-compose-<instanceID>.yml`, and these files MUST be deleted alongside the database row on cleanup.

### 4.4 Lifecycle

- **Created** when `ast dev` successfully starts containers and writes a row to the instances database.
- **Removed** when `ast dev stop` completes, when the `--local` mode Ctrl+C handler finishes cleanup, or lazily when a subcommand discovers the instance's Docker project has no running containers.

### 4.5 Active Instance Detection

An instance is **active** if its database row exists AND `docker compose -p <projectName> ps -q` (or the equivalent Docker SDK call) returns at least one container ID. Stale instances (row exists but no containers running) MUST be cleaned up by deleting the row when discovered during enumeration.

### 4.6 Same-Directory Guard

Before generating a new instance ID, `ast dev` MUST query the instances database for active instances whose `working_dir` matches the current working directory.

**If one or more active instances exist from the same directory:**

1. If `--new` is passed, skip the guard and proceed to create a new instance.
2. If the terminal is interactive (TTY attached) and exactly one active instance exists, prompt:
   ```
   Agent "my-agent" is already running (instance abc12345).
     [r] Restart it (default)
     [n] Start a new instance
     [q] Quit
   ```
   Default selection is **restart**. "Restart" means: tear down the existing instance (`docker compose down` or SDK equivalent, delete its database row), then proceed with a fresh instance ID and allocation.
3. If the terminal is interactive and multiple active instances exist from this directory, list them and prompt:
   ```
   Multiple instances of "my-agent" are running:
     abc12345  created 2026-03-12T10:00:00Z  (ports: 11000-11009)
     def67890  created 2026-03-12T11:30:00Z  (ports: 11010-11019)

     [1] Restart abc12345
     [2] Restart def67890
     [n] Start a new instance
     [q] Quit
   ```
4. If the terminal is non-interactive (no TTY), default to **restart** of the most recently created instance. Pass `--new` to override.

This preserves the current single-instance "restart" mental model by default while allowing intentional multi-instance via `--new`.

---

## 5. Port Allocation

### 5.1 Reserved Range

The CLI reserves the host port range **11000–11999** (1000 ports) for `ast dev` instances. This range is divided into contiguous **blocks of 10 ports** each, yielding 100 blocks (11000–11009, 11010–11019, ..., 11990–11999).

### 5.2 Block Allocation Algorithm

When a new instance starts:

1. Query the instances database for all `published_port` values currently in use by active instances.
2. Starting from the first block (11000), find the first block where **all 10 ports** are unoccupied — not claimed by an active instance in the database AND not bound by any other process on the host.
3. To verify host availability, call `net.Listen("tcp", ":<port>")` for each port in the candidate block. If any port in the block is occupied, skip the entire block and try the next one.
4. Once a free block is found, assign ports sequentially within it: the first service port mapping gets the first port in the block, the second gets the next, and so on.
5. If all 100 blocks are exhausted, fail with a clear error: `"No available port block in range 11000-11999. Run 'ast dev ports' to see active instances or 'ast dev stop' to free ports."`.

### 5.3 Block Overflow

A typical agent uses 3–5 ports. If an agent's compose project declares more than 10 published ports, the allocator MUST claim additional contiguous blocks as needed. The instance's port mappings in the database will span multiple blocks, and those blocks are all considered occupied for future allocations.

### 5.4 Port Mapping Schema

Each row in the `port_mappings` table records:

| Field            | Type    | Description                                |
|------------------|---------|--------------------------------------------|
| `instance_id`    | TEXT    | FK to `instances.id`.                      |
| `service`        | TEXT    | Compose service name (e.g. `playground`).  |
| `target_port`    | INTEGER | Container-internal port.                   |
| `published_port` | INTEGER | Host-published port from the reserved range.|

### 5.5 TOCTOU Handling

Port probing is inherently racy — a port may become occupied between the probe and container start. If container startup fails with a bind-conflict error (message contains "Bind for" AND "port is already allocated"):

1. Re-run the block allocation algorithm (Section 5.2).
2. Update the instance's port mappings in the database.
3. Retry container startup exactly once. If it fails again, surface the error normally.

---

## 6. Compose Isolation

### 6.1 Project Name

The Docker Compose project name (or equivalent label when using the Docker SDK) MUST be `<agentName>-<instanceID>`. This ensures unique container prefixes, network names, and volume names across concurrent instances.

### 6.2 Network

The compose project's network MUST NOT have an explicit `Name` field. When omitted, Docker Compose auto-generates a network name of the form `<projectName>_<networkKey>`, which is inherently unique per project. When using the Docker SDK, the CLI MUST create a network named `<projectName>_<networkKey>` following the same convention.

### 6.3 Volumes

Volume sharing across instances is **configurable** via `astropods.yml`:

```yaml
dev:
  volumes:
    shared: true   # all named volumes are shared across instances (default: false)
```

- **`shared: false`** (default) — named volumes are prefixed with the project name (`<projectName>_<volumeName>`), giving each instance fully isolated storage.
- **`shared: true`** — named volumes retain their explicit names (e.g. `knowledge-docs-data`) and are shared across all instances of the same agent. This is useful for large knowledge stores that are expensive to rebuild.

When `shared: true`, concurrent instances reading and writing the same volume may produce data races. This is the user's responsibility to manage; the CLI SHOULD print a warning at startup when shared volumes are enabled and multiple instances are active.

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

Output includes: agent name, instance ID, creation time, port block range, and for each port: service name, target port, and published port.

### 7.3 `ast dev stop`

| Usage | Behavior |
|-------|----------|
| `ast dev stop` | Resolve instance per Section 7.1, tear down, delete database row. |
| `ast dev stop --all` | Tear down **all** active instances, delete all rows from the database. Instances are stopped in parallel. |
| `ast dev stop --instance <id>` | Tear down the specified instance, delete its database row. |

After container teardown completes, `stop` MUST delete the instance's row and port mappings from the database. If a transitional compose file exists, it MUST be deleted as well.

### 7.4 `ast dev logs` / `ast dev trigger`

These commands resolve the instance per Section 7.1 and target the resolved instance's Docker project. No other behavioral changes.

---

## 8. Local Mode (`--local`)

When running in `--local` mode, the local agent process MUST use port mappings from the instance's database row for:

- `GRPC_SERVER_ADDR` — resolved from the `astro-messaging` service's published port for target 9090.
- Browser open URL — resolved from the `playground` service's published port for target 80.

On Ctrl+C shutdown, the `--local` cleanup handler MUST delete the instance's database row after tearing down containers.

---

## 9. Ready Block

The post-start summary box MUST display:

- The instance ID after the agent name (e.g. `✨ my-agent is ready  (abc12345)`).
- The allocated port block (e.g. `ports: 11000-11009`).
- URLs using resolved published ports (not hardcoded defaults).
- `ast dev ports` as a listed command alongside `logs` and `stop`.

---

## 10. Builder Changes

The compose builder (`BuildOptions`) accepts an optional `InstanceID` string. When set:

- `project.Name` becomes `<agentName>-<instanceID>`.
- The `astro-dev` network has no explicit `Name` (auto-generated from project).

The builder continues to emit preferred target ports. The port allocator in `cmd/dev.go` maps these to published ports from the reserved range after the builder returns and before containers are started.

When migrating to the Docker SDK, the builder's role shifts from generating YAML to producing a structured container/network/volume specification that the SDK executor consumes directly.

---

## 11. Validation Rules

1. Instance IDs MUST be exactly 8 lowercase hex characters.
2. Port allocations MUST fall within the reserved range 11000–11999.
3. No two port mappings across any active instances MAY share the same published port.
4. `--instance` matching MUST be exact — the provided value MUST equal a full instance ID.
5. The instances database MUST use WAL mode for safe concurrent reads during enumeration.

---

## 12. Files

| File | Change |
|------|--------|
| `apps/astro-cli/cmd/instance.go` | New — instance model, SQLite schema, port block allocator, same-directory guard |
| `apps/astro-cli/cmd/dev.go` | Instance lifecycle, auto-allocation wiring, `dev ports` command, `stop --all`, instance-aware subcommands, ready block changes |
| `apps/astro-cli/internal/compose/builder.go` | `InstanceID` in `BuildOptions`, unique project name, auto-generated network name, configurable volume sharing |
| `apps/astro-cli/cmd/dev_test.go` | Tests for block allocator, instance resolution, same-directory guard, port lookup |
| `apps/astro-cli/internal/compose/builder_test.go` | Tests for instance-scoped project naming, volume sharing modes |

---

## 13. Migration

- The old fixed compose path `.ast/docker-compose.yml` is no longer written. Existing users who have a running `ast dev` session from a previous CLI version will need to run `docker compose -f .ast/docker-compose.yml down` manually or let `docker compose down` handle it on next `ast dev stop`.
- On first run of the new CLI, the `.ast/instances.db` database is created automatically. No manual setup required.
- Single-instance workflows (`ast dev` → `ast dev logs` → `ast dev stop`) are fully backward-compatible — no extra flags required. The same-directory guard defaults to restart, matching current behavior.
