# Registry Pull-Through for Multi-Cluster Image Access

**Version**: 1.0
**Status**: Draft
**Date**: 2026-07-01
**Branch**: `multi-cluster-ecr`

## Overview

Tenant images live in a single ECR registry in the astro-server home account/region (`REGISTRY_URL`). Today deployed pods pull **directly** from that ECR: `image_resolver` rewrites the stored proxy reference into an ECR URL and the kubelet authenticates with its **node IAM** identity. This only works when the node's IAM role is in the same account as the ECR.

For EU (cross-region) and BYOC (separate customer AWS account) clusters this breaks:
- BYOC nodes have no IAM path to the home-account ECR — pull fails hard.
- Cross-region pull is possible but pays egress + latency and still needs IAM reach.
- ECR pull-through cache cannot help — it only fronts *public* upstreams, never a private ECR.

This spec routes pod pulls **through `astro-registry`** instead of ECR — for **every** cluster, primary included. The registry already terminates the Docker Registry v2 pull path for developers; we extend it so kubelets pull from it too. ECR becomes a backing store that clusters never see or authenticate against. A cluster needs only HTTPS to the registry, blob egress to S3, and a **cluster pull credential**.

Rollout is deliberately phased: **the primary cluster switches first and must run reliably on pull-through before any additional cluster is registered.** The primary is the proving ground — same account and region as ECR, so the pull-through mechanism itself is exercised end-to-end without the cross-region/cross-account variables layered on top. Pull-through is unconditional (no runtime flag); rollback during the proving phase is redeploying the prior server build, which resolves tenant images to ECR again.

This also removes the ECR-URL translation from `image_resolver`: the pod reference becomes a registry-host reference and the `{env}-tenant-{id}` mapping stays hidden inside the registry.

## Terminology

- **Home account / ECR** — the astro-server account holding the single tenant ECR (`REGISTRY_URL`), us-east-1.
- **Primary cluster** — the env-var-defined cluster astro-server runs against, in the home account. Pulls directly from ECR via node IAM today; **migrated to pull-through first** by this spec. It has no `clusters` row, so its CPC and its "homed here" tenant set (`accounts.cluster_id IS NULL`) are handled via config rather than a row (see Pull auth).
- **Additional cluster** — a runtime-registered row in `public.clusters` (EU, BYOC). Adopts pull-through in phase 2, once the primary is proven.
- **Cluster pull credential (CPC)** — a per-cluster, opaque, long-lived, pull-only secret. Delivered to astro-server (via External Secrets) and injected as a per-namespace `imagePullSecret` for the proxy registry host; the kubelet exchanges it at `/token` for a registry token scoped to only the tenants homed on that cluster. Not a WorkOS credential.
- **Registry token (R)** — the existing HS256 registry-scope JWT (`iss=astro-registry`) minted by `/token`, verified locally on `/v2/*`. Reused unchanged; only its issuance path for CPCs is new.

## Goals

1. **Clusters need zero AWS/ECR/IAM.** Only HTTPS to the registry, S3 blob egress, and a CPC. Uniform across primary, EU, non-AWS, and BYOC accounts — one pull path everywhere.
2. **Single ECR, single source of truth.** No replication, no per-cluster ECR, no cross-account push/assume-role, no image copy.
3. **Kill server-side ECR translation.** `image_resolver` emits a registry-host reference; ECR host, region, and `{env}-tenant-` prefix are internal to the registry.
4. **Separate pull auth from push auth.** Push stays WorkOS-interactive. Pull uses the CPC — a per-cluster machine credential with a lifecycle we control (long-lived, no forced short TTL).
5. **Isolate and contain clusters.** A compromised or offboarded cluster can be blocked in isolation (revoke its CPC) without touching others, and a cluster can only pull images of tenants homed on it — never another cluster's tenants.
6. **De-risk by phasing, not by dual paths.** Prove pull-through on the primary (in-account, low-latency, comparable against the known-good direct path) before extending it to additional clusters. The end state is one uniform pull path, not a permanent primary/additional split.

