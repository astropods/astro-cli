# Deployment access domain

## Summary

Astro now has a transport-independent domain layer for deployment access. It defines Viewer, Builder, and Admin and records access changes as durable desired state without adding HTTP routes or changing authorization behavior.

## Design

The role catalog maps product-facing access levels to the existing WorkOS role slugs and documents the permissions each built-in role is expected to contain. Permissions remain the authorization contract; roles remain replaceable bundles.

The access service validates that the deployment, organization, and target member share one tenant. Access listings resolve all distinct memberships in one database query and omit unsupported, unresolved, or foreign rows with structured warnings, so one stale mirror row cannot hide valid access.

Mutations upsert one versioned intent per subject and resource instead of holding a database lock across WorkOS calls. One resource-scoped job batches pending membership intents and lists WorkOS assignments once before applying each version independently; group intents use the group-specific API. A background reconciler removes stale direct built-in roles, applies the newest requested role, and marks that exact version synced. Partial WorkOS failures remain visible as retrying with the last error and recover through bounded backoff plus a one-minute repair sweep. A missing membership resource is discarded only when Astro's deployment lifecycle ledger confirms deletion, while a missing group discards that group's stale intent. A resource still being provisioned remains retryable. A newer request wins through version checks; custom and group-derived assignments remain untouched. WorkOS remains authoritative for effective access while Astro owns the operation ledger.

PR8.3B will expose this service through audited, `deployment:manage_access`-protected Astro APIs.

## Flow and review guide

1. **Define the access contract.** `apps/astro-server/internal/authz/access_catalog.go` maps Viewer, Builder, and Admin to the external WorkOS role slugs and their expected permissions. `apps/astro-server/internal/authz/fga.go` defines the shared assignment subject, source, and listing interfaces. Their invariants are covered by `apps/astro-server/internal/authz/access_catalog_test.go`.

2. **Validate and record a request.** `apps/astro-server/internal/authz/access_service.go` verifies that the deployment, organization, and target membership share one tenant, batches membership resolution, and converts a requested access level into desired state. `apps/astro-server/internal/account/store.go` provides the context-aware single and batch membership lookups. `apps/astro-server/internal/authz/access_service_test.go` and `apps/astro-server/internal/account/store_test.go` cover missing mirrors, database failures, foreign memberships, groups, custom roles, and no-op requests.

3. **Persist durable intent.** `apps/astro-server/internal/authz/access_sync.go` owns the versioned operation ledger: record the newest desired role, find due work, mark only the matching version synced, or retain a failure for retry. `sql/astro-server/schema.sql` defines the generic `resource_access_fga_sync` table and indexes. `apps/astro-server/internal/authz/access_sync_test.go` covers version ordering, idempotency, retry state, and concurrent no-op records.

4. **Converge WorkOS to the desired state.** `apps/astro-server/internal/authz/access_reconciler.go` reads the current direct assignments, removes stale built-in roles, applies the requested role, and leaves custom or group-derived access untouched. `apps/astro-server/internal/authz/workos_fga.go` translates those operations to the official WorkOS SDK and classifies idempotent and missing-resource outcomes. `apps/astro-server/internal/authz/access_reconciler_test.go` and `apps/astro-server/internal/authz/workos_fga_test.go` cover partial failure, recovery, newer-version races, and deleted resources.

5. **Run immediately and repair automatically.** `apps/astro-server/internal/riverqueue/resource_access_fga.go` executes one resource-scoped batch or fans out due work once per resource. `apps/astro-server/internal/riverqueue/client.go` carries the sync dependencies, `apps/astro-server/internal/riverqueue/workers.go` registers the worker only when configured, and `apps/astro-server/internal/riverqueue/periodic.go` adds the one-minute repair sweep. `apps/astro-server/internal/riverqueue/resource_access_fga_test.go` covers batched reconciliation, sweep behavior, and disabled wiring.

6. **Keep the rollout contract explicit.** `docs/01-spec/deployment-fgac-rollout.md` places this durable access service in the wider FGAC sequence. This changelog records the PR boundary: domain and reconciliation only; HTTP routing, authorization, and audit events arrive in PR8.3B.

## Migration

The new access-intent table is created with the normal database schema rollout. This PR adds no routes and is inactive until PR8.3B wires its first consumer.
