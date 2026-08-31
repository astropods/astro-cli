# astro-queen — Admin Console

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-31

Queen is the operator-facing admin tool: a single Go binary (`apps/astro-queen`) that embeds a React SPA and talks to astro-server's admin gRPC API. There is no separately-deployed "Queen service" — an operator runs the binary locally, and it connects out to whichever environment's astro-server they point it at. For mTLS certificate setup, see [`04-guides/queen-mtls-certificate-generation.md`](../04-guides/queen-mtls-certificate-generation.md); this doc covers the architecture, the real feature surface, and the auth model.

## Process model

```
queen <local|preview|prod> admin
  -> Cobra CLI (apps/astro-queen/cmd/)
  -> dials astro-server's AdminService over gRPC (internal/client/)
  -> starts a stdlib net/http server on 127.0.0.1:8888 (internal/server/)
       /api/admin/*  -> translates one HTTP request into one AdminService RPC,
                         marshals the proto response straight to JSON
       /api/astro/*  -> reverse proxy to astro-server's normal customer-facing
                         HTTP API, authenticated as the operator via device login
       /*            -> serves the embedded SPA (go:embed all:web/dist),
                         falling back to index.html for client-side routing
  -> opens the browser
```

**The AdminService is not a separate service.** It's a second `grpc.Server` started inside the same astro-server process as the public HTTP API, on its own port (`ADMIN_GRPC_PORT`, default 9091) — see `startAdminGRPCServer` in `apps/astro-server/main.go`. The real topology is: Queen, run on an operator's machine, dials straight into whichever astro-server pod is serving that environment's admin port. If that port isn't configured (`ADMIN_GRPC_CERT_FILE`/`KEY_FILE`/`CA_FILE` unset) and the deployment isn't local, astro-server doesn't start the admin server at all — there's no environment where admin access is silently open by default.

## Auth model

**mTLS is the only gate, and it grants all-or-nothing access.** `admingrpc.ServerCredentials` sets `tls.RequireAndVerifyClientCert` against a configured CA; the gRPC server itself has no interceptor chain (`grpc.NewServer(opts...)` with only `grpc.Creds(...)`) and no code anywhere inspects the client certificate's identity beyond "signed by the trusted CA." Once the handshake succeeds, the caller gets every RPC on `AdminServiceServer` unconditionally — there is no per-operator identity, no role distinction (e.g. support engineer vs. SRE), and no scoping enforced server-side.

The client certificate is shared, not per-operator: every operator fetches the same `queen-bee-client` cert/key/CA from the **Astro** 1Password vault via `queen login` (see the mTLS guide). Practically, this means **possessing the shared 1Password-vault cert is equivalent to full admin access** to every account, deployment, and cluster in that environment. Treat vault access to that item as the actual access-control boundary, not anything enforced in code.

`local` connects insecurely (no certs) to a local astro-server bound to loopback only — this path is explicitly gated on `cfg.Deployment.IsLocal()` server-side and never reachable from another host even if `ENVIRONMENT=local` is set on a routable interface.

A separate, unrelated auth path exists for the **API Client** page: it authenticates as a real WorkOS user via device login, purely to let an operator call astro-server's normal customer-facing HTTP API as themselves for support/debugging. That path has nothing to do with admin gRPC access.

**The one place Queen touches customer-facing authorization** is `GetDeploymentAccess`, which is read-only: it shows which users/groups have access to a given deployment and why (org role, deployment-role assignment, or FGA group membership). It doesn't gate anything Queen itself does.

## Feature surface

The four-word summary ("cluster status, deployments, jobs, observability") undersells this — Queen also does admin billing, account-cluster placement, and several smaller operator tools. By area, with the astro-server package that implements it (`apps/astro-server/internal/admingrpc/`):

| Area | What an operator can do | Backing package/file |
|---|---|---|
| Deployments | List/inspect across all accounts with live pod/container/volume detail; view logs and env vars; delete, restart a pod, stop, wake up, rollback, **redeploy** (also how cross-cluster migration is triggered — see below), repair a stored spec | `server.go` |
| Accounts | List/rename/soft-delete/hard-purge; billing detail (Metronome contract, provisioning status, card, spend) and retry/force-resume; set or clear the account's spend limit at any value (see below); repair Metronome ingest aliases; recover Langfuse/Bifrost provisioning; per-account cluster placement bindings (add/remove/set-default); cache invalidation | `accounts.go`, `billing.go`, `cache.go` |
| Clusters (registry) | List registered clusters with health, deregister (blocked if accounts/deployments still reference it, with a blockers list), health check, refresh ECR pull secrets | `clusters.go`, `k8s_cluster.go` |
| Cluster status (live) | Live per-namespace K8s state — Deployments, StatefulSets, Pods (with container status, resources, security context), Services, Ingresses, NetworkPolicies, Events | `server.go` (`GetClusterStatus`) |
| Jobs | Browse the raw River queue (state/kind/queue filters), inspect one job's args/errors, cancel/retry, view queue-level counts, pause/resume a named queue, trigger any job kind ad hoc with JSON args | `server.go` |
| Alerts | View the alert-condition catalog and active pending/firing alerts per deployment+workload; clear, mute (with duration), unmute | `alerts.go` |
| Audit findings | List open/resolved findings (severity-tagged); acknowledge | `audit_findings.go` |
| Quota requests | List pending/approved/denied; approve with a grant amount and note, or deny with a note. Covers the `spend_limit` key too, whose grant is a ceiling rather than a count (see [`quota.md`](quota.md)) | `quota.go` |
| Authorization resources | List an account's FGA resources and operations; start a guarded async reset (a River job, `authorization.resource_reset`, in the maintenance queue) that rebuilds them from scratch | `authorization_admin.go` (`internal/authorizationadmin` for the actual rebuild logic) |
| Evaluators (drift check) | List declarative drift-check evaluators, run a sweep, list drifted deployments for one evaluator, fix drift for a specific deployment | `server.go` |
| Blueprints (agents) | List agents fleet-wide with build counts; drill into build history | `server.go` |
| Feedback | List in-app feedback submissions | `server.go` |
| Messaging cache | Force-refresh the messaging adapter cache for a deployment | `server.go` (`RefreshMessagingCache`) |
| Migrations | Read-only audit trail over cluster-migration jobs and events, cross-linked to Deployments/Jobs | `migrations.go` |
| API Client | Generic OpenAPI explorer against astro-server's customer-facing API, authenticated as the operator | `server.go` (`ProxyHTTP`, `GetAuthConfig`) |

