# Plan: Multi-Deployment Support

## Context

Currently, each agent can only have one active deployment per account. The namespace is derived from `SHA256(accountID:sourceAccount:agentName)`, and the DB enforces a unique index on `(account_id, agent_name) WHERE status = 'active'`. This prevents running multiple instances of the same agent (e.g. staging vs production, or per-customer deployments).

## Part 1: Namespace Scan & Association

### Problem

Existing deployments have namespaces created via `SHA256(accountID:sourceAccount:agentName)` but there's no explicit mapping from namespace back to account/user ownership outside of K8s labels and the deployments table. Before we can support multiple deployments per agent, we need a clear inventory of what's running and who owns it.

### Design

On server startup, scan all active deployments from the DB and reconcile them against the K8s cluster. Build a tracked association of namespace → account ownership that serves as the foundation for migration.

**New table in `schema.sql`:**
```sql
CREATE TABLE public.namespace_ownership (
    namespace varchar NOT NULL,
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    deployment_id uuid,
    source_account text NOT NULL DEFAULT '',
    scanned_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT namespace_ownership_pkey PRIMARY KEY (namespace),
    CONSTRAINT namespace_ownership_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id),
    CONSTRAINT namespace_ownership_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id)
);

CREATE INDEX idx_namespace_ownership_account ON public.namespace_ownership(account_id);
```

**New package:** `apps/astro-server/internal/nsscan/`

**Scanner** (`nsscan/scanner.go`):

The scanner is composed of separable concerns: a **steady-state reconciler** (permanent) and **migration hooks** (temporary). The migration logic is isolated so it can be deleted cleanly once all deployments are migrated.

- `type Scanner struct` — holds `*sql.DB`, K8s client, logger, and an optional `[]ScanHook`
- `type ScanHook func(ctx context.Context, result *ScanResult) error` — extension point for migration-specific logic

**Core (permanent)** — `Scan(ctx context.Context) (*ScanResult, error)`:
  1. List K8s namespaces with label `app.kubernetes.io/managed-by=astro-server`
  2. Query all active deployments from `deployments` table
  3. Cross-reference: build sets of DB namespaces vs K8s namespaces
  4. Upsert into `namespace_ownership` (ON CONFLICT UPDATE `scanned_at`)
  5. Detect orphaned (in K8s, not in DB), stale (in DB, not in K8s), drifted (`namespace_ownership` rows not refreshed)
  6. Run any registered `ScanHook`s with the result

This is the steady-state loop — it stays forever. It keeps `namespace_ownership` fresh and catches drift.

**Migration hook (temporary)** — registered via `scanner.AddHook(migrationHook)`:
  - Lives in a separate file: `nsscan/migrate_hook.go`
  - On each scan, checks for deployments that still use the legacy namespace hash
  - Tracks migration state per deployment (pending → in-progress → done)
  - Once all legacy deployments are migrated, the hook no-ops
  - **To remove after migration:** delete `migrate_hook.go` and remove the `AddHook` call in `main.go`. Scanner continues working unchanged.

**Lifecycle:**
- `Start(ctx context.Context, interval time.Duration)` — runs `Scan` once immediately, then on a ticker. Returns immediately; respects ctx cancellation.
- `Stop()` — signals the background loop to stop

- `ScanResult`:
  - `Tracked int` — namespaces successfully associated
  - `Orphaned []string` — K8s namespaces with no DB record
  - `Stale []string` — DB records whose K8s namespace no longer exists
  - `Drifted []string` — `namespace_ownership` rows not refreshed this scan

**Integration** (`main.go`):
```go
scanner := nsscan.New(db, k8sClient, log)
scanner.AddHook(nsscan.MigrationHook(db, log)) // TEMPORARY — remove after migration
scanner.Start(ctx, 10*time.Minute)
defer scanner.Stop()
```
- Initial scan is **non-fatal** — server still starts if scan fails (e.g. K8s unreachable in dev mode)
- Periodic scan failures are logged but don't crash the server
- After migration is complete: delete `migrate_hook.go`, remove the `AddHook` line

