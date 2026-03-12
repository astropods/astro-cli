# Auto Port Allocation & Multi-Instance `ast dev`

**Date:** 2026-03-06
**Scope:** `apps/astro-cli`

## Summary

Replaced manual port offsets with automatic per-service port allocation and true multi-instance local dev support. Multiple `ast dev` invocations (same or different agents, same directory) can now run concurrently without port conflicts or Docker object collisions.

## Changes

### Auto port allocation
- Each `ast dev` probes preferred host ports and bumps to the next available when occupied.
- TOCTOU retry: if `docker compose up` still hits a bind conflict, ports are reallocated once and startup retried.

### Instance isolation
- Every `ast dev` gets a unique 8-character instance ID.
- Compose file written to `.ast/docker-compose-<id>.yml`.
- Docker project name includes instance ID (`agentName-id`), giving each run its own network namespace.
- Instance metadata persisted to `.ast/instances/<id>.json` and cleaned up on `stop`/shutdown.

### New command: `ast dev ports`
- `ast dev ports` — list all active instances and every mapped port.
- `ast dev ports <service>` — filter by service name.
- Shows remapped ports clearly (preferred vs actual).

### Instance-aware subcommands
- `logs`, `stop`, `trigger`, `ports` auto-resolve when one instance is running.
- When multiple are running, `--instance <id>` is required; the error message lists active instances with guidance.

### Removed
- `--port-offset` flag (never shipped to users).

## Files changed

| File | What |
|------|------|
| `apps/astro-cli/cmd/instance.go` | New — instance model, save/load/list/resolve, port allocator |
| `apps/astro-cli/cmd/dev.go` | Instance lifecycle, auto-allocation wiring, `dev ports` command, instance-aware subcommands |
| `apps/astro-cli/internal/compose/builder.go` | `InstanceID` in `BuildOptions`, unique project/network names |
| `apps/astro-cli/cmd/dev_test.go` | Tests for port allocator, instance resolution, port reporting |
| `apps/astro-cli/internal/compose/builder_test.go` | Test for instance-scoped project name |
| `docs/02-cli/cli-design.md` | Updated `ast dev` docs with multi-instance and `dev ports` |
| `apps/astro-cli/cmd/docs/ast.md` | Updated user-facing docs |
