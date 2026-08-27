# Experiment flags — two coexisting mechanisms

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

"Experiments" in astro-client refers to two genuinely different mechanisms
that happen to share a settings page and the word "experiment." Don't
conflate them: one is a client-only UI toggle with no server involvement,
the other is a server-owned per-account row that gates real backend
behavior.

## How to tell which kind a flag is

Check where its value is read:

- If it's read from `useExperiments()` /
  `apps/astro-client/src/lib/experiments.ts`, it's a **local flag**: a
  `localStorage` key, client-only, no server call.
- If it's read via `useAccountExperiment(account, key)` (from
  `apps/astro-client/src/api/queries/accounts.ts`), which calls
  `GET /api/v1/accounts/:account/experiments/:experiment`, it's a
  **server-owned account experiment**: a database row, checked by backend
  code on every relevant request.

The two never overlap in name or storage: local flags are keys in the
`Experiments` interface (`lib/experiments.ts`); server flags are
`experiment.Key` constants (`internal/experiment/store.go`).

## Local flags (localStorage)

`apps/astro-client/src/lib/experiments.ts` implements a small module-level
store:

- `Experiments` is a fixed interface (`{ evals: boolean }` as of this
  writing) with a `DEFAULTS` object — every flag defaults to `false`, so a
  new one can't ship enabled by accident.
- State persists to `localStorage` under key `astro:experiments`, and a
  module-level `Set` of listeners plus `useSyncExternalStore` fan the value
  out to every `useExperiments()` consumer in the tab without a remount.
- The `storage` event only fires in *other* browser tabs, so same-tab
  updates are pushed manually via `notify()` after every `setExperiment()`
  call; the `storage` listener handles picking up writes made in sibling
  tabs.
- `hasExperiments` is `true` whenever `DEFAULTS` has at least one key. It
  exists so `ExperimentsSettings.tsx` can redirect away when there's nothing
  local left to show, without also hiding server-owned switches on the same
  page (see below).
- There is no server call anywhere in this file. A local flag only ever
  changes rendering in the browser that set it; it has no effect for other
  members of an organization and doesn't survive clearing site data.

`AgentTabBar.tsx` is the one production consumer: it reads `experiments.evals`
to decide whether to show the Eval tab on agent detail pages.

## Server-owned account experiments

`internal/experiment/store.go` defines a small, fixed set of `Key` constants
and a `Store` backed by one table, `account_experiments` (columns:
`account_id`, `experiment`, `enabled`, `updated_at`). A missing row means
disabled; there's no separate "unset" state.

Two experiments exist today:

| Key | Meaning | Where it's checked |
|---|---|---|
| `fine_grained_access` | Opts an organization into FGA-based deployment access (owners/admins keep access, creators become deployment owners, other members need an assigned role) instead of the default org-role model. | `authz.NewAccountExperimentResourceGate`, wired in `main.go`, used by deployment-access rollout checks. |
| `prompt_classification_stats` | Enables per-account classification of coding-tool prompts (purpose/topic breakdown) surfaced on an Insights detail page. | `handlers.GetAccountInsights` / `GetAccountInsightsSource`, gated via an `experiment.Gate` built with this key. |

### Read/write API

`handlers/experiments.go` exposes both experiments through one pair of
routes, keyed by a URL slug distinct from the stored `Key` so the wire name
can outlive an internal rename:

- `GET /api/v1/accounts/:account/experiments/:experiment`
  (`handlers.GetAccountExperiment`)
- `PUT /api/v1/accounts/:account/experiments/:experiment`
  (`handlers.UpdateAccountExperiment`)

Both require `org:manage` permission on the account
(`middleware.RequireAccountPermission`), so a personal account's owner can
still manage their own personal-account experiments. The slug-to-key map
(`experimentsBySlug`) also carries per-flag policy:

- `orgOnly`: rejects personal accounts. Only `fine-grained-access` sets this
  — it's meaningless for an account with no other members.
  `prompt-classification-stats` runs off an account's own telemetry, so it's
  available to personal accounts too.
- `invalidates`: whether flipping the switch must also invalidate the
  deployment cache, because that cache reads the flag per request. Only
  `fine-grained-access` sets this.

A successful write logs an audit event (`auditlog.AccountUpdateExperiment`).

### Reading a flag server-side

Backend code never queries `account_experiments` directly outside the
`experiment` package. It goes through an `experiment.Gate`
(`NewGate(store, key)`), constructed once per key in `main.go`
(`fgaExperiment`, `classificationExperiment`) and threaded into the handlers
that need it. `Gate.Enabled(ctx, accountID)` is the only read path used by
feature code.

## Frontend settings pages

Both pages live under the "Account & org settings UI" area
(`apps/astro-client/src/pages/settings/**`); this doc is the canonical
source for the flag *mechanism* itself, not the settings-page UI shell.

- **`ExperimentsSettings.tsx`** (personal account settings): renders the
  local `evals` toggle from `useExperiments()`, plus the server-owned
  `prompt_classification_stats` switch scoped to the reader's personal
  account. It redirects to `/settings/account` only when there are no local
  flags left *and* no personal account context — so retiring the last local
  toggle can't accidentally hide the server-owned one.
- **`OrgExperimentsSettings.tsx`** (organization settings): renders both
  server-owned switches (`fine_grained_access`, `prompt_classification_stats`)
  scoped to the organization from the route (`orgSlug`). It has no local
  flags — those are personal-browser state, not something an org setting
  page would show for other members.

Both pages use the same query hooks, `useAccountExperiment` /
`useUpdateAccountExperiment` (`apps/astro-client/src/api/queries/accounts.ts`),
which call the read/write API above and invalidate the query on a successful
write.

## Verify

- `go test ./internal/experiment/...` (store read/write, `Gate.Enabled`).
- `go test ./handlers/... -run Experiment` (slug resolution, `orgOnly`
  rejection, cache invalidation on write, audit logging).
- `cd apps/astro-client && bun x vitest run src/lib/experiments.test.tsx src/pages/settings/OrgExperimentsSettings.test.tsx`
  (`ExperimentsSettings.tsx` itself has no dedicated test file as of this
  writing — only the local-flag store and the org settings page do).
