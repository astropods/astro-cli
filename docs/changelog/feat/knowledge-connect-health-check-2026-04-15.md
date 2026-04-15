# Health check on knowledge connect + PrivateLink integration

## Summary

External knowledge stores were marked `ready` immediately on connect with no connectivity verification. Bad credentials or unreachable hosts silently succeeded. Additionally, PrivateLink required separate attach/detach commands even though the VPC endpoint service name is the same as the host.

## Design

**Health check** — After the store record is created, `CheckHealth` attempts a provider-specific connectivity test using the plaintext credentials (before encryption). Per-provider strategy:

| Provider | Method |
|----------|--------|
| postgres | `pgx.Connect` + `Ping` |
| redis | `go-redis` `Ping` |
| qdrant | HTTP GET `/healthz` |
| neo4j | HTTP GET `/` (port 7474) or TCP dial |
| pinecone | HTTPS GET with `Api-Key` header |
| default | TCP dial |

If the check fails, the store is set to `status=error` with the failure message. The store still exists so users can inspect, delete, or retry. `--skip-health-check` bypasses the check entirely.

**PrivateLink integration** — `ast knowledge connect --private-link` replaces the old `ast knowledge privatelink attach/detach` subcommands. The `--host` flag doubles as the VPC endpoint service name (e.g., `com.amazonaws.vpce.us-east-1.vpce-svc-xxx`) and the region is auto-parsed from it. When `--private-link` is set, the health check is skipped (DB isn't reachable until the endpoint is accepted), the endpoint record is created, and the CLI polls for PrivateLink readiness.

The separate `POST/DELETE /knowledge/:name/privatelink` API endpoints and the `privatelink attach/detach` CLI subcommands are removed.

## Migration

- Replace `ast knowledge privatelink attach --store <name> --service <svc> --region <region>` with `ast knowledge connect --private-link --host <svc> ...`
- The `privatelink detach` command no longer exists. Delete and recreate the store to remove PrivateLink.
