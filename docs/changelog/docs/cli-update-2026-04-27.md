## Summary

Rewrites the public Fern documentation to reflect the current CLI command surface and platform concepts. Adds new pages covering the full workflow from local development through deployment and agent management.

## Design

### New pages

- **Your first blueprint** — explains `ast blueprint push <name>`, pushing the same project under multiple names (staging/prod), `--allow-account-override` for org scope mismatches, and `ast blueprint get --card` to preview the agent card.
- **Deploy your first agent** — covers `ast blueprint deploy`, variable injection (`--var KEY=@SECRET`), adapters (web/insecure-web/slack), and `--dry-run`.
- **Managing your agents** — operational lifecycle after deployment: inspect (`agent list/get`), observe (`agent logs --tail`), control (`pause/resume/restart`), update (`agent redeploy`), and delete.
- **Working with accounts** — explains account scoping, `ast account list`, `ast account switch`, and switching back with `-`.
- **Managing secrets** — account vault: `secrets create/list/update/delete/import` (import now uses `--file/-f`), secrets vs plain variables, and referencing vault values at deploy time with `KEY=@SECRET_NAME`; `KEY=@` as shorthand when the secret name matches the variable name.

### Updated pages

- **Your first project** — trimmed to four steps; adds "Build your agent logic" step explaining that `ast project create` generates `AGENTS.md` and `CLAUDE.md` so coding agents (Claude Code, Copilot, Cursor) have the context needed to implement a correct agent; replaces "scaffold" with "agent harness" throughout.
- **Welcome** — updated Concepts to cover the full lifecycle (Project → Spec → Blueprint → Agent → Account → Secrets → Agent Card); updated cards and next steps.
- **CLI overview** — updated workflow list and command group table to match current command names.
- **CLI reference** — full rewrite for new command groups (`project`, `blueprint`, `agent`, `secrets`, `spec`, `settings`, `account`); adds `blueprint build` section; corrects push signature (`ast blueprint push [name]`, name optional — falls back to spec) and flags (`--visibility`, `--no-build`, `--allow-account-override`); updates `secrets import` to `--file` flag syntax; removes stale `validate`/`explain` top-level entries.
- **Make your agent discoverable** — adds "Previewing your agent card" section with `ast blueprint get <name> --card`; updates push references to `ast blueprint push`.
- **Authentication** — removes stale credential storage and API authentication sections; adds next-steps link to accounts page.
- **Push to registry** — updated to `ast blueprint push <name>` syntax.

### Navigation

New pages wired into `docs.yml` under **Get started** (blueprints, deploy-agent, managing-agents) and a new **Platform** section (accounts, secrets, agent-card).

## Migration

No action required. Documentation-only change.