### Files to create/modify
- **Create:** `apps/astro-server/internal/nsscan/scanner.go` — core scanner (permanent)
- **Create:** `apps/astro-server/internal/nsscan/migrate_hook.go` — migration hook (temporary, delete after migration)
- **Modify:** `apps/astro-server/schema.sql` — add `namespace_ownership` table
- **Modify:** `apps/astro-server/main.go` — start scanner with hook

---

## Part 2: Multi-Deployment Schema

### Schema Changes (`schema.sql`)

All changes are declarative in `schema.sql` — Atlas diffs and applies automatically.

**Updated `deployments` table (final state):**
```sql
CREATE TABLE public.deployments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    namespace varchar NOT NULL,
    display_name varchar(64) NOT NULL DEFAULT '',
    deployment_spec_json text NOT NULL,
    status varchar NOT NULL DEFAULT 'active',
    deployed_at timestamp NOT NULL DEFAULT now(),
    undeployed_at timestamp,
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id)
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

CREATE UNIQUE INDEX idx_deployments_active_display_name ON public.deployments(account_id, display_name) WHERE status = 'active' AND display_name != '';
```

Key changes from current schema:
- **Removed** `idx_deployments_active_agent` unique index — no longer one-deployment-per-agent. Multiple active deployments of the same agent are allowed.
- **Added** `display_name` column — user-facing name, unique per account among active deployments (when non-empty)
- **Kept** `idx_deployments_account_agent` as a non-unique index for lookups

### Namespace Generation (`handlers/deploy.go`)

The `deploymentNamespace` hash function is **removed**. Instead, namespace is derived from the deployment's UUID:

```go
func deploymentNamespace(deploymentID string) string {
    return "astro-" + strings.ReplaceAll(deploymentID, "-", "")[:20]
}
```

- **New deployments:** generate the deployment UUID upfront (`uuid.New()`), derive namespace from it, then insert the row with that ID
- **Redeploys:** look up the existing active deployment's namespace from DB (existing `resolveDeploymentNamespace` pattern) — the namespace is stable across builds because we reuse the same row's namespace

This eliminates the hash function and the `sourceAccount` dependency in namespace derivation.

### Deployment Store Changes (`internal/deploymentstore/store.go`)

- `SaveDeployment(id, accountID, agentName, displayName, buildID, namespace, specJSON)` — accepts pre-generated UUID
- `GetActiveDeployment(accountID, agentName)` — unchanged signature, but now may return multiple rows; rename to `GetActiveDeployments` returning `[]*Deployment`
- `GetActiveDeploymentByDisplayName(accountID, displayName)` — new, for lookups/undeploy by display name
- `GetDeploymentHistory(accountID, agentName)` — unchanged
- `MarkUndeployed(deploymentID)` — by ID instead of by agent name (since multiple active deployments exist)
- `ListAllActive()` — add `display_name` to SELECT and scan
- `Deployment` struct gains `DisplayName string`

### Deploy Handler Changes (`handlers/deploy.go`)

- `prepareDeployment()` reads `displayName` from spec's `Target.DisplayName`
- Validates display name (max 64 chars, no control chars, trimmed)
- Uniqueness pre-check: query for active deployment in same account with same display name → 409 if conflict
- For new deployment: generate UUID, derive namespace from it
- For redeploy (same agent + display name already active): reuse existing namespace
- `deployContext` struct gains `displayName string`, `deploymentID string`
- Namespace label: `"astro.dev/display-name": displayName`
- `SaveDeployment` call passes pre-generated ID

### Undeploy Handler Changes (`handlers/deploy.go`)

- Undeploy request body uses `display_name` to identify which deployment to undeploy (instead of assuming one-per-agent)
- `MarkUndeployed` called with deployment ID
- If display name is empty/omitted, undeploy the sole active deployment for the agent (backward compat); error if multiple exist

### Spec Changes (`packages/astro-spec/`)

- `DeploymentTarget.DisplayName string` field (`json:"display_name" yaml:"display_name"`) — already added by PR #266
- Validation: optional, max 64 chars, no control chars

### API Routes

No route changes — display name travels in the request body/spec.

### Admin gRPC / Queen

- `ListAllActive` response already includes full `Deployment` struct — adding `display_name` to scan is sufficient
- Queen TUI displays it as an extra column

