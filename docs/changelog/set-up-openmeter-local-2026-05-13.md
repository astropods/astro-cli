# Local Dev: Real OpenMeter + Queen Admin

## Summary

Local development previously used a fake in-memory OpenMeter stub that hardcoded five meters and returned seed data. This meant local dev diverged from production behavior — entitlement enforcement, meter aggregation, and plan limits were all simulated. This change replaces the fake with a real OpenMeter instance and adds bootstrap tooling to seed it, and wires up `queen local admin` so the admin UI can connect to a locally running server.

## Design

### Removing fake OpenMeter

`apps/astro-server/cmd/fakeopenmeter/` is deleted. The `dev.sh` startup logic that launched it is removed along with the associated PID cleanup.

### Bootstrap script

`scripts/bootstrap-openmeter.sh` seeds a real OpenMeter instance with all nine meters, nine features, and the `private_beta` plan as defined in `docs/03-architecture/openmeter-integration.md`. It is idempotent — 409 responses (already exists) are treated as success, so it is safe to re-run.

The script reads `OPENMETER_URL` from the environment and exits with a notice if it is unset:

```
OPENMETER_URL=https://... bash scripts/bootstrap-openmeter.sh
```

`scripts/local-dev.sh` calls the bootstrap once at startup after Docker services are up, before the server starts. This ensures meters exist before `astro-server`'s `ValidateMeters` check on startup.

### Queen local admin

`queen local admin` connects to the local `astro-server` admin gRPC on `localhost:9091`.

Two changes make this work:

- **Server side** (`apps/astro-server/main.go`): `startAdminGRPCServer` now starts with `insecure.NewCredentials()` when `ENVIRONMENT=local` instead of refusing to start when mTLS certs are absent.
- **Client side** (`apps/astro-queen`): `Config` gains an `Insecure bool` field (runtime-only, `yaml:"-"`), set to `true` for the `local` environment in `cmd/server.go`. `client.New` skips mTLS cert loading when `cfg.Insecure` is true. This is safe — the server only binds insecurely on loopback when `ENVIRONMENT=local`.

## Migration

- Set `OPENMETER_URL` in `apps/astro-server/.env` to your OpenMeter instance URL.
- Run `bash scripts/bootstrap-openmeter.sh` once (or let `local-dev.sh` do it on next startup).
- Run `queen local admin` to open the admin UI against local data.
