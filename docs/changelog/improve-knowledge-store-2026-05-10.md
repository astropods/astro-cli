## Summary

Polish pass on the knowledge store flows. Most of these are bug fixes around state that didn't reflect reality — picker mode hardcoded to "shared", delete failing with a generic 500 instead of pre-checking bindings, KMS errors in local dev when the fallback path was right there. Plus copy fixes (the post-create hints described a YAML schema and CLI flag that don't exist) and provider expansion (Neo4j + Pinecone).

## Design

### Knowledge binding picker (deploy form)

- Mode now derives from the actual binding state instead of a `useState("shared")` initial that ignored async-loaded template values. `rawArn` (binding from form or template default) drives whether the toggle reads "Shared" or "Local"; an `explicitlyShared` flag covers the case where the user clicked Shared but hasn't picked a store yet. Bindings arriving async from the template response now flip the toggle automatically.
- Toggle order swapped — Shared first, Local second.
- Native `title=""` replaced with `Tooltip` + `TooltipProvider` (no global provider in this app, so wrapped locally).
- Empty state when no compatible stores exist now reads "No *PostgreSQL* knowledge stores connected" with a neutral `Database` icon + an inline Add store button, instead of a bare two-line panel.

### Delete knowledge store

- `DeleteKnowledgeStoreDialog` takes a `boundAgents?: BoundAgent[]` prop. When non-empty, renders a blocked-state Dialog (no name-confirm input, no destructive button) listing the active bindings (`agent name → knowledge.<name>`). Both call sites (`KnowledgeStores` list page, `SettingsPanel` detail page) forward `store.bound_agents`.
- The 409 fallback message is kept as a safety net for the race where bindings appear between the page fetch and the click.

### Knowledge store create flow

- After creation completes (or post-provisioning polling resolves), `ConfigureForm` now navigates straight to the store detail page (`navigate(knowledgeDetailPath(store.name), { replace: true })`) instead of rendering a `SuccessStage` step. The "store added" page added no signal beyond what the detail page already shows; the YAML and CLI snippets it displayed (`knowledge: - store: ...`, `ast dev --source ...`) were inaccurate — neither shape exists in the spec/CLI. `SuccessStage.tsx` deleted.
- The Public-mode connection panel shows the two NAT gateway IPs (`3.213.168.251/32`, `13.222.89.6/32`) as inline copyable pills with a one-line conditional hint. Hardcoded as `NAT_GATEWAY_IPS` for now — easy to swap for a region-aware API later.
- Empty-state copy on the detail page: "Select this store as a shared database when deploying an agent" (the previous copy referenced an `astropods.yml` `knowledge` block, which doesn't exist for managed bindings — they're a deploy-time concern).

### Provider expansion

Three gating lists in `knowledge-utils.ts` (`ALL_PROVIDERS`, `MANAGED_PROVIDERS`, `EXTERNAL_PROVIDERS`) now include:

- **Neo4j** — managed + external. Server-side support already complete (`provider.go` defines image, ports, healthcheck, credentials).
- **Pinecone** — external only (`Cloud: true` provider; SaaS, can't run as a managed StatefulSet).

`PROVIDER_PORTS["neo4j"]` corrected from `7474` (HTTP) to `7687` (Bolt) — Neo4j drivers connect over Bolt, and the spec's `URLScheme` is already `bolt`. The healthcheck masked this (TCP-dial fallback for non-7474 ports), but agents would have failed at runtime when their driver hit 7474.

### Server: KMS-required credentials fallback

`ResolveCredentials` had three paths but path 1 (KMS) errored out instead of falling through to path 2 (k8s Secret) when KMS was unavailable. For managed stores the StatefulSet's k8s Secret holds plaintext credentials and is the documented local-dev fallback — that's exactly the case where this path matters. Now: if `EncryptedDataKey + dbCreds` are present but `kmsClient == nil`, **managed** stores fall through to the secret reader; **external** stores still error (no Secret exists for them). The strict `KMSRequired` test still passes (it injects nil for both `kmsClient` and `secretReader`, so it still hits the early-return).

### Deployment tile error_message

`DeploymentTile` rendered the `Error` badge but ignored `deployment.error_message`. Now it surfaces the message inline below the meta row when `status === "error"`, in the tile's status color, with `whitespace-pre-line break-words` for multi-line messages.

## Migration

No action required.
