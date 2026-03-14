# Async Database-Driven Deployment Architecture

## Summary

Deploy/undeploy handlers synchronously call K8s APIs inline with the HTTP request. This causes four problems:

1. **Scalability** — Each deploy holds an HTTP connection open for the full K8s provisioning duration (secrets, configmaps, services, statefulsets, deployments, ingresses, cleanup). Under load, this saturates the server.
2. **No status visibility** — The deployment is either "in flight" (no DB record) or "active" (done). Users can't track provisioning progress or understand failures.
3. **KEDA conflict** — The existing drift checker sees KEDA-scaled-to-zero namespaces as "drifted" and would fight the autoscaler. There's no concept of a paused/scaled-down deployment.
4. **No retry/recovery** — If the K8s apply partially fails, the deployment is saved as "active" with errors baked into the response. There's no way to retry or recover.

Decouple the user-facing API from K8s operations. Handlers write desired state to the database and enqueue River jobs. Workers reconcile DB state to K8s asynchronously. A unified reconciler replaces drift check and namespace scan, with KEDA awareness.

```
User → POST /deploy → Handler → DB (status=pending) → River Job → K8s
                                                         ↓
                                              DB (status=active|failed)
                                                         ↑
User → GET /deployments/:id/status ──────────────────────┘
```

## Design

### Deployment status state machine

```
pending ──→ provisioning ──→ active ──→ undeploying ──→ undeployed
                │                │              │
                ↓                ↓              ↓
              failed          scaled_down     failed
                               │
                               ↓
                          provisioning (wakeup)
```

- `pending` — DB record saved, River job enqueued, waiting for worker pickup
- `provisioning` — Worker is actively applying K8s resources
- `active` — All resources applied successfully, deployment is live
- `failed` — Worker encountered an error, resources cleaned up, error details stored
- `undeploying` — Undeploy requested, River job enqueued
- `undeployed` — All K8s resources deleted
- `scaled_down` — KEDA has scaled namespace to zero; deployment spec is intact in DB

---

### Phase 1: Schema changes (`sql/astro-server/schema.sql`)

astro-server uses a single SDL file (`schema.sql`) rather than versioned migrations. All changes are made directly to the schema definition. Backward-compatible — existing `active`/`undeployed` rows remain valid.

#### 1.1 Alter `deployments` table

```sql
ALTER TABLE deployments
  ADD COLUMN error_message TEXT,
  ADD COLUMN error_details JSONB,
  ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN current_revision INT;
```

- `error_message` — Human-readable failure reason. NULL when healthy. Cleared on successful re-deploy.
- `error_details` — Structured JSON array matching `[]k8s.DeploymentError` shape. Allows the UI to show per-resource failures.
- `status_changed_at` — Tracks when status last transitioned. Used by the reconciler to detect stale `provisioning` jobs (worker crashed mid-apply).
- `current_revision` — Points to the active revision in `deployment_revisions`. NULL for legacy rows (backfilled in §1.5). The deploy worker reads the spec from this revision, not from the deployment row directly. This enables rollback: set `current_revision` to a previous value and enqueue a deploy job.

No CHECK constraint on status — the application layer enforces valid transitions. This avoids migration headaches as statuses evolve.

#### 1.2 Create `deployment_events` table

```sql
CREATE TABLE deployment_events (
  id            BIGSERIAL PRIMARY KEY,
  deployment_id TEXT      NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  status        TEXT      NOT NULL,
  message       TEXT,
  details       JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_deployment_events_deployment ON deployment_events(deployment_id);
```

Every status transition inserts a row. Provides a full audit trail for debugging and UI timeline views. CASCADE ensures cleanup when deployments are purged.

#### 1.3 Create `scaled_namespaces` table

```sql
CREATE TABLE scaled_namespaces (
  namespace      TEXT PRIMARY KEY,
  deployment_id  TEXT      NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  scaled_down_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Tracks which namespaces KEDA has scaled to zero. The reconciler inserts rows when it detects KEDA-managed zero replicas. The wakeup worker deletes them when re-provisioning. Keyed by namespace since one namespace = one deployment in our model.

#### 1.4 Drop `deployment_env_vars` table

```sql
DROP TABLE IF EXISTS deployment_env_vars;
```

This table stores resolved environment variables per workload — including resolved secret values. It is write-only in production: `normalized.go` inserts into it during deploy, but no handler, worker, or query ever reads from it. The deploy worker re-resolves env vars from the spec + `deployment_variables` at apply time, making this table redundant.

Dropping it eliminates an unnecessary copy of resolved secrets sitting in a queryable table. Less attack surface if someone gets DB read access.

#### 1.5 Create `deployment_revisions` table

```sql
CREATE TABLE deployment_revisions (
  id              BIGSERIAL PRIMARY KEY,
  deployment_id   TEXT      NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  revision        INT       NOT NULL,
  build_id        TEXT      NOT NULL,
  spec_json       JSONB     NOT NULL,
  kms_ciphertext  BYTEA,
  kms_key_id      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(deployment_id, revision)
);
CREATE INDEX idx_deployment_revisions_deployment ON deployment_revisions(deployment_id);
```

Every deploy and redeploy inserts a new revision row instead of overwriting `deployment_spec_json` on the `deployments` row. The revision number auto-increments per deployment (`SELECT COALESCE(MAX(revision), 0) + 1`). The `deployments.current_revision` column points to the active revision.

This is the foundation for rollback: to roll back, set `current_revision` to a previous value and enqueue a deploy job. The deploy worker reads the spec from the revision pointed to by `current_revision`.

Includes `kms_ciphertext` and `kms_key_id` so each revision is independently decryptable — the KMS key may rotate between deploys.

#### 1.6 Backfill existing rows

```sql
-- Backfill status_changed_at from original deploy time
UPDATE deployments SET status_changed_at = deployed_at;

