# docs: correct stale platform facts

## Summary

Several pieces of contributor-facing documentation had drifted from the code they describe. Because `agents.md` (aliased as `CLAUDE.md`) is the first thing both humans and coding agents read, inaccurate entries actively misdirect work. This change corrects four verified drifts. It is documentation-only — no runtime behavior changes.

## Design

Each correction was verified against the source of truth in-tree:

- **`agents.md` — astro-queen description.** Was "Bubbletea TUI admin client." The app has no `bubbletea` dependency; it is a Cobra CLI that serves an embedded React SPA (`apps/astro-queen/web_embed.go` → `//go:embed all:web/dist`) backed by the AdminService gRPC API. Description updated to match.
- **`agents.md` — Moon targets cheatsheet.** The `deployment:*` line listed a non-existent `deployment:playground` target and omitted several real ones. Corrected against `deployment/moon.yml` (actual tasks: `astro-client`, `astro-registry`, `astro-server`, `clean`, `collector`, `messaging`, `smoke-test-astro-client`). The hard-coded total count was removed in favor of the existing "regenerate with `moon query tasks`" note, since the count cannot be asserted accurately by hand.
- **`README.md` — smoke-test cadence.** Said tests run "every 15 minutes"; `.github/workflows/smoke-test.yml` schedules `cron: "0 * * * *"` (hourly). Corrected, with the workflow/cron cited.
- **`docs/04-guides/tanstack-query.md` — `refetchOnWindowFocus`.** Guide claimed `true`; `apps/astro-client/src/lib/queryClient.ts` sets `false`. Guide updated to match the code. *(If `true` was the intended behavior, this instead surfaces a code/doc mismatch for a maintainer to reconcile — see the PR description.)*

## Migration

None. Documentation-only.
