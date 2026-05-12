# MySQL knowledge store (external)

## Summary

Adds MySQL as a supported knowledge store provider, alongside Postgres, in **external (bring-your-own) mode only**. Users can now connect an existing MySQL database under an Astro ARN; managed (platform-provisioned) MySQL is not yet offered.

## Design

The knowledge store stack was already provider-pluggable — each provider declares itself once in the `astro-spec` builtin registry and the rest of the platform (handlers, healthcheck, deployer credential resolution, client UI) drives off that single source of truth. MySQL fits cleanly into this shape; this change is a registry entry plus a real driver-level healthcheck.

- **Registry entry** (`packages/astro-spec/provider.go`): `mysql` is registered under section `knowledge` with image `mysql:8.0`, default port `3306`, env prefix `MYSQL`, and bind credentials `user → MYSQL_USER`, `password → MYSQL_PASSWORD`, `database → MYSQL_DATABASE`. The image and exec health probe are pre-wired so that flipping the UI to expose managed mode later is a one-line change. `Cloud` is left `false` because MySQL is a self-hostable database, not a SaaS.
- **Healthcheck** (`apps/astro-server/internal/knowledgestore/healthcheck.go`): a new `checkMySQL` path uses `github.com/go-sql-driver/mysql` and `db.PingContext` to verify connectivity at connect time, mirroring the existing `checkPostgres` (pgx) path. Falling back to a plain TCP dial would have accepted a reachable port that wasn't actually MySQL or that rejected the supplied credentials.
- **Client gating** (`apps/astro-client/src/components/knowledge/knowledge-utils.ts`): MySQL is added to `EXTERNAL_PROVIDERS` and `ALL_PROVIDERS`, but deliberately **not** to `MANAGED_PROVIDERS`. The UI is the gate for managed mode; the server's `LookupBuiltin` check happily accepts `mysql` for the connect-external path. The `PROVIDER_FIELDS`, `PROVIDER_PORTS`, `PROVIDER_LABELS`, and icon mappings for MySQL were already in place from a prior scaffolding pass — connecting them through the provider lists is the final step.

External MySQL credentials are stored as `HOST`, `PORT`, `DATABASE`, `USERNAME`, `PASSWORD` (matching `ExternalCredentialKeys("mysql")`, which already existed). KMS encryption and the connect/health-check/PrivateLink flows are reused unchanged.

## Migration

No action required. Existing knowledge stores are unaffected. MySQL becomes selectable in the "New knowledge store" flow and the connect dialog. The `go-sql-driver/mysql` Go dependency is added to `apps/astro-server`.