-- Create revision 1 for every existing deployment from their current spec
INSERT INTO deployment_revisions (deployment_id, revision, build_id, spec_json, kms_ciphertext, kms_key_id, created_at)
SELECT id, 1, build_id, deployment_spec_json, kms_ciphertext, kms_key_id, deployed_at
FROM deployments
WHERE deployment_spec_json IS NOT NULL;

-- Point all existing deployments to revision 1
UPDATE deployments SET current_revision = 1
WHERE deployment_spec_json IS NOT NULL;
```

Backfills both `status_changed_at` and creates an initial revision for every existing deployment. Runs immediately after the schema changes so all rows are consistent before new code deploys. Safe to blanket-update since no new code writes these columns until the application is deployed with the new workers.

---

### Phase 2: Deployment store

#### 2.1 Status constants (`status.go`)

```go
const (
    StatusPending      = "pending"
    StatusProvisioning = "provisioning"
    StatusActive       = "active"
    StatusFailed       = "failed"
    StatusUndeploying  = "undeploying"
    StatusUndeployed   = "undeployed"
    StatusScaledDown   = "scaled_down"
)
```

Replace all hardcoded `"active"` and `"undeployed"` strings throughout the codebase with these constants.

#### 2.2 Model updates (`store.go`)

Add fields to `Deployment` struct:

```go
ErrorMessage    *string          `json:"error_message,omitempty"`
ErrorDetails    json.RawMessage  `json:"error_details,omitempty"`
StatusChangedAt time.Time        `json:"status_changed_at"`
CurrentRevision *int             `json:"current_revision,omitempty"`
```

Update all SELECT queries (`scanDeployment` helper) to include the new columns.

Add revision model:

```go
type DeploymentRevision struct {
    ID             int64           `json:"id"`
    DeploymentID   string          `json:"deployment_id"`
    Revision       int             `json:"revision"`
    BuildID        string          `json:"build_id"`
    SpecJSON       json.RawMessage `json:"spec_json"`
    KMSCiphertext  []byte          `json:"-"`
    KMSKeyID       *string         `json:"-"`
    CreatedAt      time.Time       `json:"created_at"`
}
```

#### 2.3 New methods (`store.go`)

**`UpdateStatus(id, status, errorMsg string, errorDetails json.RawMessage) error`**

Single transaction:
1. `UPDATE deployments SET status=$2, error_message=$3, error_details=$4, status_changed_at=NOW() WHERE id=$1`
2. `INSERT INTO deployment_events (deployment_id, status, message, details) VALUES ($1, $2, $3, $4)`

This is the **single entry point** for all status changes. Every worker and handler calls this method instead of ad-hoc UPDATE statements.

**`SaveDeploymentPending(params SaveDeploymentParams, txFn TxFunc) (*Deployment, error)`**

For first deploys. Like `SaveDeploymentFull` but:
- Inserts deployment row with `status='pending'`, `current_revision=1`
- Inserts revision 1 into `deployment_revisions` with the spec, build, and KMS fields
- Inserts initial deployment_event `(id, "pending", "Deployment queued", nil)`
- Still handles marking previous active deployment for the same agent as `undeployed` (safe because first deploys create a new namespace — the old namespace isn't touched until the undeploy job runs)
- Enqueues the River deploy job in the **same transaction** as the DB save. The `txFn` callback receives the `pgx.Tx` and calls `river.Client.InsertTx` inside it. This guarantees atomicity — if the job insert fails, the deployment record is rolled back too. No window where a `pending` record exists without a corresponding job.

**`UpdateDeploymentPending(params, txFn TxFunc) (*Deployment, error)`**

For redeploys (same deployment ID, same namespace). Like `UpdateDeploymentFull` but:
- Inserts a new revision into `deployment_revisions` (revision = `MAX(revision) + 1` for this deployment)
- Updates the deployment row: `current_revision=N`, `status='pending'`, `build_id=...`
- Inserts deployment_event `(id, "pending", "Redeploy queued (revision N)", nil)`
- Enqueues the River deploy job in the same transaction (same `txFn` pattern)
- Does NOT overwrite `deployment_spec_json` on the deployment row — the spec lives in the revision. (The column stays for backward compat with existing queries but is no longer the source of truth.)
- Does NOT mark anything as undeployed — this is an in-place update. The K8s apply will do a rolling update in the existing namespace. Old pods keep serving until K8s rolls them out.

**`GetCurrentRevision(deploymentID string) (*DeploymentRevision, error)`**

```sql
SELECT r.* FROM deployment_revisions r
JOIN deployments d ON d.id = r.deployment_id AND d.current_revision = r.revision
WHERE r.deployment_id = $1
```

Used by the deploy worker to load the spec for the revision being deployed.

**`GetRevisions(deploymentID string) ([]DeploymentRevision, error)`**

```sql
SELECT * FROM deployment_revisions WHERE deployment_id = $1 ORDER BY revision DESC
```

Used by the status endpoint and rollback handler to show revision history.

**`SetCurrentRevision(deploymentID string, revision int, txFn TxFunc) error`**

Transaction:
1. Verify the revision exists: `SELECT 1 FROM deployment_revisions WHERE deployment_id=$1 AND revision=$2`
2. `UPDATE deployments SET current_revision=$2, status='pending', status_changed_at=NOW() WHERE id=$1`
3. Insert deployment_event `(id, "pending", "Rollback to revision N", nil)`
4. Call `txFn` to enqueue the deploy job in the same transaction

**`GetDeploymentsInStatus(statuses ...string) ([]*Deployment, error)`**

```sql
SELECT * FROM deployments WHERE status = ANY($1) ORDER BY deployed_at DESC
```

Used by the reconciler to find all `active` and `provisioning` deployments.

**`MarkScaledDown(deploymentID, namespace string) error`**

Transaction:
1. `INSERT INTO scaled_namespaces (namespace, deployment_id) VALUES ($2, $1) ON CONFLICT (namespace) DO NOTHING`
2. `UpdateStatus(id, "scaled_down", ...)`

**`ClearScaledDown(namespace string) error`**

```sql
DELETE FROM scaled_namespaces WHERE namespace = $1
```

**`IsScaledDown(namespace string) (bool, error)`**

```sql
SELECT EXISTS(SELECT 1 FROM scaled_namespaces WHERE namespace = $1)
```

#### 2.4 Events query (`events.go`)

```go
type DeploymentEvent struct {
    ID           int64           `json:"id"`
    DeploymentID string          `json:"deployment_id"`
    Status       string          `json:"status"`
    Message      string          `json:"message,omitempty"`
    Details      json.RawMessage `json:"details,omitempty"`
    CreatedAt    time.Time       `json:"created_at"`
}