### Frontend (`astro-client`)

- Deploy form: "Agent Name" field → sends `target.display_name` (already in PR #266)
- Deployment list: show display name with fallback to agent slug (already in PR #266)
- Undeploy: pass display name to identify deployment
- Handle 409 on deploy for duplicate display name

### Files to modify
- `apps/astro-server/schema.sql`
- `apps/astro-server/internal/deploymentstore/store.go`
- `apps/astro-server/handlers/deploy.go`
- `packages/astro-spec/deployment_spec.go`
- `apps/astro-server/internal/admingrpc/server.go` (if it reads deployments)
- `apps/astro-client/src/lib/api.ts` (types)
- Frontend deploy/list components

---

## Part 3: Display Name — DB as Source of Truth

### Problem

PR #266 stores the display name as a K8s namespace annotation (`astro.dev/display-name`) and reads it back from K8s in `ListDeployments`. This has problems:
- K8s is the source of truth for a user-facing field — annotation loss means display name loss
- Listing deployments requires hitting the K8s API per namespace
- With multi-deployment, display name is per-deployment, not per-namespace
- No uniqueness enforcement — two deployments in the same account could have the same display name

### Design

Store `display_name` in the `deployments` table. The DB is the source of truth; the K8s annotation is kept as a convenience for `kubectl` users but is not authoritative.

**Schema** (already included in Part 2's final `deployments` table):
- `display_name varchar(64) NOT NULL DEFAULT ''`
- Unique partial index: `(account_id, display_name) WHERE status = 'active' AND display_name != ''` — enforces uniqueness within an account, allows empty (unnamed) deployments

**Deploy Handler:**
- `prepareDeployment()` reads `displayName` from `Target.DisplayName`, validates (max 64 chars, no control chars, trimmed)
- Before saving, check uniqueness: query for active deployment in same account with same display name → 409 with `"display_name already in use by another active deployment"`
- Still writes `astro.dev/display-name` annotation on the K8s namespace (secondary, not authoritative)

**ListDeployments Handler — shift from K8s-only to DB+K8s:**
- Instead of listing K8s namespaces → scraping annotations for display name:
  1. Query `deployments WHERE account_id = ? AND status = 'active'` — gets `display_name`, `namespace`, `id` from DB
  2. For each, hit K8s for runtime status (pods, replicas, ready count)
- Display name comes from the DB row, not from `ns.Annotations`
- This is a prerequisite for multi-deployment — can't rely on namespace labels when multiple deployments exist

### Uniqueness rules
- Display name must be unique among active deployments within the same account
- Empty display name is allowed (not subject to uniqueness) — unnamed deployments fall back to agent slug in the UI
- Uniqueness enforced at DB level via partial unique index and at handler level with pre-check for better error message

### Files to modify
- `apps/astro-server/schema.sql` — already covered in Part 2
- `apps/astro-server/internal/deploymentstore/store.go` — add display_name to all queries + new lookup method
- `apps/astro-server/handlers/deploy.go` — uniqueness pre-check, shift ListDeployments to DB+K8s
- `apps/astro-client` — handle 409 on deploy

---

## Verification

1. **Scanner**: Start server, check logs for "Namespace scan complete" with tracked/orphaned/stale counts. Check `namespace_ownership` table has rows matching active deployments.
2. **Deploy same agent twice**: Deploy agent with display name "Production", then same agent with display name "Staging" — both should be active simultaneously in separate namespaces
3. **Redeploy existing**: Redeploy an agent (same agent + same display name) — should reuse existing namespace from DB
4. **Undeploy specific**: Undeploy by display name while the other stays active
5. **List deployments**: Both deployments show in list with their respective display names
6. **Display name uniqueness**: Deploy two agents in the same account with the same display name — second should get 409
7. **Display name from DB**: Deploy agent with display name, verify `ListDeployments` returns it from DB (not from K8s annotation)
8. **Empty display name**: Deploy without display name — should succeed, UI falls back to slug
9. **Namespace from UUID**: New deployment gets namespace `astro-<truncated-uuid>`, verify it's stored and reused on redeploy
10. **Run existing tests**: `go test ./apps/astro-server/...`
