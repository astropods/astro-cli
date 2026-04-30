# Cleanup: drop the auto-injection path for knowledge credentials

## Summary

Stage 1 of the cleanup that lands on top of the unified `deployment_build_env` foundation. Removes the duplicate path the agent had for knowledge-store credential env vars, and pulls ingestion containers into the same secretKeyRef wiring.

## Design

Before: `template.go` auto-injected `${knowledge.<X>.credentials.<Y>}` references into `agent.Environment` for every knowledge entry. The resolver materialised those into the agent's full Secret. The applier *also* emitted per-store `secretKeyRef` entries on the agent container via `knowledgeCredEnvVars`. Same name on the agent twice — `envFrom`-resolved literal *and* direct `secretKeyRef`. K8s precedence picked the direct entry, so the literal in the Secret was wasted bytes; the agent's Secret carried provider creds it didn't need; and ingestion containers (which `envFrom` the agent's Secret) were the only consumers of the literal copy.

After:
- `template.go` no longer injects credential refs into `agent.Environment`. Connection coordinates (`HOST`/`PORT`/`URL`) still flow through it — those are non-secret service URLs.
- `knowledgeCredEnvVars` becomes the single source of truth for credential env vars on the agent. Its provider key map gains `DB` for postgres so the database name reaches the agent the same way `USER` and `PASSWORD` do.
- Ingestion containers (`Job`, `CronJob`, `BuildIngestionDeployment`) gain an `ExtraEnv` field and receive the same `knowledgeCredEnvVars` slice the agent does. They get per-store renamed `secretKeyRef` entries directly, no longer dependent on the agent's full Secret.
- The agent's projected Secret shrinks: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, the per-store renamed forms, and `REDIS_PASSWORD` no longer appear in it. They live exclusively in the per-store cred Secrets and are referenced via `secretKeyRef`.

## Migration

Side effect on cutover: rolling restart of the agent + every ingestion pod when the env-hash and direct-env list change. Application code sees the same env vars via the same names. The agent's Secret is smaller (provider creds gone); ingestion pods now carry per-store `secretKeyRef` entries instead of mounting them through the agent Secret.

The old test that asserted the agent's deploy Secret carried POSTGRES_* values is updated: the values now live in per-store cred Secrets, and the merged effective env check on the agent (envFrom + container.env) confirms the agent still sees every expected key.

## What still has to land

- Drop `deployment_variables` and `deployment_resolved_keys`. Requires switching `RehydrateSecrets` (and the deploy handler) to read user variable values from `deployment_build_env` user_var rows instead. Not in this PR.
- Per-role projection of `deployment_build_env` into the K8s applier (`<agent>-<build>-<role>-config` / `-credentials`), eliminating the messaging-only Secret carve-out and replacing the agent's global CM+Secret with role-scoped resources. Not in this PR.

These are the structural cleanups that retire the old wiring entirely. They're deferred so this PR can stay focused on removing the duplicate credential path without rewriting how every container's env gets mounted.