## Non-Goals

1. **Regionalizing image bytes.** One backing store in the home region. Cross-region pull latency/egress remains (accepted: "single ECR, simplest topology"). Regional backing stores are a later milestone if latency demands it.
2. **Changing the push path.** Developer `ast push` and its WorkOS token flow are untouched.
3. **Per-image or per-deployment pull scoping.** The CPC is scoped at the granularity of *tenants homed on the cluster* (`accounts.cluster_id`), not individual images or deployments. A cluster can pull any repo of an account assigned to it. Finer scoping is a later refinement (see Open Questions).
4. **Public/base-image handling.** Docker Hub pull-through and third-party pass-through in `image_resolver` are unchanged.

## Design

### Pull path

```
┌────────┐   pull registry/{ns}/{img}   ┌──────────────┐   verify R    ┌─────┐
│kubelet │─────────────────────────────▶│ astro-registry│──────────────▶│ ECR │
│(add'l  │   401 → /token (Basic CPC)   │  (v2 proxy)   │  (home acct)  └──┬──┘
│cluster)│◀─────────────────────────────│               │                 │307 S3
└───┬────┘   Bearer R                    └──────────────┘                 ▼
    │                                                                   ┌─────┐
    └───────────── GET layer (pre-signed S3 URL, direct) ─────────────▶│ S3  │
                                                                        └─────┘
```

1. **Manifest / auth** flow through the registry: 401 challenge → `/token` with the CPC in the Basic password slot → registry token R → `/v2/*` verified locally (identical mechanics to the push token flow in `docs/03-architecture/registry-token-auth.md`).
2. **Blob bytes bypass the registry — already true today.** On a blob `GET`, ECR returns a 307 to a pre-signed S3 URL. `rewriteLocationHeader` only rewrites Location headers whose host matches the ECR backend (upload sessions); the S3 download redirect is a *different* host and is passed through unchanged, so the kubelet fetches layers directly from S3. The registry is already a control-plane component for pulls (manifests, token, redirects) — no code change needed. Pre-signed URLs are time-limited and self-authorizing.

Requirement: additional-cluster nodes need egress to the home-region S3 endpoint. This replaces the ECR IAM requirement, not adds to it.

### Image resolution — the server passes the pushed reference through

The stored spec already carries the developer-facing pull URL for tenant images: `registry.astropods.ai/{account}/{image}:{tag}` (what `ast push` wrote). The deployment template **passes tenant images through unchanged** — no ECR host, no `{env}-tenant-` prefix, no account→id rewrite, no `ECRNamespace`. Public images (ECR pull-through cache) and third-party images are unchanged.

All name→repo mapping lives in **astro-registry**: at pull time it resolves the `{account}` namespace (by name, or by id for transitional refs) to the tenant's ECR repo, exactly as the push path already does. Whatever the spec carries just works, and account rename/transfer history — if ever needed — is the registry's concern, not the control plane's. This is unconditional; rollback during the proving phase is redeploying the prior server build.

### Pull auth: the cluster pull credential

**Distinct from WorkOS push auth.** The `/token` handler gains a second issuance path selected by credential shape:

- **CPC format** `astrocp_{clusterID}_{secret}` — the prefix routes to the CPC path; `clusterID` identifies the requesting cluster; `secret` is high-entropy random. The reserved `clusterID` `primary` denotes the primary cluster.
- **Storage** — for additional clusters, only `sha256(secret)` persists, in a new `clusters.pull_key_hash` column (plaintext shown once at issuance). For the **primary**, which has no row, the hash lives in registry config (`PRIMARY_PULL_KEY_HASH`), consistent with the primary being a deployment artifact rather than a table row.
- **Authentication** — registry parses `clusterID`, loads the hash (row for additional, config for `primary`), and constant-time compares. Rejects if the cluster is missing, `enabled=false`, or the hash mismatches. Overwriting the hash instantly revokes that one cluster.
- **Authorization (cross-cluster isolation)** — for each requested `repository:{ns}/{img}:pull`, the registry resolves `{ns}` → `accountID` and grants `pull` **only if the tenant is homed on the requesting cluster**: for an additional cluster, `accounts.cluster_id == clusterID`; for `primary`, `accounts.cluster_id IS NULL`. Scopes for tenants homed elsewhere are dropped (spec intersection behavior, never an error). The minted R is pull-only and carries only the authorized repos.

