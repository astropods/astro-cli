## Summary

`ast dev` already injected `POSTGRES_HOST`, `POSTGRES_PORT`, and `POSTGRES_DB` into the agent container for container-mode postgres knowledge stores, but **not** `POSTGRES_USER` or `POSTGRES_PASSWORD`. The sidecar booted with whatever the user happened to declare as a `knowledge.inputs` entry (or refused to boot if they declared nothing) and the agent had no credentials to connect with — agents fell into permanent `OperationalError` retry loops in local dev unless the developer hand-wired the variables through `agent.inputs` or top-level `inputs`.

The prod deploy path (`apps/astro-server/internal/k8s/spec_applier.go: generateKnowledgeCredentials`) auto-generates user `astro` plus a random password and injects all five vars as secrets into both pods. Local dev now mirrors that.

## Design

Single resolver in `apps/astro-cli/internal/compose/builder.go` returns the `POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB` triple from `envVars` (`.env` / `ast configure`) with prod-matching fallbacks:

| Var                 | Default           | Source of truth                                |
|---------------------|-------------------|------------------------------------------------|
| `POSTGRES_USER`     | `astro`           | matches prod `generateKnowledgeCredentials`    |
| `POSTGRES_PASSWORD` | `localdev`        | stable so the pgdata volume survives restarts  |
| `POSTGRES_DB`       | `SanitizeDBName(s.Name)` | already the prod default                |

Both call sites use the same resolver, so the sidecar and the agent always boot with matching credentials:

- `BuildProject` — postgres sidecar service env (replaces the previous `POSTGRES_DB`-only injection).
- `BuildEnvironment` — agent container env (now writes the full triple instead of just `POSTGRES_DB`).

`envVars` overrides keep working, so `.env` files and `ast configure` stay authoritative when set. Existing `knowledge.inputs` / `agent.inputs` declarations also still flow — they layer **after** the auto-inject and can override per-side if a project intentionally diverges, though for postgres credentials they're now redundant.

The password default is stable rather than random by design: a per-run random would invalidate the `knowledge-*-data` volume on every `ast project start` and force a manual `docker volume rm` between sessions.

## Migration

No spec changes required. Existing projects that worked around this by declaring `POSTGRES_USER` / `POSTGRES_PASSWORD` under `agent.inputs` or top-level `inputs` can delete those entries — the auto-inject covers them. **If the workaround used `POSTGRES_USER: postgres`**, leaving it in place will now mismatch the sidecar's `astro` default; either remove the workaround or also set `POSTGRES_USER=postgres` in `.env` so both sides agree.

Projects with an existing `knowledge-*-data` volume initialized by an earlier `ast dev` need a one-time `docker volume rm <project>-knowledge-*-data` so postgres re-initializes with the new `astro` superuser.