**Defined in the proto but not reachable from Queen today** (no route, no frontend usage): `ListImages`, `QueryDatabase`, `RegisterCluster`/`EnableCluster`/`DisableCluster`/`UpdateCluster` (the manual cluster-registration flow — see [`01-spec/cluster-registration-config-spec.md`](../01-spec/cluster-registration-config-spec.md), which proposes removing these in favor of automated boot-sync), and `StartRiverUI`/`StopRiverUI`/`GetRiverUIStatus` (superseded by the bespoke jobs UI above). `ListOutboundDomains` is reachable only via `queen <env> icons domains`, a CLI-only tool for brand-icon coverage unrelated to the admin console.

**Authorization resource reset is environment-gated, currently preview-only.** `StartAuthorizationResourceReset` checks a `resetEnabled` flag wired from `FGA_AUTHORIZATION_RESET_ENABLED`, which is `true` in `config/astro-server/preview.env` and `false` in `prod.env` as of this writing — the reset button exists in the Queen UI everywhere, but the RPC refuses in production.

## Setting an account's spend limit

`SetAccountSpendLimit` (`internal/admingrpc/billing.go`, reached from the
Billing section of the account page) writes the account's own spend limit to
any value, with no bound of its own. `billing.MaxSelfServeSpendUSD` governs
what a *customer* may choose for itself; it does not govern an operator grant.

The control renders whatever the account's billing state, so an operator does
not have to clear a suspension or wait on a quota request to change the number.
It disables only when the account has no Metronome customer, because there is
then nothing to write a limit to.

One write, two numbers. A limit above the account's ceiling raises the ceiling
(`account_limits`, resource `spend_limit`) to match, then enqueues a gateway
budget re-derive. Leaving them apart would clamp the AI gateway back to the
lower number and leave the customer's own limits dialog refusing the limit in
force. Clearing the limit leaves the ceiling alone: a ceiling is a grant, and
dropping a limit is not revoking it.

This is the operator path to the same numbers the self-serve quota request
reaches by review. See [`quota.md`](quota.md)'s spend-limit section for the
ceiling's semantics and the customer-facing route.

## Cross-cluster migration, concretely

There's no standalone "migrate" button. The flow is:

1. An operator changes an account's allowed clusters (Account Detail page → `AddAccountCluster`/`RemoveAccountCluster`/`SetAccountDefaultCluster`).
2. If a deployment's current cluster falls outside the account's new allowed set, `placementOrphaned` flags it. `ListDeployments`/`GetDeployment` surface this as `placement_orphaned` plus a derived (not stored) `migrating_to_cluster_id` when a migration is already in flight, and the Deployments/Deployment Detail pages render a warning with a human-readable hint.
3. The operator clicks **Redeploy** on the affected deployment, which calls `ReapplyDeployment`. The confirm dialog states plainly that it will move the deployment to the account's default cluster before redeploying, and that existing pods may stay on the previous cluster until the deploy worker finishes.
4. Server-side, this enqueues both a cluster-migration job and a deploy job.
5. The **Migrations** page is a read-only audit trail over these jobs and the resulting `deployment_events` — useful for confirming a migration happened, not for triggering one.

## Not deployed as a running service

There's no Kubernetes manifest, Terraform resource, or CI publish/release step for astro-queen — CI (`build-queen.yml`) builds and cross-compiles the binary on every relevant push/PR and uploads it as a GitHub Actions artifact, nothing more. Operators build or download the binary and run it locally against whichever environment's admin gRPC endpoint they need (`admin.astropod.ai:443` for preview, `admin.astropods.ai:443` for prod — the ingress/DNS routing for those hostnames lives in the private `astro-infra` submodule, out of scope for this repo's docs). See [`../../apps/astro-queen/docs/setup.md`](../../apps/astro-queen/docs/setup.md) for building and running it, including local dev.