The result: a compromised cluster A credential can pull only A's tenants, never B's or the primary's, and is revoked without disturbing any other cluster.

WorkOS routing is unchanged: a JWT in the password slot goes to the existing validator; an `astrocp_`-prefixed string goes to the CPC path. `/v2/*` scope enforcement (pull for GET/HEAD) is unchanged — it verifies R's `access` claim locally, so the per-cluster authorization decision is made once at issuance, not per request.

The registry gains read access to `public.clusters` (hash + `enabled`) and `public.accounts` (`cluster_id`, resolved from `{ns}`). Same shared Postgres it already uses for membership.

### Credential provisioning (astro-server injects; infra delivers)

The CPC is a static, per-cluster credential — nothing about it is per-deployment. Infra generates it and **delivers it to astro-server** (via External Secrets, as `REGISTRY_PULL_CREDENTIAL` — the same pattern as the AI-gateway master key); astro-server injects it into each tenant namespace at apply time.

- **Infra side.** Generate the CPC (`astrocp_{clusterID}_{secret}`), hold it in Secrets Manager, and project it into the astro-server namespace via an ExternalSecret. For the primary, also set the registry's `PRIMARY_PULL_KEY_HASH` = `sha256(secret)`. No node bootstrap, no per-node docker config.
- **Server side.** On every apply, before pods are created, the applier writes a `kubernetes.io/dockerconfigjson` Secret (`astro-registry-pull`) for the proxy host into the tenant namespace and links it to the namespace `default` ServiceAccount's `imagePullSecrets`. Every tenant pod uses the default SA (none set a `serviceAccountName`), so the credential is picked up automatically — no pod-spec changes. The kubelet then runs the standard v2 token handshake (401 → `/token` with the CPC → Bearer) on pulls from the proxy host. No-op when the credential/proxy host are unset (local dev).

**Rotate / revoke** — regenerate a cluster's CPC, overwrite its stored hash (the `clusters` row, or `PRIMARY_PULL_KEY_HASH` for the primary) and the Secrets Manager value; the next apply (or ESO refresh + redeploy) re-injects it. Contained to one cluster; no shared signing secret rotates. The CPC has no forced expiry, so there is no timed refresher.

This is the key contrast with an ECR-token approach: because we own the credential, there is no 12h expiry driving a mandatory refresher, and because it is per-cluster, revocation and blast radius are contained to a single cluster.

Why server-injected rather than a node-level docker credential: injection is per-namespace, so it composes with per-cluster credentials and future finer scoping, and it needs no change to node bootstrap/AMIs. The trade-off is that astro-server carries the (small) injection path below.

### Registry as critical infrastructure

Pull-through moves the registry from a dev-only component into the pull path of **every** pod — the primary's included, from phase 1 onward (scale-up, restart, rollout, node cycling). It must be treated as tier-1 before the primary is switched:
- Run ≥N replicas (already stateless / horizontally scalable — no shared in-memory state).
- Own an availability SLO; registry unavailability → `ImagePullBackOff` cluster-wide.
- Passing the S3 redirect through (above) keeps its load control-plane-sized, not proportional to image bytes.

This is the central risk of the primary-first choice: unlike the earlier "primary stays on direct-ECR" framing, the home cluster now *does* depend on the registry. Phase 1 hardens the registry (replicas, SLO, monitoring) before switching the primary, and keeps the prior server build one redeploy away until pull-through is proven.