func (s *Store) GetDeploymentEvents(deploymentID string, limit int) ([]DeploymentEvent, error)
```

---

### Phase 3: Deployer package (`internal/deployer/`)

Extract K8s apply/teardown logic from the deploy handler into a reusable package. Both the River workers and the handler need this logic.

#### 3.1 Struct

```go
type Deployer struct {
    K8sClient    k8s.ClusterClient
    AccountStore *account.AccountStore
    Cfg          *config.Config
    Store        *deploymentstore.Store
    Log          *logger.Logger
}
```

#### 3.2 Apply method

```go
func (d *Deployer) Apply(ctx context.Context, dep *deploymentstore.Deployment) (*k8s.ApplyResult, error)
```

Extracted from `handlers/deploy.go` lines 378-411:

1. Load the current revision: `store.GetCurrentRevision(dep.ID)` → get `spec_json` and KMS fields from the revision row (not from the deployment row). Deserialize `revision.SpecJSON` → `*spec.AstroDeploymentSpec`
2. Build `k8s.ApplierConfig`:
   - `Namespace`: from `dep.Namespace`
   - `RegistryURL`, `ProxyRegistryHost`, `Environment`, `IngressDomain`, etc.: from `d.Cfg.Deployment`
   - `ImagePullPolicy`: derived from `d.Cfg.Deployment.K8sClientMode`
   - `NamespaceLabels`: `astro.dev/account-id`, `astro.dev/account`, `astro.dev/agent`, `astro.dev/build` — the deployment record has `account_id` but not `account_name`. Query account name via `AccountStore` at apply time (avoids schema changes, handles account renames). This adds one DB query per deploy in the worker path (vs. the handler path where account info is already loaded). If the account lookup fails (deleted account, DB error), `Apply` returns an error and the worker marks the deployment as `failed` with the lookup error message — this is correct behavior since you can't deploy to a nonexistent account.
3. Construct `k8s.NewApplier(d.K8sClient, applierConfig)`
4. Call `applier.ApplyDeploymentSpec(ctx, resolvedSpec)`
5. Return the `*k8s.ApplyResult`

#### 3.3 Teardown method

```go
func (d *Deployer) Teardown(ctx context.Context, dep *deploymentstore.Deployment) error
```

Extracted from `handlers/deploy.go` lines 582-593:

1. Delete namespace: `d.K8sClient.Clientset().CoreV1().Namespaces().Delete(ctx, dep.Namespace, metav1.DeleteOptions{})`
2. Return error (caller handles DB status update)

---

### Phase 4: River workers

#### 4.1 Deploy worker (`deploy.go`)

```go
type DeployArgs struct {
    DeploymentID string `json:"deployment_id"`
}

