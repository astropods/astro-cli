# Container env: dedupe, scope per role, foundation for unified table

## Summary

Two related bugs and the structural foundation to retire both:

1. **Duplicate env entries on the agent container.** `buildContainerStatuses` returned one row per `envFrom` resolution AND one per direct `env` entry, so a name in both produced two rows. Manifested in the UI's Variables panel: switching the active container in the dropdown left ghost DOM rows from the previous container, because rows were keyed by env name and React's reconciliation under duplicate keys is undefined.

2. **Cross-role secret leak onto the agent.** Variables targeting only `interface.*` (e.g. `SLACK_BOT_TOKEN` with `Targets=["interface.slack"]`) were ending up in the agent's mounted Secret. The messaging container's own scoped Secret already carried them; including them in the agent's bundle was a plain leak.

Plus the data-layer foundation for the unified `deployment_build_env` table the spec describes (`docs/01-spec/unified-deployment-env-spec.md`) — populated on every apply, consumed by the API, but not yet load-bearing for the K8s applier (that's the cleanup PR).

## Design

### Dedupe (`apps/astro-server/handlers/deploy.go`)

K8s already defines env precedence: a container's direct `env` entries override `envFrom` resolution on the same name. The API now mirrors that. `buildContainerStatuses` resolves `envFrom` into a per-name map, then overlays direct `env` (direct wins), preserving insertion order. One row per name. Decision lives on the server; clients render exactly what the container will see.

### Scope filter for the agent's bundle (`apps/astro-server/internal/k8s/spec_applier.go`)

`scopeAgentEnv` filters `result.SecretData` and `result.ConfigMapData` to exclude any variable whose `Targets` are entirely `interface.*`. The messaging container's own scoped Secret reads from the unfiltered resolved data, so it's unaffected; the filter only narrows the agent's view. Variables with mixed targets (e.g. `["agent","ingestion"]`) stay.

### Unified deployment env: foundation only

New table `deployment_build_env` keyed by `(deployment_id, role, env_name)` with `value_encrypted`, `nonce`, `is_secret`, `source`, plus optional `user_var_name` / `account_var_ref` / `optional` provenance for `source='user_var'` rows. Per-row encryption with the deployment data key.

`Resolve(ds, opts)` is a pure function that walks the deployment spec and emits one `Resolution` per `(role, env_name)`. Per-role rows for `agent`, `messaging`, `collector`, `knowledge:<name>`, `ingestion:<name>`. `Targets` are load-bearing — vars only reach roles their `Targets` list. Per-store renaming (`POSTGRES_USERS_USER` for the second postgres store) handled here.

`deployer.populateBuildEnv` is invoked on every successful apply: builds `ResolveOptions` from the rehydrated spec + bound state + auto-generated knowledge creds (surfaced via `ApplyResult.AllCredentials`), runs `Resolve`, encrypts and persists rows. Best-effort — failure logs and the apply continues.

### API + UI annotation (no behavior change for legacy deployments)

`GetDeployment` overlays `Source` and `IsSecret` from `deployment_build_env` onto each container's env entries. Values still come from K8s; rows supply categorical provenance the UI uses for the badge color and authoritative redaction. When no rows exist (legacy or pre-migration deployments), the UI falls back to the existing `From`-based heuristic and `isSensitiveEnvVar`. The frontend's badge palette grows: `user_var` → input, `knowledge_cred` → credential, `auth_token` → credential, `platform_meta` → platform, `service_url` → service, `adapter_config` → adapter, `derived` → derived.

## Migration

Side effect on cutover: agent + ingestion pods rolling-restart because the env-hash changes (`scopeAgentEnv` removes interface-only entries from the agent's CM+Secret). Application code sees the same env vars it needs; the entries that disappear are the leaked ones it was never supposed to see. Coordinate with a deploy window if the deployment count is large.

No DB migration required for existing deployments — `deployment_build_env` is additive; rows are populated lazily on the next apply for each deployment.

The K8s applier itself is unchanged: it still wires the agent's CM+Secret as a global pair (now scoped to non-interface entries) and uses `knowledgeCredEnvVars` for per-store credential `secretKeyRef` entries on the agent. The follow-up cleanup PR replaces this with per-role projection from `deployment_build_env` and drops `deployment_variables` / `deployment_resolved_keys`.