## Schema

`public.clusters` gains:

| Column | Type | Notes |
|---|---|---|
| `pull_key_hash` | `bytea` (nullable) | `sha256` of the cluster's CPC secret. NULL until first issuance. Overwrite = rotate/revoke. |

No change to `deployments`, `accounts`, `agent_versions`. `accounts.cluster_id` continues to drive which cluster a tenant is homed on — now also the key the registry authorizes CPC pulls against (`NULL` = primary). The primary has no row, so its hash lives in config, not this column.

## Configuration

Registry (new):
- Read access to `clusters` (`pull_key_hash`, `enabled`) and `accounts` (`cluster_id`), over the existing `DATABASE_URL` — no new connection. Used only at `/token` issuance for CPC auth + cross-cluster scoping, not per `/v2/*` request.
- `PRIMARY_PULL_KEY_HASH` — `sha256` of the primary cluster's CPC (the primary has no `clusters` row). The `clusterID` `primary` authenticates against it.

Server:
- `ProxyRegistryHost` must be reachable + TLS-valid from every cluster, primary included (already the developer-facing registry host).
- `REGISTRY_PULL_CREDENTIAL` — the cluster's CPC, used to build the tenant `imagePullSecret`. Delivered via External Secrets; empty disables injection (local dev).

Infra (per cluster):
- Generate the CPC, hold it in Secrets Manager, and project it into the astro-server namespace via ExternalSecret as `REGISTRY_PULL_CREDENTIAL`. For the primary, also set the registry's `PRIMARY_PULL_KEY_HASH` = `sha256(secret)`.

No new signing secret: R is still minted with `REGISTRY_TOKEN_SECRET`. `REGISTRY_URL` / `AWS_REGION` stay as the registry's ECR backend config; they leave the server's deploy path entirely (tenant image resolution no longer references ECR).

### Phase 1 — primary on pull-through (the proving ground)

1. **Registry hardening** — bring the registry to tier-1 (replicas, SLO, monitoring) *before* it is in the primary's pull path.
2. **Schema** — add `clusters.pull_key_hash` (Atlas, no reader yet).
3. **Registry** — add the CPC `/token` issuance path (primary via `PRIMARY_PULL_KEY_HASH`, `NULL`-home scoping); blob redirect pass-through already exists. Backward compatible: WorkOS push/pull unchanged.
4. **Server** — the deployment template passes tenant image references through unchanged (the ECR-URL rewrite and `ECRNamespace` are deleted), and the applier injects the `astro-registry-pull` Secret + default-SA link in each tenant namespace from `REGISTRY_PULL_CREDENTIAL`. Namespace→repo resolution moves to the registry.
5. **Infra** — deliver the primary CPC to astro-server via ExternalSecret (`REGISTRY_PULL_CREDENTIAL`) and set `PRIMARY_PULL_KEY_HASH` on the registry.
6. **Prove it** — deploy an agent on the primary; confirm pulls (cold + warm), scale-up, rollout, and node-cycle all pull through the registry with layers coming from S3 directly. Watch registry SLO and `ImagePullBackOff` rate. Bake until observably reliable before phase 2.

Rollback during phase 1 is redeploying the prior server build (which resolves tenant images to ECR again) — there is no runtime flag. Because the change is a code path, not a toggle, keep the previous image one revert away until pull-through is proven.

### Phase 2 — additional clusters

7. **Server + admin** — issue-on-register generates a per-cluster CPC and stores its `sha256` in the cluster row; infra delivers that CPC to astro-server (per-cluster) so the applier injects it into that cluster's tenant namespaces.
8. **Acceptance** — register a test additional cluster, deploy a tenant agent homed on it, confirm pull-through and direct-from-S3 layers; confirm a *different* cluster's CPC is denied that repo (isolation); rotate the CPC and confirm the old secret is rejected and the rolled credential restores pulls.