func (DeployArgs) Kind() string { return "deploy" }

func (DeployArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{
        Queue:       queueDeploy,
        MaxAttempts: 3,
        UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStateRunning}},
    }
}

type DeployWorker struct {
    river.WorkerDefaults[DeployArgs]
    deployer *deployer.Deployer
    store    *deploymentstore.Store
    log      *logger.Logger
}
```

Work method logic:

```
1. dep := store.GetDeploymentByID(args.DeploymentID)
2. if dep == nil || dep.Status != StatusPending {
       log.Info("skipping, not in pending status")
       return nil  // idempotency — already processed or cancelled
   }
3. store.UpdateStatus(dep.ID, StatusProvisioning, "", nil)
4. result, err := deployer.Apply(ctx, dep)
5. if err != nil {
       // Total failure. Do NOT teardown — this may be a redeploy where old pods
       // are still running in the same namespace. Tearing down would nuke them.
       // K8s rolling update semantics mean old pods survive if the new ones fail
       // to come up. Mark failed and let the user fix + redeploy.
       store.UpdateStatus(dep.ID, StatusFailed, err.Error(), nil)
       return fmt.Errorf("deploy failed: %w", err)
       // Returning error lets River retry (up to MaxAttempts=3).
       // On final attempt, status stays failed.
   }
6. if len(result.Errors) > 0 {
       // Partial failure — some K8s resources failed to apply but others succeeded.
       // Do NOT teardown for the same reason as above. Mark failed with error details
       // so the user can see which resources failed and fix their spec.
       errJSON, _ := json.Marshal(result.Errors)
       store.UpdateStatus(dep.ID, StatusFailed, "partial failure", errJSON)
       return nil  // no retry — user needs to fix the spec
   }
7. // Success
   store.UpdateStatus(dep.ID, StatusActive, "", nil)
8. return nil
```

**Why MaxAttempts=3:** K8s API server restarts, network blips, and etcd leader elections are common transient failures. 3 attempts with River's exponential backoff handles these without user intervention. Persistent failures (bad image, quota) will exhaust all attempts and land in `failed` status, where the user must fix the issue and redeploy.

**Why UniqueOpts with ByArgs+ByState:** Prevents duplicate jobs for the same deployment. If a user triggers redeploy while a job is already queued/running, the insert is deduplicated.

**Why no teardown on failure:** Deploys and redeploys operate on the same namespace and deployment ID. A redeploy is a K8s rolling update — the old pods keep serving until new ones pass readiness checks. If the new spec fails to apply, tearing down the namespace would destroy the old working pods too. Instead, mark `failed` and leave K8s in its current state. For first deploys, this means a partially-created namespace may linger, but the user will either fix and redeploy (which overwrites it) or undeploy (which cleans it up).

**Rapid redeploy handling:** If a user triggers two redeploys in quick succession for the same deployment ID, the `UniqueOpts{ByArgs, ByState}` deduplication prevents duplicate jobs. The second `UpdateDeploymentPending` call creates a new revision and updates `current_revision`, but the already-queued/running job uses `store.GetCurrentRevision(dep.ID)` at execution time (step 1 in the worker), so it reads from whatever `current_revision` points to at that moment. If the job hasn't started yet, it picks up the latest revision. If it's already running, the second insert is a no-op and the user needs to wait for it to finish, then redeploy again. Both revisions are preserved in history regardless.

#### 4.2 Undeploy worker (`undeploy.go`)

```go
type UndeployArgs struct {
    DeploymentID string `json:"deployment_id"`
}

func (UndeployArgs) Kind() string { return "undeploy" }

func (UndeployArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{Queue: queueDeploy, MaxAttempts: 3}
}

type UndeployWorker struct {
    river.WorkerDefaults[UndeployArgs]
    deployer *deployer.Deployer
    store    *deploymentstore.Store
    log      *logger.Logger
}
```

Work method logic:

```
1. dep := store.GetDeploymentByID(args.DeploymentID)
2. if dep == nil || dep.Status != StatusUndeploying {
       return nil  // already processed
   }
3. err := deployer.Teardown(ctx, dep)
4. if err != nil {
       store.UpdateStatus(dep.ID, StatusFailed, "undeploy failed: " + err.Error(), nil)
       return nil
   }
5. store.ClearScaledDown(dep.Namespace)
6. store.UpdateStatus(dep.ID, StatusUndeployed, "", nil)
7. store.MarkUndeployedByID(dep.ID)  // set undeployed_at timestamp
8. return nil
```

#### 4.3 WakeUp worker (`wakeup.go`)

```go
type WakeUpArgs struct {
    DeploymentID string `json:"deployment_id"`
}

func (WakeUpArgs) Kind() string { return "wakeup" }

