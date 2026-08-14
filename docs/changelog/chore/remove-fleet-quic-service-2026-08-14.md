# Remove the Fleet QUIC device-connectivity service

## Summary

Removes `ast connect`, the gRPC-over-QUIC device-connectivity service (`ConnectService` on UDP port 9092) that let the CLI register a device, heartbeat, and receive server-pushed shell commands. The admin console's "Connected Devices" page and the `SendCommand`/`ListConnectedDevices` admin RPCs depended on this transport and are removed with it.

This does not touch the WorkOS device-authorization flow used by `ast login` (`GetAuthConfig`, `/api/auth/device*`), which is a separate, unrelated feature that happens to also involve a "device."

## Design

The feature was self-contained behind two packages in astro-server:

- `internal/connectgrpc` — the QUIC listener (including the AWS NLB QUIC-LB connection-ID generator), TLS setup, JWT stream auth, and the `ConnectService` implementation.
- `internal/devicestore` — the `connected_devices` table accessor.

Removing it meant unwinding the wiring in `main.go` (`startFleetGRPCServer`, the `FleetGRPCConfig` block in `internal/config`), the `CommandDispatcher` seam in `admingrpc.Server` that let admin dispatch shell commands to a connected device, and the hand-maintained `admin.pb.go`/`admin_grpc.pb.go` JSON-over-gRPC stubs for `ConnectedDevice`/`SendCommand*`/`ListConnectedDevices*` (this repo has no `buf.gen.yaml`, so these are edited by hand per their own file header). The `connect.proto` package and its generated Go are deleted outright since nothing else referenced them.

On the admin console (astro-queen), the "Devices" nav item, its route, the `connected-devices.tsx` page, and the proxy handlers for `/api/admin/devices*` are removed.

The `connected_devices` table is dropped from `sql/astro-server/schema.sql`, the Atlas-managed declarative schema; Atlas applies the `DROP TABLE` on the next diff.

A design spec for a future private-knowledge-store connector tunnel (`docs/01-spec/knowledge-store-private-connectivity.md`) explicitly built on top of this transport ("reuses this entire transport stack, no new port, no new NLB") is also removed, since its foundation no longer exists.

## Migration

None for running deployments — the `FLEET_GRPC_PORT`/`FLEET_TLS_CERT_PATH`/`FLEET_TLS_KEY_PATH` env vars are now unused and can be dropped from deployment config whenever convenient. No client ever shipped a working `ast connect` command, so there are no devices to migrate off the feature.
