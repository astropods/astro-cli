# Cluster configuration and placement

**Status:** Authoritative, but volatile — this area changed daily over the
past two weeks and is still settling; treat the "What's still moving"
section as load-bearing, not a footnote, before trusting a specific detail.
**Last verified:** 2026-08-26

This doc covers how astro-server knows about the Kubernetes clusters it can
deploy to, how it decides which cluster a given deployment or account uses,
and how it moves a deployment from one cluster to another. It's the as-built
description for five packages: `internal/clusterid`, `internal/clusterconfig`,
`internal/clusterfields`, `internal/clustercfg`, and `internal/clusterplacement`.

**This area changed daily over the last two weeks and is still settling.**
Five of the last dozen commits touching these packages landed in the last
week alone, including a same-week rewrite of the migration-outcome model and
a same-week unification of what "primary cluster" means. Treat the
[What's still moving](#whats-still-moving) section below as load-bearing, not
a footnote — a detail here is more likely than most docs in this tree to be
stale by the time you read it.

## Boundary with clusterstore

`internal/clusterstore` is part of the Registry auth area in
[`../README.md`](../README.md)'s area map (alongside `apps/astro-registry`,
`deploytoken`, `ingesttoken`) and owns the `clusters` table itself: schema,
row validation, insert/update/delete, and generating the per-cluster
registry pull credential used to authorize image pulls. It's the write
path.

The packages this doc covers sit on top of `clusterstore` and answer three
different questions:

- **What config does a deployment on cluster X get?** (`clustercfg`, backed
  by `clusterfields`'s shared validation rules)
- **Which clusters can account Y deploy to, and what's the default?**
  (`account.ClusterBindings`, in `internal/account/clusters.go` — not one of
  the five packages above, but the table this doc's placement logic reads
  and writes; see [Account cluster placement](#account-cluster-placement))
- **How does a deployment's cluster_id ever change, safely?**
  (`clusterplacement`)

`internal/clusterid` is the small utility both of the latter two depend on:
one definition of "what counts as the default cluster."

`internal/k8scache` is unrelated to any of this. It's a generic Redis-backed
cache (`Cache` interface: `Get`/`Set`/`Invalidate`/`GetMany`) that several
unrelated subsystems reuse for their own keys — `accountcache`,
`blueprintcache`, `deploycache`, `knowledgecache`, `listcache`, `obssummary`,
and multiple River workers all construct their own key prefixes against the
same `Cache` interface. `clusterplacement.Migrator` takes one only to
invalidate a deploy-cache entry after a migration; it isn't a cluster
registry of any kind. See [k8scache](#k8scache-a-shared-primitive-not-a-cluster-concept)
below for the one paragraph it needs.

## The default cluster, in one definition

`internal/clusterid.Resolver` is the single place that decides what an empty
or unset cluster id means. It's built once from `DEFAULT_CLUSTER_ID`
(`clusterid.New(cfg.Deployment.DefaultClusterID)`) and passed down instead of
every call site re-deriving the rule:

```go
type Resolver struct{ primary string }

func (r Resolver) Canonical(clusterID string) string   // "" -> primary
func (r Resolver) Same(a, b string) bool               // compares after Canonical
func (r Resolver) IsPrimary(clusterID string) bool
func (r Resolver) Label(clusterID string) string       // for messages; "unrecorded" if no default configured
```

This landed recently (`e18e19f81`, "Give cluster identity one definition") to
fix a real bug: the placement-mismatch check used to treat both an empty id
*and* the literal string `"default"` as meaning the primary cluster, and
didn't know the primary's own row id once anything (like `account_clusters`)
started recording it explicitly. A deployment already on the primary,
addressed by its real id, read as a move *away* from an unrecorded cluster,
and the migrator would tear it down and rebuild it in place. `Resolver` is
the fix: nothing writes the string `"default"` anymore, and `Canonical`/`Same`
are the only comparison either `clusterplacement` or `account.ClusterBindings`
perform.

Every call site builds its own `clusterid.Resolver` from
`cfg.Deployment.DefaultClusterID` (or `cfg.ServerConfig.Deployment...` in
River workers) rather than sharing one instance — cheap to construct, no
shared-state risk.

## Cluster config: the source of truth for connectivity data

`internal/clusterconfig` owns loading and syncing the JSON file astro-infra
renders — every cluster astro-server can reach, **including the one
astro-server itself runs on.**

```go
type Entry struct {
    ID, Region, EKSClusterName, EKSClusterEndpoint, EKSClusterCA string
    AgentIngressDomain, AgentPublicIngressDomain, IngestionIngressDomain string
    LangfuseBaseURLExt, LangfuseVPCEIPs                                 string
    PodSubnetCIDRs, PodSubnetIPv6CIDRs                                  string
    LokiURL, PrometheusURL, TenantRouterInternalURL                    string
}
```

`Load(path)` reads and parses the file (empty path returns `nil, nil` — see
[local dev](#local-dev-has-its-own-entry) for why that's not the production
path). `Find(entries, id)` picks the entry matching `DEFAULT_CLUSTER_ID`.
`Sync` upserts every entry into `clusterstore` via `UpsertFromConfig`, then
calls `clusterstore.DeleteRemoved` to delete (or, if still referenced by an
account or deployment, log-and-skip) any config-synced row absent from the
current file.

**There is no primary-versus-additional split.** This is the single biggest
way the shipped system differs from both specs in `01-spec/` (see
[Where the specs disagree with this doc](#where-the-specs-disagree-with-this-doc)).
Every cluster, including the one astro-server is deployed into, is a row in
`clusters`, sourced from the same config file, told apart only by whether its
id matches `DEFAULT_CLUSTER_ID`. `INGRESS_DOMAIN`, `INGESTION_INGRESS_DOMAIN`,
`POD_SUBNET_CIDRS`, `POD_SUBNET_IPV6_CIDRS`, `LANGFUSE_BASE_URL_EXT`,
`LANGFUSE_VPCE_IPS`, and `TENANT_ROUTER_INTERNAL_URL` env vars that used to
carry the primary's own values don't exist anymore — that data comes from
the default cluster's own config entry, exactly like every other cluster.

### Boot wiring (`main.go`, `buildRegistryConfig`)

1. `CLUSTER_CONFIG_PATH` is required. An empty path fails `buildRegistryConfig`
   outright (`"CLUSTER_CONFIG_PATH is required"`) — there is no fallback to
   individual env vars anymore.
2. `clusterconfig.Load` parses the file.
3. `clusterconfig.Find` resolves `DEFAULT_CLUSTER_ID` against the loaded
   entries *before* syncing anything — a default id that doesn't match any
   entry (for example, renamed in config without updating the env var) fails
   boot before `Sync` deletes or upserts a single row.
4. `clusterconfig.Sync` runs.
5. `RegistryConfig.DefaultClusterID` and `PrometheusURL` are set from the
   resolved default entry. In EKS mode, `Region`/`EKSBootstrapName`/
   `EKSBootstrapURL`/`EKSBootstrapCA` also come from that entry — the default
   cluster's `ClusterClient` is built from its cluster-config entry, not from
   dedicated bootstrap env vars. In local mode, those EKS fields are inert
   (the client is built from kubeconfig instead) and the entry's CA is empty.

A failure here is handled differently by environment
(`exitOnLocalClusterMisconfig`): **local mode exits the process** if the
cluster config can't be read, because that entry is the only source of the
local cluster's ingress domains and observability URLs — a degraded boot
would silently produce deploys that resolve to nothing. **Managed
environments warn and degrade instead** (`k8s: registry init failed` /
`k8s: features unavailable`), on the theory that an operator is watching and
the rest of the API is still worth serving without Kubernetes.

### Local dev has its own entry

Before `0fd4b6e74` ("Give local dev a cluster config"), `K8S_CLIENT_MODE=local`
short-circuited `buildRegistryConfig` before it ever read
`CLUSTER_CONFIG_PATH`, so a locally deployed agent got no ingress domain and
metrics panels had no backend — the env vars that used to carry that data had
just been removed by the primary/additional unification, and nothing replaced
them for local mode. Now `scripts/dev.sh` generates a one-entry cluster-config
file for that developer's machine (the cluster id, kube context, and backend
URLs differ per developer), and boot loads/syncs it the same way production
does. Local mode still builds its `ClusterClient` from kubeconfig, not from
the entry's (placeholder) EKS coordinates — `Registry.Get` returns the
primary client for the default cluster's own id rather than dialing those
placeholder coordinates.

## Field contracts (`clusterfields`)

`clusterfields` exists so `clusterstore` (write time) and `clustercfg` (read
time) validate the exact same required fields with the exact same rules,
instead of two hand-maintained copies drifting apart.

```go
type DeployConfig struct {
    AgentIngressDomain, IngestionIngressDomain string
    LangfuseBaseURLExt                          string
    LangfuseVPCEIPs                              string // optional, see below
    PodSubnetCIDRs                               string
}

type Registration struct {
    Region, EKSClusterName, EKSClusterEndpoint string
    Deploy DeployConfig
}
```

`ValidateDeployNonEmpty(clusterID, d)` checks every `DeployConfig` field is
non-empty (`clusterID == ""` produces a store-style message, a non-empty id
produces a deploy-time message naming the cluster). `ValidateRegistrationNonEmpty`
adds the EKS identity fields on top, for `clusterstore.UpsertFromConfig`.

`langfuse_vpce_ips` is deliberately excluded from the required-fields list in
both callers: it's optional on every cluster, including the default one —
only a cluster that needs a PrivateLink netpol exception to reach Langfuse
sets it at all. Everything else is required on every cluster with no
exception, including the default one now that it's a row too.

## Resolving per-deployment config (`clustercfg`)

`clustercfg.Resolve(ctx, registry, dep, clusterID)` is the one function that
turns "this deployment targets cluster X" into the ingress domains,
observability URLs, netpol CIDRs, and registry pull credential a deploy
actually needs. `deployer.Deployer` calls it on every apply, redeploy, and
teardown-config path; `admingrpc.Server.resolveIngressForCluster` calls it
for the admin health-check surface.

```go
type Resolved struct {
    AgentIngressDomain, AgentPublicIngressDomain, IngestionIngressDomain string
    LangfuseBaseURL        string
    LangfuseVPCEIPs        []string
    PodSubnetCIDRs         []string
    PodSubnetIPv6CIDRs     []string
    CPSubnetCIDRs          []string // apiserver ENI subnets for service-proxy ingress NP
    RegistryPullCredential string
}
```

Two paths through `Resolve`:

- **`reg == nil`, or `clusterID == "" && reg.DefaultClusterID() == ""`** — no
  registry, or a registry with nothing configured as the default (local dev
  before a cluster config exists, or a boot where `buildRegistryConfig`
  never produced a default id). Falls back to `dep`
  (`config.DeploymentConfig`) verbatim: this is the pre-cluster-config
  behavior, kept so an unconfigured environment fails soft instead of
  erroring on every deploy.
- **Otherwise** — `reg.GetEntry(ctx, clusterID)` (empty `clusterID` resolves
  to the default cluster inside the registry). The entry is validated with
  `clusterfields.ValidateDeployNonEmpty` and, if `dep.ProxyRegistryHost != ""`
  (pull-through enabled) and the entry has no pull credential yet, `Resolve`
  errors rather than silently deploying with no way to pull the image.

One asymmetry survives even with the default cluster as a row:
`CPSubnetCIDRs` (apiserver ENI subnets for the service-proxy ingress
NetworkPolicy) has no per-cluster column at all — it always comes from `dep`
and is only populated when the resolved entry `IsDefault`. If the default
cluster's `LangfuseBaseURL` comes back empty from its own row, `Resolve`
falls back to `dep.LangfuseBaseURL` too. Both are called out in code
comments as deliberate, not oversights — there's no product need yet for
either to vary by cluster beyond the default one.

## Cluster identity in the registry (`internal/k8s.Registry`)

Not one of this doc's five packages, but the thing `clustercfg.Resolve` reads
from, so its shape matters here. `k8s.Registry` is the in-process owner of
every `ClusterClient`:

- `Default()` / `DefaultClusterID()` — the default cluster's client and id.
- `Get(ctx, id)` — a `ClusterClient` for any cluster id, including the
  default cluster's own id (returns the same client as `Default()` rather
  than rebuilding it from the row — a local kubeconfig cluster has no EKS
  coordinates that would resolve).
- `GetEntry(ctx, id)` — the cached `ClusterEntry` (config-owned fields) for
  an id; empty id resolves to the default cluster. Returns `ErrClusterNotFound`
  even for the default cluster if boot sync hasn't written its row yet — a
  missing row is a real failure, never silently synthesized.
- `List(ctx)` — every row from `clusterstore` verbatim, including or
  excluding the default cluster depending on whether it currently has a row.
- `Refresh(ctx, id)` — evicts cached client/entry/Loki/Prometheus clients for
  one id, so the next `Get`/`GetEntry` re-reads the row. Called after every
  admin cluster mutation, and by the deploy handler right after
  `validateDeployTargetCluster` warms the entry cache for an explicit
  `cluster_id` — evicted immediately so a SQL backfill or another replica's
  boot sync is visible to the same request's `clustercfg.Resolve` call.

## Account cluster placement

`account_clusters` (`account_id`, `cluster_id`, `is_default`, one row per
allowed pairing, unique-partial-indexed so at most one row per account has
`is_default`) is the allow-list a deployment's `cluster_id` is checked
against. It's populated and read through `account.ClusterBindings`
(`internal/account/clusters.go`), constructed as
`accountStore.Clusters(clusterid.New(cfg.Deployment.DefaultClusterID))`.

Two free functions carry the actual policy, both pure and easy to reason
about in isolation:

```go
func DefaultClusterID(allowed []ClusterBinding) string // the row with is_default, else lexically-first id, else ""
func IsAllowed(clusterID string, allowed []ClusterBinding) bool // true for every id when allowed is empty
```

**An account with no bindings is unrestricted** — `IsAllowed` returns `true`
for anything when `allowed` is empty. This is the zero-migration-required
case: an account created before `account_clusters` existed, or one that's
simply never had a binding added, behaves exactly like today's deploy flow
with no picker and no restriction.

`ClusterBindings.Add/Remove/SetDefault` back the `AddAccountCluster`/
`RemoveAccountCluster`/`SetAccountDefaultCluster` admin RPCs
(`internal/admingrpc/accounts.go`) astro-queen's Account Detail page calls —
see [`astro-queen.md`](astro-queen.md) for the operator-facing workflow; this
doc only covers what the RPCs do to the data:

- **`Add`** checks the cluster exists, materializes a primary binding first
  if the account has none yet (`materializePrimary` — an account with an
  empty allow-list stays unrestricted right up until its first explicit
  binding, at which point it needs the primary made explicit too, or adding
  a second cluster would silently make the account's *existing* implicit
  primary placement unreachable), then inserts/updates the row. The new
  binding becomes default if the caller asked, if it already was, or if the
  account had no default yet.
- **`Remove`** refuses (`account.ErrClusterInUse`) while any deployment on
  that account is `active`/`failed`/`pending` on that cluster. If it removed
  the account's default, it promotes the lexically-first remaining binding
  to default rather than leaving the account with none.
- **`SetDefault`** just moves the flag; it errors if the target isn't
  already an allowed cluster (`account.ErrClusterNotAllowed`) — a caller
  must `Add` first.

`BindPrimary` / `BackfillPrimaryBindings` (called from a leader-elected
background pass in `main.go`) give an account with no bindings an explicit
default binding to the primary, once one exists to bind to. This exists for
readers, like the registry's own image-pull authorization
(`clusterpull.Authorizer.ResolveHomedAccount`, `apps/astro-registry`), that
need the allow-list to be an exhaustive, queryable set rather than
interpreting "empty" themselves.

### Deploy-time resolution and enforcement

`handlers/deploy.go` is where `clusterid`, `account.ClusterBindings`, and
`clustercfg` meet, in two passes matching the sign-then-submit template flow:

- **`resolveTemplateClusterID`** (template time, before signing): a caller's
  explicit `cluster_id` must be `account.IsAllowed`, or the request is
  rejected (`ClusterNotAvailableError`, wraps `ErrClusterNotAllowed`, → 403)
  before anything is signed. No explicit pick on an *update* to an existing
  deployment keeps that deployment's current cluster
  (`current.EffectiveClusterID()`); no pick on a fresh deploy uses
  `account.DefaultClusterID(allowed)`. The resolved id is always passed
  through `clusters.Canonical(...)` before being written into the template.
- **`enforceAccountClusterPlacement`** (deploy time, after signature verify):
  re-checks the already-locked-in `target.cluster_id` against the account's
  *current* allow-list, since an operator can change it in the window
  between signing and submission.
- **`validateDeployTargetCluster`** runs after that: rejects an unknown
  cluster id (400) or an unhealthy one, via `k8sReg.GetEntry` and a health
  check — independent of account policy, this is "does the cluster exist and
  work."
- A cross-account deploy (deploying someone else's public agent into your
  own account) re-runs `enforceAccountClusterPlacement` against the target
  account's bindings right after confirming target-account membership and
  before the source-agent visibility check, since the target account's
  allow-list is what actually matters once the deployment lands there.
- An update to an existing deployment with a migration already in flight
  (`clusterplacement.InFlightMove` returns non-empty) is rejected with 409
  (`ErrMigrationInFlight`) unless the new submission targets that same
  in-flight destination — a redeploy can't race a migration for the same
  deployment.

## Cross-cluster migration (`clusterplacement`)

`clusterplacement.Migrator.MigrateDeployment` is the only code path that
changes a deployment's `cluster_id` outside of a normal deploy. It's called
from the `deployment.migrate_cluster` River job
(`MigrateDeploymentClusterWorker`, `internal/riverqueue/migrate_cluster.go`),
enqueued by `ReapplyDeployment` (see
[`astro-queen.md`](astro-queen.md#cross-cluster-migration-concretely) for the
operator-facing trigger and [`deployment-state-machine.md`](deployment-state-machine.md#orphan-and-drift-handling--narrower-than-it-might-look)
for `placementOrphaned`'s narrow scope in the state machine).

`MigrateDeployment` reasons over four outcomes (`MigrateOutcome`), reworked
recently (`5f09ae62e`, "Report what a cluster migration did") specifically
because the old boolean return hid a real race — a deployment could be left
`pending` with nothing running and nothing queued if a status changed
underneath the migration:

| Outcome | Meaning |
|---|---|
| `MigrateApplied` | Tore down the source cluster (best-effort — swallows `ErrClusterClientUnavailable`), patched the stored spec's `target.cluster_id`, recorded the migration event, enqueued a deploy job on the target. |
| `MigrateAlreadyOnTarget` | The deployment already routes to the requested target. If it's `pending` with no deploy queued yet, enqueues one instead of no-op'ing silently. |
| `MigrateNotMigratable` | The deployment doesn't exist or isn't in a migratable status (`active`, `failed`, `pending`). |
| `MigrateSourceMoved` | The deployment's actual current cluster no longer matches the `SourceClusterID` the caller expected (a second migration landed first) — repoints a still-`pending` row's stored spec back to where it actually is and re-enqueues, so the job doesn't leave a live race half-applied; errors on a lost race instead of silently no-op'ing, so River retries. |

`InFlightMove(dep, clusters)` (used by both the deploy handler and
`populateAdminDeploymentPlacement`) derives "this deployment is mid-migration,
headed to cluster Y" from data that's already there: a `pending` deployment
whose stored spec JSON names a different cluster than
`dep.EffectiveClusterID()` currently does. Nothing new is stored for this —
it's a read over the existing (spec, row) pair. This is also new this week
(`2e49d3ded`, "Show a deployment's in-flight cluster move") — astro-queen's
deployments list previously showed only the stale current cluster during a
migration with no indication a move was under way.

`MigrationEventMessage` / `AccountMigrationEventMessage` format the
`deployment_events` row a migration writes. **Their prefixes are matched by
SQL in `admingrpc.ListClusterMigrations`** (see the comment in
`internal/admingrpc/migrations.go`) — changing the message shape without
updating that query silently breaks the Migrations audit page.

`admingrpc.placementOrphaned(accountClusterIDs, deploymentClusterID)` (not
part of `clusterplacement` itself, but the thing that decides whether a
deployment needs migrating) is list-membership: a deployment is orphaned
when its cluster isn't in the account's current allowed set. An account with
an empty allow-list orphans nothing (matches `account.IsAllowed`'s
unrestricted-when-empty rule), and an `undeployed` deployment is never
flagged, since there's nothing left to redirect.

## `k8scache`: a shared primitive, not a cluster concept

`internal/k8scache.Cache` is a small Redis-or-noop cache interface
(`Get`/`Set`/`Invalidate`, plus a `GetMany` fast path for backends that
support multi-key fetch) with a 15-second TTL constant and a `ListKeyPrefix`
constant scoped to one specific use (`ListDeployments`'s namespace-list
cache). Its name suggests it's part of the cluster registry; it isn't — it's
a general caching primitive that predates and is unrelated to the packages
above. `accountcache`, `blueprintcache`, `deploycache`, `knowledgecache`,
`listcache`, and `obssummary` all build their own key prefixes against the
same `Cache` interface, and most River workers take one as a dependency.
`clusterplacement.Migrator` holds one only to call
`deploycache.Invalidate(ctx, cache, accountID)` after a migration — it's a
consumer like any other, not a special relationship.

## What's still moving

This area had five commits land in the eight days before this doc's
last-verified date, three of them behavioral, not cleanup:

- **`a41d2bf62`** (2026-08-13) unified the primary-vs-additional cluster
  model into "every cluster is a row." This is the single biggest reason
  both `01-spec/` documents in this area are stale — see below.
- **`e18e19f81`** (2026-08-20) introduced `clusterid.Resolver` to fix the
  "default" alias / unrecorded-target bug described above, and landed the
  (initially empty) `account_clusters` table in the same PR, ahead of the
  code that would use it.
- **`5f09ae62e`** (2026-08-20) reworked `MigrateDeployment`'s return shape
  from a boolean to the four-outcome model above, specifically to close a
  status race that could leave a deployment stuck `pending`.
- **`2e49d3ded`** (2026-08-20) added `InFlightMove` so the admin console
  shows a migration in progress instead of a stale cluster id.
- **`0fd4b6e74`** (2026-08-21) fixed local dev, which had been silently
  broken (no ingress domain, no metrics backend) since the primary/additional
  unification removed the env vars local mode depended on and nothing filled
  the gap until this commit.

None of this reads as unfinished or half-built — every commit above is a
complete, tested change, not a partial branch of logic. But three real
things are worth flagging as **not yet built**, despite spec text that
implies otherwise:

- **No `config_source_hash` optimization.** `cluster-registration-config-spec.md`
  proposes skipping a row write when the config-derived hash is unchanged.
  The schema has no such column; `UpsertFromConfig` always writes/updates
  every synced cluster on every boot, every replica.
- **No required-region constraint.** `multi-cluster-account-support-spec.md`'s
  Non-Goals section explicitly excludes this, and nothing in `account.IsAllowed`
  or `resolveTemplateClusterID` checks region at all — an account's allowed
  clusters can span any regions with no validation. This is a documented
  non-goal, not a gap, but worth confirming if you're relying on it.
- **`RegisterCluster`/`EnableCluster`/`DisableCluster`/`UpdateCluster` are
  gone**, not merely deprecated — `admingrpc/clusters.go` has no such
  methods, and the `clusters` table comment says so directly ("the
  (now-removed) RegisterCluster RPC"). If you're reading proto definitions
  that still reference these, they're stale scaffolding, not a live surface.

## Where the specs disagree with this doc

Two drafts in `01-spec/` describe this area, and neither matches the shipped
system on its central architectural claim:

- **`multi-region-cluster-support-spec.md`** describes the primary cluster as
  an env-var-only singleton with no row in `clusters`, additional clusters
  registered at runtime via `RegisterCluster`/`EnableCluster`/`DisableCluster`
  admin RPCs, and an `enabled` flag gating which clusters accept traffic.
  None of this is true today: the default cluster has a row like any other,
  those three RPCs don't exist, and there's no enabled/disabled distinction
  at all — every row present in `clusters` is usable, full stop.
- **`cluster-registration-config-spec.md`** gets the boot-sync mechanism
  itself right — config file, one-time startup sync, `DeregisterCluster`'s
  FK-gated manual delete surviving as the only mutation RPC, no background
  reconciler — and correctly describes `RegisterCluster`/`UpdateCluster`/
  `EnableCluster`/`DisableCluster` going away. But it still assumes **the
  primary cluster keeps its own separate env vars and is never in the
  cluster-config file** ("Never includes the primary/default cluster, which
  keeps its own env vars"). `a41d2bf62` landed the day after this spec's
  date and removed exactly that split — the primary is now just the config
  entry whose id matches `DEFAULT_CLUSTER_ID`, sourced from the same file as
  every other cluster.

Both specs are marked superseded as part of this pass, pointing at this doc
for the as-built system. `cluster-registration-config-spec.md`'s boot-sync
mechanics are still the right mental model for *how config gets into
`clusters`* even though its primary-cluster framing is wrong — this doc, not
either spec, is now the one to trust for both.

`multi-cluster-account-support-spec.md` is **not** marked superseded: reading
`account.ClusterBindings`, the `AddAccountCluster`/`RemoveAccountCluster`/
`SetAccountDefaultCluster`/`ListAccountClusters` RPCs, and
`resolveTemplateClusterID`/`enforceAccountClusterPlacement` in
`handlers/deploy.go` against that spec's design shows they match closely —
same table shape, same admin RPC set, same two-phase template/deploy
resolution, same "account with no bindings is unrestricted" rule, same
explicit non-goal on region constraints. It's still marked `Draft` in its own
header, but its content is accurate to the shipped system and is a reasonable
design-intent reference alongside this doc, not a stale one.

## Implementation locations

- Cluster identity: `apps/astro-server/internal/clusterid/clusterid.go`
- Config loading and boot sync: `apps/astro-server/internal/clusterconfig/` (`entry.go`, `sync.go`)
- Shared field validation: `apps/astro-server/internal/clusterfields/` (`config.go`, `validate.go`)
- Per-deployment config resolution: `apps/astro-server/internal/clustercfg/resolve.go`
- Cross-cluster migration: `apps/astro-server/internal/clusterplacement/` (`placement.go`, `migrate.go`)
- Cluster row storage (boundary, not covered in depth here): `apps/astro-server/internal/clusterstore/store.go`
- Cluster client/entry cache: `apps/astro-server/internal/k8s/registry.go`
- Account allow-list: `apps/astro-server/internal/account/clusters.go`
- Deploy-time resolution and enforcement: `apps/astro-server/handlers/deploy.go` (`resolveTemplateClusterID`, `enforceAccountClusterPlacement`, `validateDeployTargetCluster`)
- Admin RPCs: `apps/astro-server/internal/admingrpc/accounts.go` (account bindings), `internal/admingrpc/clusters.go` (cluster registry), `internal/admingrpc/placement.go` (`placementOrphaned`)
- Migration River job: `apps/astro-server/internal/riverqueue/migrate_cluster.go`
- Boot wiring: `apps/astro-server/main.go` (`buildRegistryConfig`, `exitOnLocalClusterMisconfig`, `backfillPrimaryBindings`, `backfillPrimaryPlacement`)
- Shared cache primitive: `apps/astro-server/internal/k8scache/cache.go`