func (WakeUpArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{Queue: queueDeploy, MaxAttempts: 3}
}

type WakeUpWorker struct {
    river.WorkerDefaults[WakeUpArgs]
    deployer *deployer.Deployer
    store    *deploymentstore.Store
    log      *logger.Logger
}
```

Work method logic:

```
1. dep := store.GetDeploymentByID(args.DeploymentID)
2. if dep == nil || dep.Status != StatusScaledDown {
       return nil
   }
3. store.UpdateStatus(dep.ID, StatusProvisioning, "", nil)
4. result, err := deployer.Apply(ctx, dep)
5. if err != nil {
       store.UpdateStatus(dep.ID, StatusFailed, err.Error(), nil)
       return nil
   }
6. store.ClearScaledDown(dep.Namespace)
7. store.UpdateStatus(dep.ID, StatusActive, "", nil)
8. return nil
```

The wakeup flow re-applies the full spec from DB. KEDA will have set replicas to 0, but our apply sets them back to the desired count from the spec. KEDA can then scale them down again if the workload goes idle.

#### 4.4 Reconcile worker (`reconcile.go`)

Replaces both `driftcheck.go` and `nsscan.go`.

```go
type ReconcileArgs struct{}

func (ReconcileArgs) Kind() string { return "reconcile" }

type ReconcileWorker struct {
    river.WorkerDefaults[ReconcileArgs]
    deployer *deployer.Deployer
    store    *deploymentstore.Store
    k8s      k8s.ClusterClient
    pool     *pgxpool.Pool
    client   *river.Client[pgx.Tx]
    log      *logger.Logger
}
```

Work method logic:

```
1. if k8s == nil { return nil }  // no K8s client, skip

2. // --- Active deployment reconciliation ---
   deps := store.GetDeploymentsInStatus(StatusActive)
   for _, dep := range deps {
       scaledDown, _ := store.IsScaledDown(dep.Namespace)
       if scaledDown { continue }  // skip KEDA-managed

       // Check for KEDA scale-down
       if isKEDAScaledDown(ctx, dep.Namespace) {
           store.MarkScaledDown(dep.ID, dep.Namespace)
           continue
       }

       // Check for reconciliation opt-out.
       // If the namespace has the annotation astro.dev/reconcile=paused, skip drift
       // remediation. Users set this during debugging to prevent the reconciler from
       // fighting manual K8s changes. The annotation is checked on the namespace
       // (not per-resource) for simplicity.
       if hasAnnotation(ctx, dep.Namespace, "astro.dev/reconcile", "paused") {
           log.Info("Reconciliation paused via annotation, skipping drift check",
               "deployment_id", dep.ID,
               "namespace", dep.Namespace,
           )
           continue
       }

       // Drift detection (adapted from driftcheck.Checker)
       drifts := detectDrift(ctx, dep)
       if len(drifts) > 0 {
           log.Warn("Drift detected, enqueuing re-apply",
               "deployment_id", dep.ID,
               "drifts", len(drifts),
           )
           store.UpdateStatus(dep.ID, StatusPending, "", nil)
           insertDeployJob(ctx, dep.ID)
       }
   }

3. // --- Stale provisioning detection ---
   provisioning := store.GetDeploymentsInStatus(StatusProvisioning)
   for _, dep := range provisioning {
       if time.Since(dep.StatusChangedAt) > 15*time.Minute {
           log.Error("Deployment stuck in provisioning",
               "deployment_id", dep.ID,
               "since", dep.StatusChangedAt,
           )
           store.UpdateStatus(dep.ID, StatusFailed, "timed out in provisioning", nil)
       }
   }

4. // --- Namespace ownership maintenance (from nsscan) ---
   maintainNamespaceOwnership(ctx, deps)

5. // --- Orphan detection ---
   detectOrphanedNamespaces(ctx, deps)  // log only, no auto-delete

6. return nil
```

**KEDA detection — `isKEDAScaledDown(ctx, namespace)`:**

```
1. Use K8s dynamic client to list ScaledObjects (keda.sh/v1alpha1):
   client := dynamic.NewForConfig(k8s.Config())
   gvr := schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"}
   objects, _ := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