## What this kills

- Server-side ECR URL construction and `{env}-tenant-` prefixing for tenant images — deleted from the server (`image_resolver` becomes a passthrough; the template emits proxy-host refs). The one translation lives only in the registry.
- Direct-to-ECR pulls from clusters, and the ECR/IAM reach that required — clusters now need only HTTPS to the registry, S3 egress, and the injected pull secret.
- Any need for cross-account ECR access, assume-role, repo policies, per-cluster ECR, or replication for BYOC/EU pulls.
- The permanent primary/additional pull-path split — the end state is one uniform path.

## What this preserves

- The push path: `ast push`, WorkOS token flow, membership/permission checks at `/token` for user credentials.
- Direct-ECR node-IAM pull as a **rollback path** (redeploy the prior server build) during the primary proving phase — not as the steady state.
- The registry token format, HS256 signing, and `/v2/*` local scope enforcement.
- Docker Hub pull-through and third-party pass-through in `image_resolver`.
- Agent-transfer correctness — the pushed reference keeps its original account namespace, which the registry resolves to the repo where the image physically lives.

## Decisions

1. **Primary first, then multi-cluster — decided.** The primary switches to pull-through and must run reliably (agreed soak period, registry at tier-1, `ImagePullBackOff` and SLO watched) before any additional cluster is registered. This inverts the earlier "primary stays on direct-ECR" stance: the home cluster now depends on the registry, mitigated by hardening it in phase 1 and keeping the prior server build one redeploy away as rollback. End state is one uniform pull path.

## Open questions

1. **Promotion to phase 2 is an operator judgment call.** Phase 1 is done when the primary is observably running clean on pull-through — no formal metric gate. `ImagePullBackOff` rate and registry SLO are there to watch, not to pass a threshold. Additional clusters are registered once the primary is trusted.
2. **Blob egress path — resolved (no change needed).** Verified: the proxy already passes ECR's S3 download redirect through unchanged (only same-host upload redirects are rewritten), so layer bytes go kubelet→S3 directly and the registry stays control-plane-sized. Requirement stands: additional-cluster nodes need S3 egress to the home region. No stream-through bottleneck to design around.
3. **Authorization granularity — account-home vs deployment (phase-2 correctness gap).** The registry resolves the image namespace to an account and scopes off that account's `accounts.cluster_id`. This diverges from where the pod actually runs when the deployment's cluster differs from the resolved account's home cluster — a **cross-home deployment** (`deployments.cluster_id` ≠ `accounts.cluster_id`) or a **cross-cluster transfer** (the pushed reference keeps the original account's namespace, whose home cluster may not be where the new owner deploys). Both are **harmless in phase 1** (everything is `NULL`-home = primary) but real once clusters differ. The fix belongs in the registry: at issuance, verify the requesting cluster actually runs a deployment referencing `{ns}/{img}` (consult `deployments`) rather than trusting the namespace's home cluster. Recommended: ship account-home scoping for phase 1; resolve before enabling cross-cluster placement or transfers.
4. **CPC per cluster vs per namespace.** Recommended: one CPC per cluster, same value written into each tenant namespace's pull secret on that cluster. Per-namespace credentials add lifecycle cost without a threat-model gain — the cluster legitimately pulls every tenant homed on it, and cross-cluster isolation is already enforced at issuance.
5. **CPC expiry.** Recommended: no forced expiry; rotation/revocation is an explicit per-cluster admin action. Revisit if a scheduled rotation policy is required for compliance.
6. **Primary CPC provisioning.** The primary's CPC hash is config (`PRIMARY_PULL_KEY_HASH`). Open: is a plain config secret acceptable, or should the primary also become a `clusters` row purely to unify credential storage? Recommended: keep it config, consistent with the primary-as-deployment-artifact model established in the multi-region spec.