2. If no ScaledObjects exist → return false (not KEDA-managed)
3. For each ScaledObject, read .status.conditions where type == "Active"
4. If ALL ScaledObjects have Active=False → return true (KEDA intentionally at zero)
5. If ANY ScaledObject has Active=True → return false (KEDA is scaling up or maintaining)
```

No need to check ready replicas, desired replicas, or HPAs. The `Active` condition is KEDA's own declaration of intent. KEDA sets `Active=True` **before** it tells the HPA to scale up replicas, so during a 0→N scale-up window `Active` is already `True` — no race condition. `Active=False` means KEDA sees no trigger activity and has intentionally scaled to `idleReplicaCount`/`minReplicaCount` (zero for scale-to-zero configs).

**Drift auto-remediation — behavior change:**

Today, drift check is observe-only (logs drift but takes no action). The reconciler changes this to auto-remediation: detected drift triggers a re-apply. This is a significant operational change. If a user manually tweaks a K8s resource during debugging (e.g., changing an env var, scaling replicas), the reconciler will revert the change within 10 minutes. The `astro.dev/reconcile=paused` namespace annotation (see step 2 in the work method above) provides an opt-out for this scenario. Users can `kubectl annotate ns <ns> astro.dev/reconcile=paused` to freeze reconciliation, then remove the annotation when done.

**Drift detection — `detectDrift(ctx, dep)`:**

Reuses core logic from `internal/driftcheck/checker.go`:
- Load normalized workloads from DB for the deployment
- For each workload, compare against K8s actual state:
  - Missing resource → drift
  - Image mismatch → drift
  - Replicas mismatch (and not KEDA) → drift
  - CronJob schedule mismatch → drift
- For services/ingresses: check existence

Move the comparison logic into the reconcile worker directly (~100 lines) rather than keeping the driftcheck package.

#### 4.5 Queue configuration (`client.go`)

Add to `Config` struct:
```go
ServerConfig *config.Config
AccountStore *account.AccountStore
```

Add queue constant and config:
```go
const queueDeploy = "deploy"

// In New():
Queues: map[string]river.QueueConfig{
    river.QueueDefault: {MaxWorkers: 10},
    queueDeploy:        {MaxWorkers: 5},
    queueWorkOS:        {MaxWorkers: 1},
},
```

Add `InsertTx` method that accepts an existing transaction, so callers can atomically save a DB record and enqueue a job in one transaction:

```go
func (q *Queue) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*river.JobInsertResult, error) {
    return q.client.InsertTx(ctx, tx, args, opts)
}
```

This is a thin wrapper exposing River's `InsertTx`. The handler's `SaveDeploymentPending` already runs inside a `txFn` callback that receives the `pgx.Tx` — it calls `queue.InsertTx(ctx, tx, DeployArgs{...}, nil)` inside that same callback. If either the deployment save or the job insert fails, the entire transaction rolls back. No window where a `pending` record exists without a job.

Also add a standalone `Insert` for cases where the caller doesn't have a transaction (e.g., reconciler enqueuing drift re-apply jobs):

```go
func (q *Queue) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*river.JobInsertResult, error) {
    tx, err := q.pool.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("riverqueue: begin tx: %w", err)
    }
    defer tx.Rollback(ctx)

    result, err := q.client.InsertTx(ctx, tx, args, opts)
    if err != nil {
        return nil, fmt.Errorf("riverqueue: insert: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("riverqueue: commit: %w", err)
    }
    return result, nil
}
```

#### 4.6 Worker registration (`workers.go`)

```go
func addWorkers(workers *river.Workers, cfg Config) {
    // Shared deployer instance for all deploy-related workers
    dep := &deployer.Deployer{
        K8sClient:    cfg.K8sClient,
        AccountStore: cfg.AccountStore,
        Cfg:          cfg.ServerConfig,
        Store:        deploymentstore.NewStore(cfg.DB),
        Log:          cfg.Logger,
    }
    store := deploymentstore.NewStore(cfg.DB)

    river.AddWorker(workers, &HeartbeatWorker{...})           // unchanged
    river.AddWorker(workers, &WorkOSEventsWorker{...})        // unchanged
    river.AddWorker(workers, &DeployWorker{deployer: dep, store: store, log: cfg.Logger})
    river.AddWorker(workers, &UndeployWorker{deployer: dep, store: store, log: cfg.Logger})
    river.AddWorker(workers, &WakeUpWorker{deployer: dep, store: store, log: cfg.Logger})
    river.AddWorker(workers, &ReconcileWorker{deployer: dep, store: store, k8s: cfg.K8sClient, log: cfg.Logger})

    // REMOVED: DriftCheckWorker, NsScanWorker
}
```

#### 4.7 Periodic jobs (`periodic.go`)

Remove DriftCheck and NsScan entries. Add:

```go
river.NewPeriodicJob(
    river.PeriodicInterval(10*time.Minute),
    func() (river.JobArgs, *river.InsertOpts) {
        return ReconcileArgs{}, &river.InsertOpts{
            UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute},
        }
    },
    &river.PeriodicJobOpts{RunOnStart: true},
),
```

---

### Phase 5: Handler changes (`handlers/deploy.go`)

#### 5.1 DeployAgent

Signature change — remove `k8sClient`, add `queue *riverqueue.Queue`.

Remove:
- `k8sClient == nil` check
- `k8s.NewApplier()` + `applier.ApplyDeploymentSpec()` — entire K8s apply block
- Status/response based on `applyResult`

Keep:
- Spec parsing, prepareDeployment, entitlement check
- Spec stripping, encryption, resolved env, normalized spec save

Change:
- First deploy: `SaveDeploymentFull` → `SaveDeploymentPending` (inserts with `status='pending'`)
- Redeploy: `UpdateDeploymentFull` → `UpdateDeploymentPending` (updates same row, sets `status='pending'`)
- The River job is enqueued **inside** the `txFn` callback, in the same DB transaction as the deployment save. This guarantees atomicity — no window where a `pending` record exists without a corresponding job:

```go
// First deploy
dep, err := deployStore.SaveDeploymentPending(params, func(ctx context.Context, tx pgx.Tx) error {
    _, err := queue.InsertTx(ctx, tx, riverqueue.DeployArgs{DeploymentID: dctx.deploymentID}, nil)
    return err
})

// Redeploy (same deployment ID)
dep, err := deployStore.UpdateDeploymentPending(params, func(ctx context.Context, tx pgx.Tx) error {
    _, err := queue.InsertTx(ctx, tx, riverqueue.DeployArgs{DeploymentID: dctx.deploymentID}, nil)
    return err
})
```

New response — `http.StatusAccepted` (202) instead of 200:
```go
c.JSON(http.StatusAccepted, deployment.DeployResponse{
    Status:       "pending",
    DeploymentID: dctx.deploymentID,
    Name:         dctx.agentName,
    BuildID:      dctx.buildID,
    K8sNamespace: dctx.k8sNS,
})
```

#### 5.2 UndeployAgent

Signature change — remove `k8sClient`, add `queue *riverqueue.Queue`.

Remove:
- `k8sClient` nil check + `Namespaces().Delete()` + error handling

Change:
- Accept `active` OR `scaled_down` status (not just `active`)

Add:
```go
if err := deployStore.UpdateStatus(dep.ID, "undeploying", "", nil); err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
    return
}

if _, err := queue.Insert(c.Request.Context(), UndeployArgs{DeploymentID: dep.ID}, nil); err != nil {
    log.Error("Failed to enqueue undeploy job", "error", err)
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule undeploy"})
    return
}
```

Response:
```go
c.JSON(http.StatusAccepted, deployment.UndeployResponse{
    Status:       "undeploying",
    DeploymentID: dep.ID,
    Name:         dep.AgentName,
    K8sNamespace: dep.Namespace,
})
```

#### 5.3 New: GetDeploymentStatus

```go
func GetDeploymentStatus(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store) gin.HandlerFunc
```

- Route: `GET /api/v1/deployments/:id/status`
- Auth: JWT middleware → verify user is member of deployment's account
- Response:
```json
{
  "deployment_id": "abc123",
  "status": "provisioning",
  "current_revision": 3,
  "error_message": null,
  "error_details": null,
  "deployed_at": "2026-03-14T...",
  "status_changed_at": "2026-03-14T...",
  "events": [
    {"status": "pending", "message": "Deployment queued", "created_at": "..."},
    {"status": "provisioning", "message": "", "created_at": "..."}
  ],
  "revisions": [
    {"revision": 3, "build_id": "build-xyz", "created_at": "..."},
    {"revision": 2, "build_id": "build-abc", "created_at": "..."},
    {"revision": 1, "build_id": "build-initial", "created_at": "..."}
  ]
}
```

#### 5.4 New: WakeUpDeployment

```go
func WakeUpDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue *riverqueue.Queue) gin.HandlerFunc
```

- Route: `POST /api/v1/deployments/:id/wakeup`
- Auth: JWT middleware → verify membership
- Validates deployment status is `scaled_down`
- Sets status to `pending`, enqueues `WakeUpArgs`
- Returns `202 {"status": "pending", "deployment_id": "..."}`

#### 5.5 New: RollbackDeployment

```go
func RollbackDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue *riverqueue.Queue) gin.HandlerFunc
```

- Route: `POST /api/v1/deployments/:id/rollback`
- Auth: JWT middleware → verify membership
- Request body: `{"revision": 2}`
- Validates:
  - Deployment exists and user has access
  - Deployment status is `active` or `failed` (can't rollback a `pending`/`provisioning` deployment — one is already in flight)
  - Target revision exists and is different from `current_revision`
- Calls `store.SetCurrentRevision(dep.ID, revision, txFn)` which atomically sets `current_revision`, sets `status='pending'`, and enqueues the deploy job
- Returns `202`:
```json
{
  "status": "pending",
  "deployment_id": "abc123",
  "current_revision": 2,
  "message": "Rolling back to revision 2"
}
```

The deploy worker picks up the job, calls `store.GetCurrentRevision(dep.ID)` which now returns revision 2's spec, and applies it. No special rollback worker — the same deploy worker handles it because rollback is just "apply a different revision's spec."

---

### Phase 6: Server wiring (`main.go`)

#### 6.1 runAPI

Create queue for insert-only — don't start workers (they only run in worker mode):

```go
rq, rqErr := riverqueue.New(context.Background(), cfg.Database.URL, riverqueue.Config{
    DB:           db,
    K8sClient:    k8sClient,
    AccountStore: accountStore,
    ServerConfig: cfg,
    Logger:       log,
})
if rqErr != nil {
    log.Error("Failed to create River queue for API", "error", rqErr)
    os.Exit(1)
}
// Note: do NOT call rq.Start() — workers run in worker mode only
```

Pass `rq` to `setupRoutes`.

#### 6.2 setupRoutes

Add `queue *riverqueue.Queue` parameter. Update handler registrations:

```go
// Before
handlers.DeployAgent(log, agentIndex, accountStore, cfg, k8sClient, deploymentStore, ent)
handlers.UndeployAgent(log, agentIndex, accountStore, cfg, k8sClient, deploymentStore)

// After
handlers.DeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, ent, queue)
handlers.UndeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, queue)
```

New routes:
```go
api.GET(protected, "/deployments/:id/status", "Get deployment status",
    handlers.GetDeploymentStatus(log, accountStore, deploymentStore))
api.POST(protected, "/deployments/:id/wakeup", "Wake up a scaled-down deployment",
    handlers.WakeUpDeployment(log, accountStore, deploymentStore, queue))
api.POST(protected, "/deployments/:id/rollback", "Rollback to a previous revision",
    handlers.RollbackDeployment(log, accountStore, deploymentStore, queue))
```

#### 6.3 runWorker

Add `ServerConfig` and `AccountStore` to the River config:

```go
rq, rqErr := riverqueue.New(workerCtx, cfg.Database.URL, riverqueue.Config{
    DB:           db,
    OMClient:     omClient,
    AccountStore: accountStore,
    K8sClient:    k8sClient,
    ServerConfig: cfg,           // NEW
    WorkOSAPIKey: cfg.Auth.WorkOSAPIKey,
    OrgClient:    orgClient,
    Logger:       log,
})
```

---

### Phase 7: Cleanup

Delete replaced code:

| File | Action |
|------|--------|
| `apps/astro-server/internal/riverqueue/driftcheck.go` | Delete |
| `apps/astro-server/internal/riverqueue/nsscan.go` | Delete |
| `apps/astro-server/internal/driftcheck/` | Delete directory |
| `apps/astro-server/internal/nsscan/` | Delete directory |

**Namespace ownership maintenance** from nsscan is preserved — the upsert logic moves into `ReconcileWorker.maintainNamespaceOwnership()`.

**Drift detection logic** from driftcheck is adapted into the reconcile worker's `detectDrift()` function (~100 lines of comparison logic).

**`deployment_env_vars` cleanup:** Remove the `INSERT INTO deployment_env_vars` block from `normalized.go` `SaveNormalizedSpec` (~lines 165-175). Update `normalized_test.go` to remove the assertion that reads from the dropped table.

---

## Key files

| File | Change |
|------|--------|
| `sql/astro-server/schema.sql` | Edit — new columns, tables |
| `internal/deploymentstore/status.go` | Create — status constants |
| `internal/deploymentstore/events.go` | Create — event model + query |
| `internal/deploymentstore/revisions.go` | Create — revision model + queries |
| `internal/deploymentstore/store.go` | Edit — new fields, new methods |
| `internal/deployer/deployer.go` | Create — extracted K8s apply/teardown |
| `internal/riverqueue/deploy.go` | Create — deploy worker |
| `internal/riverqueue/undeploy.go` | Create — undeploy worker |
| `internal/riverqueue/wakeup.go` | Create — wakeup worker |
| `internal/riverqueue/reconcile.go` | Create — unified reconciler |
| `internal/riverqueue/client.go` | Edit — deploy queue, Insert method |
| `internal/riverqueue/workers.go` | Edit — register new workers, remove old |
| `internal/riverqueue/periodic.go` | Edit — reconcile replaces drift+nsscan |
| `handlers/deploy.go` | Edit — async flow, new endpoints |
| `main.go` | Edit — queue wiring, new routes |
| `internal/riverqueue/driftcheck.go` | Delete |
| `internal/riverqueue/nsscan.go` | Delete |
| `internal/driftcheck/` | Delete directory |
| `internal/nsscan/` | Delete directory |

## Verification

1. **Build:** `cd apps/astro-server && go build ./...` — must compile cleanly
2. **Status transitions:** Deploy → 202 `{status: "pending"}`, poll `/deployments/:id/status` → pending → provisioning → active
3. **Undeploy:** → 202 `{status: "undeploying"}`, poll → undeploying → undeployed
4. **Failure:** Deploy with invalid image → status moves to `failed` with `error_message` populated
5. **Reconciler:** Delete a K8s resource manually → reconciler detects drift and re-applies within 10min
6. **KEDA:** Scale deployment to 0 replicas + create ScaledObject → reconciler marks `scaled_down`, does NOT re-apply
7. **Wakeup:** POST `/deployments/:id/wakeup` on scaled_down deployment → provisioning → active
8. **Rollback:** Deploy revision 1, redeploy revision 2, then POST `/deployments/:id/rollback` with `{"revision": 1}` → status moves to pending → provisioning → active, running revision 1's spec
9. **Revision history:** GET `/deployments/:id/status` → response includes `current_revision` and `revisions` array with all past revisions
