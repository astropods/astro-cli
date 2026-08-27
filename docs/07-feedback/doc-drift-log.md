# Doc/code drift log

A running log of two kinds of thing found while reading or fixing docs, that
weren't fixed on the spot: real code issues, and doc-vs-code drift whose fix
needs domain knowledge, product judgment, or a change bigger than the pass
that found it. Not a backlog of one-line doc typos or factual fixes — those
get fixed in place, per [`../README.md`](../README.md)'s rule.

Each entry: what, where, why it's deferred rather than fixed, and **Caught
by:** how it was found — PR review, scheduled audit (see
[`../04-guides/doc-honesty-audit.md`](../04-guides/doc-honesty-audit.md)),
verify-on-use check, the weekly freshness check, or other. That tag isn't
for this file's own sake: it's the only way to eventually tell which of
those channels is actually catching things, versus adding process for no
return. Applies to every 2026-08-27-and-later entry, including the three
same-day entries that predate this instruction (backfilled for
consistency); entries from 2026-08-26 and earlier aren't retagged. Remove
an entry once it's addressed (fixed, or a
deliberate decision is recorded in the relevant doc's own text) — don't let
this file grow into a second backlog system.

## 2026-08-26 — docs/skills system audit (drift, taxonomy, coverage)

Found during a full audit of `docs/` and `.claude/skills/` against the actual
codebase. Fixed on the spot: `authentication-flow.md`, `supabase-knowledge-store.md`,
`knowledge-store.md`'s PrivateLink section, `account-deletion-plan.md`'s and
`messaging-oidc-auth-spec.md`'s banners, `agents.md`'s app inventory, the
`astro-audit-cli-docs` skill's stale paths, and the doc-freshness hook's
silent-failure risk. Deferred below because each needs more than a targeted edit.

**Real code issue found while fixing a doc (not a doc problem):**
- `apps/astro-server/internal/deployment/deployment_spec.go`'s
  `DeploymentSlackAuth` struct comment says "user_id and anyone are rejected
  at deploy." The actual validator (`handlers/deploy.go`,
  `validateAuthorizationSpec`) accepts `user_id` grants on Slack — a second
  comment right next to the check explains why (Slack identity resolves via
  a linked identity mapping). The struct comment is simply stale; harmless
  today since it's not load-bearing logic, but worth a one-line fix next
  time that file is touched.

**Docs that actively contradict another, already-canonical doc (pick one,
fix or delete the other):**
- `05-implementation/ecr-namespace-migration.md` — describes a
  `TemplateInput.ECRNamespace`/name-keyed registry-proxy scheme; the real
  code is UUID-keyed. Neither this doc nor `registry-pull-through-spec.md`
  describes what actually runs; needs a rewrite or a new doc.

**Active plan depends on infrastructure that was never built (product
judgment needed, not a doc fix):**
- `06-plan/deployment-lineage-hardening.md`'s PR2 ("Recover orphaned
  deployments label `source_account_id`") assumes a "reconciler" reads a
  `astro.dev/source-account-id` namespace label and calls
  `RecoverOrphanedDeployment` to repair lineage. Neither the reconciler
  design nor `RecoverOrphanedDeployment` was ever built — the real
  replacement, `deploycontroller`, is architecturally different (informer-
  driven, not polling) and has no equivalent mechanism. Confirmed via grep:
  zero references to the namespace label anywhere in `internal/deploycontroller`
  or `internal/k8s`. This PR is still listed as pending work in an actively-
  maintained plan ("Master Plan (current)"), so it needs someone who knows
  the intended lineage-recovery design to decide whether to redesign PR2
  against `deploycontroller` or drop it, not a docs pass.

**Also found while writing the astro-queen doc, fixed on the spot (not
docs — real config/build issues):**
- `apps/astro-queen/docs/setup.md` documented a `queen configure`/`queen
  server` command shape and a flat `config.yaml` (server/cert_file/key_file/
  ca_file) that no longer exist — the real commands are `queen login` +
  `queen <local|preview|prod> admin`. Rewrote it.
- `apps/astro-queen/moon.yml`'s `start` task ran `./bin/queen server`,
  which isn't a registered subcommand — would have errored every time.
  Fixed to `./bin/queen local admin`.

Deployment/K8s, Evals, and astro-queen are all now mapped in
`docs/README.md` — their canonical docs are fixed/written as of this pass.

## 2026-08-26 — map expansion sweep, Phase A (map hygiene)

Found while sweeping the repo for area-map coverage gaps ahead of expanding
`docs/README.md`. Fixed on the spot: linked `github-connection.md` and
`notifications-spec.md` into the area map, added `**Status:**`/
`**Last verified:**` to `github-connection.md`, expanded the Deployment/K8s,
Evals, Insights, and Auth rows' code-path globs to include packages that were
always in scope but omitted, and fixed `agents.md`'s stale
`packages/astro-identity-gen` path (ported to
`apps/astro-server/internal/identitygen` as a Go package).

**Coverage gaps identified, deferred to their own doc-writing pass (see the
map-expansion plan):** variables/secrets vault, account lifecycle & identity,
messaging/chat interfaces, auditlog, systemaudit, quota, experiment (feature
flags). The cross-cutting Go `Store`/sentinel-error, `internal/openapi`
route-wiring, and Go testing conventions gaps are now closed, as of Phase F
below. Blueprint push/build/register lifecycle, cluster config & placement, and observation &
alerts are now mapped as of Phase B, below.

## 2026-08-26 — map expansion sweep, Phase B (blueprint lifecycle, cluster config, observation alerts)

Wrote `03-architecture/blueprint-lifecycle.md`, `blueprint-deploy-flow.md`,
`cluster-configuration.md`, and `observation-alerts.md`; bannered
`multi-region-cluster-support-spec.md`, `cluster-registration-config-spec.md`,
and `observation-alert-evaluator-spec.md` superseded where the code confirmed
it; fixed `notifications-spec.md`'s stale audience-resolution claim. All now
mapped in `docs/README.md`.

**Real code issues found while writing docs (not fixed, harmless today):**
- `apps/astro-server/handlers/observation_alerts.go`'s
  `DeploymentAlertItem.Severity` doc comment says `"warning" | "error"`; the
  actual values are `info`/`warning`/`critical` via `Severity.String()`.
- `apps/astro-client/src/lib/api.ts`'s `DeploymentAlert.severity` type is
  `"warning" | "critical"`, omitting `"info"`. Harmless only because both
  `info`-severity conditions (`cpu_over_provisioned`,
  `memory_over_provisioned`) are currently disabled — would silently
  mis-render if either were re-enabled without this type being fixed first.
- `apps/astro-server/handlers/agents.go`'s `RegisterAgentRequest.Visibility`
  doc comment says it's "only applied on first registration"; the code
  applies it on every register call.
- GitHub-triggered builds compute the same non-fatal spec-validation warnings
  CLI push does but discard them (`"[]"` hardcoded) instead of persisting to
  `agent_versions.validation_warnings` — a CLI-pushed and a GitHub-built
  version of the identical spec end up with different stored warnings. Looks
  like an oversight, not a design choice; needs someone who knows whether
  GitHub-build warnings were deliberately suppressed before it's changed.

## 2026-08-26 — map expansion sweep, Phase C (variables vault, account lifecycle & settings, messaging/chat, agent runtime panels)

Wrote `03-architecture/variables-secrets-vault.md`, `account-lifecycle.md`,
`account-settings-ui.md`, `messaging-chat-interfaces.md`, and
`agent-runtime-panels.md`. AI Gateway is covered in its own entry below (a
separate agent resolved that gap directly). All five now mapped in
`docs/README.md`. Fixed on the spot: `apps/astro-client/CLAUDE.md`'s "side
panel pattern" section claimed Configure/Trace/Chat all render as `SidePanel`
children; Configure is actually a full page, and `PodDetailPanel` (not
mentioned before) does use `SidePanel` — corrected the list and the example.

**Real code issues found while writing docs (not fixed):**
- `internal/accountvars`'s "can't change the `secret` flag without a new
  value" rule lives only in the handler, with a comment admitting it's ad
  hoc — no store-level invariant stops a future direct write from producing
  an inconsistent `(secret, nonce)` pair.
- AI Gateway dev-key revocation during account purge is best-effort: LiteLLM
  holds no FK back to Astro, so a revoke failure during purge leaves a
  working credential upstream with no local record to alert on it later
  (the code's own comment acknowledges this).
- Slack directory sync (`UsersList`) caps at 50 pages × 200 users (10k
  members); beyond that it silently returns a partial directory, logged only.
- `apps/astro-server/handlers/workload_metrics.go`'s `GetWorkloadMetrics`
  does a live per-request K8s `Pods().Get()` for CPU/memory limits and PVC
  names, data the DB deployment spec already owns, on every poll while the
  pod Metrics sub-tab is open. This is exactly the pattern `agents.md`'s K8s
  usage rule warns against, though it fails soft. (`GetDeploymentRuntime`,
  `GetDeploymentStatus`, and `GetDeploymentEvents` were checked and are
  compliant, reading DB-persisted snapshots.)
- The Slack bare-user-ID matcher and display-name fallback logic are
  duplicated, not shared, between `internal/slackidentity`/
  `handlers/slack_display.go` and
  `apps/astro-client/src/components/activity/insights-user-identity.ts` —
  synced only by a comment in the Go file asking that all three stay in
  sync.

**Coverage gaps identified, deferred:** quota/resource-limits UI
(`ResourceLimitsSection.tsx`) and the Slack connector settings surface have
no canonical doc yet.

## 2026-08-26 — billing subsystem audit

Found during a full-stack read of `internal/billing/*`, `handlers/billing.go`,
the Metronome provider/metering, and the billing client. None of these are
documented anywhere as intentional; all look like real gaps.

**Server — gating/state machine**
- `otherSelfLimitInAlarm` (webhook_jobs.go) reads live provider thresholds
  before `ApplySignal` decides whether to keep `usage_limit_active` set; the
  final `Recompute` transaction only serializes the write, not this
  read-then-decide step. Two webhook jobs for the same account could race.
  The same shape of race exists between `liftSelfLimit` (a client PUT) and a
  concurrent webhook.
- `BillingResumeWorker` doesn't re-check current status before restoring
  deployments, unlike the suspend worker. A resume job queued behind
  backlog could fire after a *later* signal re-suspended the account.
- A webhook arriving between `CreateCustomer` and `SetBillingCustomerID`
  being persisted (billing_provision.go) resolves to `ErrAccountNotFound`
  and is dropped permanently — the "unknown customer is a permanent no-op"
  design assumes the customer isn't ours yet, not that it's ours and just
  not linked.
- Most latch setters (`SetAlert`, `SetForceSuspend`, `SetCreditsExhausted`,
  `SetUsageLimit`) bump `updated_at` on every redelivery even when the flag
  was already true, which can push genuinely stale accounts further back in
  `ListForRecompute`'s staleness-ordered queue.

**Server — Metronome provider/metering**
- `usage_thresholds.go`'s `metricEventType` has a `default` case (not an
  explicit case per metric) that silently maps any future `UsageMetric` to
  compute's billable-metric ID. Add a metric to `AllUsageMetrics` without a
  matching case here and its thresholds silently bind to compute instead —
  no error, wrong number.
- `CustomerSpend`/`usageSpend` sum credit balances and line items with no
  check that they share a credit type. Safe only because exactly one type
  (USD cents) exists today; `metronome-billing-spec.md` already plans a
  second pricing unit (`ASTRO_CREDIT`) in a later phase, which this will hit
  with no currency-match guard in place.
- `SetCustomerSpendThreshold`/`SetCustomerUsageThreshold` are
  list→archive→create with no lock and no 409-handling on the create call —
  a double-click/retry can surface a false failure even though the
  threshold landed (the write succeeded on the other request).
- `resolveBillingCustomer`'s lazy customer creation has no lock either;
  concurrent first-load billing requests (realistic: multiple tabs) can each
  create a separate Metronome customer and race on which ID persists.
- `billableMetricIDs` cache never invalidates; a metric deleted/recreated in
  Metronome's dashboard (the documented normal provisioning model) leaves
  the cache silently stale until the process restarts.
- An unparseable CPU/memory string on a workload bills as CU=0 with no log
  at all, while the 34-day backdating-limit skip case logs loudly — same
  "can't bill this" outcome, inconsistent visibility.

**Client**
- `ManageLimitsDialog` destructures only `isLoading` from `useBillingSpend`,
  never `isError`. A failed load renders fully-editable, blank fields
  indistinguishable from "no thresholds set," with a working Save button on
  top — unlike its sibling `PayAsYouGoCard` on the same query.
- `formatMoney` (`lib/billing-balances.ts`) has no guard against a malformed
  currency code; `toLocaleString(..., {currency})` throws for a non-ISO-4217
  string, and every caller passes a raw, untyped provider field straight
  through. Combined with the server-side currency-mixing risk above, this
  is a real crash vector once a second pricing unit exists, not just a
  cosmetic bug today.

## 2026-08-26 — AI Gateway / Bifrost-otel docs pass

Wrote [`03-architecture/ai-gateway.md`](../03-architecture/ai-gateway.md),
resolving the "AI Gateway/Bifrost LLM-metering path has no as-built docs"
gap logged above. Covers astro-server's `internal/aigateway/**` and
`handlers/ai_gateway_keys.go` in full; `apps/bifrost-otel/**` gets a short
section (real production code moved into the monorepo in one commit, not
fresh/unstable — see the doc's own note on why it isn't fully re-documented
here instead of cross-linked to `billing-architecture.md` and astro-infra's
own `docs/architecture/16-llm-usage-metering.md`).

**Real code issue found (not fixed, harmless today):**
- `internal/deployer/deployer.go`'s AI Gateway block is commented
  "Rotate-on-redeploy — each call to Apply mints a fresh key and demotes the
  previous to a short-lived prev slot." `Provisioner.EnsureDeploymentKey`
  does the opposite by design (its own doc comment: "No rotation: the key
  minted here lives for the lifetime of the deployment. Redeploys reuse
  it."), and `deployment_ai_gateway` has no `key_id_prev`/`prev_expires_at`
  columns to hold such a slot — the original LiteLLM-era plan
  ([`06-plan/ai-gateway-astro-server.md`](../06-plan/ai-gateway-astro-server.md)
  §4) specced a rotation worker with exactly that two-stage shape, but it was
  never built (no `internal/riverqueue/*rotate*` file exists). The
  `deployer.go` comment describes that unbuilt design, not the shipped
  behavior. Harmless today since nothing reads the comment as a contract,
  but worth a one-line fix next time that file is touched.

## 2026-08-26 — map expansion sweep, Phase E (deliberate defer/fold decisions)

The map-expansion plan's last phase was a short list of areas to fold into an
existing doc rather than write standalone, or to defer outright, recorded
here so the absence reads as a decision, not an oversight.

**Folded into existing docs, both resolved this pass:**
- Claude Code work classification (`internal/classification`,
  `internal/workclassifier`) — added to `03-architecture/insights.md` and the
  Insights area-map row rather than a standalone area; small (824 lines) and
  tightly coupled to the insights rollup it feeds.
- Payment/Stripe card vault (`internal/payment`) — added to the existing
  Billing area-map row's glob; `billing-architecture.md` already documented
  the Stripe-linking behavior in depth, this only needed linking, not new
  writing.

**Flagged, not documented as a convention (there isn't one):**
- Frontend forms. 12 files use `<form>`/`onSubmit` (`Onboarding.tsx`,
  `OrganizationNew.tsx`, `RequestBlueprint.tsx`, `AgentConfigure.tsx`,
  `FeedbackModal.tsx`, `DeploymentChatThreadView.tsx`,
  `InteractionForm.tsx`, `ConnectKnowledgeStoreDialog.tsx`,
  `EditCredentialsDialog.tsx`, `ConfigureForm.tsx` (new-knowledge-store),
  and `useDeployForm.ts`, the outlier). No form library is installed
  (`react-hook-form`/`zod`/`formik`/`yup` all absent from `package.json`,
  confirmed absent from source too), so each form hand-rolls controlled
  state and validation independently rather than following one shared
  pattern — there's nothing consistent here to write a guide about without
  papering over the inconsistency. `useDeployForm.ts` is the real pain
  point: 1,481 lines, validation spread across many independent `useState`
  error slots with no central schema. This reads as an oversight, not a
  deliberate stance: the codebase has 61 runtime dependencies and readily
  adopts abstraction libraries for equivalent problems elsewhere (TanStack
  Query for data fetching is a documented convention in `agents.md`;
  `downshift` is already a dependency), and nothing in the code or docs
  states a reason to avoid one here. If the team wants to invest in this,
  `react-hook-form` + `zod` is the natural pairing, precedented by how
  TanStack Query was adopted for the analogous data-fetching gap — but
  that's a real decision for whoever owns this surface, not something to
  default into as part of a docs pass.

**Deliberately deferred, not a gap:**
- Trading-card / brand-icon / identity-gen "cosmetic" cluster
  (`packages/astro-trading-card`, `packages/astro-brand-icons`,
  `apps/astro-server/internal/identitygen`) — low product impact, and
  `04-guides/brand-icon-coverage.md` already covers the brand-icon half. The
  one real issue in this cluster (the identity-gen package's stale path) was
  already fixed in Phase A. Revisit if this area's churn or impact profile
  changes, not on a schedule.

## 2026-08-26 — billing-overview.md verification surfaces stale Unlimited-plan content in its sibling docs

Found while verifying `03-architecture/billing-overview.md` against code ahead
of stamping it. The overview's Plans table listed a third "Unlimited" package
("the creator's verified email is on an internal domain" → "usage is rated at
zero"). Commit `e69af8608` ("chore(billing): remove the unlimited plan",
2026-08-25) deleted that plan from the code entirely: `billing.Plan` now has
only `PlanCredit`/`PlanNoCredit` (`internal/billing/provider.go`), `plan()`
(`internal/riverqueue/billing_provision.go`) no longer reads a creator's email
at all, and `METRONOME_PACKAGE_ID_UNLIMITED`, `BILLING_UNLIMITED_EMAIL_DOMAINS`,
`hasEmailDomain`, and `AccountStore.GetCreatorVerifiedEmail` are all gone
(confirmed by grep: zero hits in `apps/astro-server`). Fixed `billing-overview.md`
in place (removed the row, "one of three packages" → "one of two") and stamped
it.

`billing-architecture.md` and `billing-data-flow.md` were fixed in a later
pass: every Unlimited-plan reference removed or rewritten (the plan-resolution
tables, the rank-3 and usage-cap prose, the env var table, Known gaps), both
re-stamped `**Last verified:** 2026-08-26` for real this time.

The map-expansion plan's final phase: three convention guides that don't fit
the area map (each cuts across dozens of unrelated features, not one code
path), and an extension to an existing doc. Wrote
[`04-guides/go-store-pattern.md`](../04-guides/go-store-pattern.md),
[`04-guides/openapi-route-wiring.md`](../04-guides/openapi-route-wiring.md),
and [`04-guides/go-testing-conventions.md`](../04-guides/go-testing-conventions.md);
extended `apps/astro-client/CLAUDE.md` with component-props and
component-file-structure sections. Linked all three Go guides from
`agents.md`'s Development Workflow (steps 2 and 5), where they were already
referenced by description but pointed at nothing.

**Real code issues and inconsistencies found (not fixed):**
- `internal/accountvars.Store.Delete` returns a plain `fmt.Errorf(...)`
  instead of a sentinel on a not-found row, inconsistent with its own `Get`
  method's `(nil, nil)` convention and with `clusterstore`'s identical
  zero-rows case, which does use a sentinel.
- `main.go`'s `deployTokenRoutes` (`GET /deployments/authorize`,
  `POST /deployments/feedback/scores`) register directly on the gin group,
  bypassing the `internal/openapi` builder with no comment explaining the
  omission, unlike the one other bypass in `/api/v1` (a binary PDF stream),
  which is explicitly commented as intentional. These two are real endpoints
  invisible to `/openapi.json`.
- Path/query params are always typed as plain strings in the generated
  OpenAPI spec regardless of real type (e.g. a numeric `:index` param) — a
  builder limitation, not a bug, but worth knowing when adding a route.
- Go's Postgres-gating test pattern is split in two with no written rule
  choosing between them: `//go:build integration` + `t.Fatal` on missing
  `DATABASE_URL` (excluded from default `go test ./...`) versus no build tag
  + `t.Skip` (compiled and run by default, silently skipped without a DB).
  Which pattern a given file uses looks like author preference, confirmed by
  reading examples of both, not a documented rule. The new testing-
  conventions guide states this as observed practice rather than inventing
  a rule nobody follows.
- `new-blueprint/LinkConfirmDialog.tsx` uses a plain `type XProps = {...}`
  with no union or derivation reason to justify it, a one-off against the
  `interface XProps` convention documented elsewhere in `apps/astro-client`.
  `components/blueprint-detail/index.ts` is the only barrel `index.ts` in
  the whole `src/components` tree, and even its own Storybook stories
  bypass it and import files directly. `src/pages/<Route>` folder casing is
  a genuine, unresolved split (kebab-case vs. PascalCase) with no
  older-vs-newer trend by commit date, documented as such rather than
  picking a winner.

Frontend forms are logged separately above (not resolved this pass, flagged
with a recommendation for whoever picks it up).

This closes the map-expansion plan started in Phase A. All planned areas are
now mapped or explicitly, recordedly deferred.

## 2026-08-26 — post-completion audit

A full adversarial audit of every doc written across Phases A-F: re-ran all
27 area-map Verify commands for real (fresh, verbose, no cache), and had
independent agents fact-check specific, checkable claims in each new doc
against current code rather than re-reading and trusting them. All 27
Verify commands passed with no new instance of the pipe-escaping bug class.
The fact-check surfaced a real batch of wrong/stale claims, all fixed in
place this pass: `blueprint-deploy-flow.md`'s Deploy-vs-Configure options
table, cache-invalidation list, and poll-interval attribution;
`account-settings-ui.md`'s overstated `ProfileEditor` sharing;
`variables-secrets-vault.md`'s wrong local-KMS env var;
`quota.md`'s false "no migration file" claim (both tables are in
`schema.sql`); `account-profile.md`'s transposed location/pronouns limits;
`agent-runtime-panels.md`'s stale SidePanel "inconsistency" (already fixed
in `apps/astro-client/CLAUDE.md` a few phases ago, the doc just never
caught up) and its off-by-one file-count; `auditlog.md`'s undercounted
indexes; `ai-gateway.md`'s off-by-one test count;
`cluster-configuration.md`'s fabricated code-comment quote;
`go-testing-conventions.md`'s overclaimed "everything under e2e/",
overclaimed shared `testDB` helper name, and a wrong drift-log citation;
`openapi-route-wiring.md`'s invented `DeferredPOST` method; and
`apps/astro-client/CLAUDE.md`'s `BlueprintDetailHeaderProps` example missing
a required prop. `billing-architecture.md` and `billing-data-flow.md` were
found stamped "Last verified: 2026-08-26" while still describing the
Unlimited billing plan (removed in `e69af8608`, 2026-08-25) as live; see
the billing-specific note below for that fix.

**Real code issue found (not fixed):**
- `apps/astro-server/internal/envelope/localkms.go`'s comment says local-dev
  KMS activates on "`K8S_CLIENT_MODE=local`"; the actual gate is
  `cfg.Deployment.IsLocal()` (`ENVIRONMENT=local`), a different config var
  entirely. `variables-secrets-vault.md` had inherited the same wrong
  variable name from this comment; the doc is now fixed, the code comment
  is not. Harmless (a comment, not load-bearing logic), but worth a
  one-line fix next time that file is touched.

**Audit methodology note, for future reference:** one review claimed
`cluster-configuration.md`'s cited commit hashes were unverifiable due to
"no git history in this checkout." That was the reviewing agent's own
environment issue, not a real gap: `git log`/`git show` work fine in this
repo, and all seven hashes it cited resolve to real commits with dates
matching the doc's claims exactly. Don't trust a "can't verify, no git
history" finding without checking `git log` directly first.

## 2026-08-26 — closing the docs-vs-neighbor-code gap

Root cause of a real class of drift: `agents.md` already said to prefer
existing conventions, but had no explicit rule for when nearby code and a
documented convention disagree, so an agent had no tie-breaker. Added one
to Development Workflow step 2 (a documented convention wins over code that
doesn't follow it; that code is inherited drift, not precedent) and
strengthened step 1 to check the relevant app's own `CLAUDE.md`, not just
`docs/README.md`, before touching code.

Extended `apps/astro-client`'s color-lint ratchet pattern to inline styles:
new `local-theme/no-static-inline-style` ESLint rule (`warn`, flags a
literal-valued `style={{}}` property only for a small, deliberately narrow
set of CSS properties Tailwind definitely covers, and explicitly excludes
any string literal calling a dynamic CSS function like `var()`,
`color-mix()`, or a gradient function, since those are computed/theme-
driven values, not lazy static ones, even though the AST sees a plain
string). Baseline 4, enforced via `scripts/check-inline-style-budget.mjs`
in CI, same shape as the existing theme-color budget. Deliberately did
**not** build an equivalent mechanical check for `<Card>` reuse over
hand-rolled chrome (32 hand-rolled spots vs. 9 real `<Card>` imports) —
that pattern is a 3-class combination too ambiguous to check without real
false-positive risk, so it stays convention-only, annotated honestly in
`apps/astro-client/CLAUDE.md` rather than silently asserted as enforced.

Also annotated every rule in `CLAUDE.md`'s Styling section with its real
enforcement status and current violation count (matching `docs/README.md`'s
own habit of admitting gaps), and generalized the "never `style={}`"
rule's two named exceptions into the actual underlying principle (a value
the Tailwind JIT can't statically resolve), so the exception isn't read as
an exhaustive memorized list.

**Real bug found and fixed while building this:** `apps/astro-client/vitest.config.ts`'s
`include` pattern was `src/**/*.test.{ts,tsx}` only — `eslint-rules/*.test.js`
was never covered, so the existing `no-raw-theme-colors.test.js` suite has
never actually executed via `vitest run` or `moon run astro-client:test`,
despite `moon.yml` listing `eslint-rules/**/*` as a cache input (implying
it was tested). Fixed the include pattern; both rule test suites (45 tests)
now run for real, and the full client suite (204 files, 2146 tests) still
passes.

**Early classification attempt, corrected before shipping:** a first pass
at the inline-style rule flagged `color: "var(--card-contrast)"`,
`background: "var(--success)"`, and a `color-mix()` call as violations —
all three are exactly the kind of runtime/theme-driven value the rule is
supposed to exclude, just expressed as a string literal the naive check
couldn't distinguish from a hardcoded one. Caught by manually reading every
flagged violation before finalizing the baseline, not by trusting the
first count.

## 2026-08-26 — pre-rebase check against 22 commits landed on main

Before rebasing this branch onto `main`, read every commit `main` gained
since this branch diverged (22 commits, real product work landing in
parallel) and checked each against the docs this branch just wrote or
touched, since a rebase would otherwise silently import drift nobody
looked at. Per policy, code changes stayed untouched either way; only docs
were fixed or, where a fix needed more than a targeted edit, logged here.

**Fixed on the spot:**
- `agent-runtime-panels.md` described `use-container-log-errors.ts`'s
  background per-container error probe in a way that could read as the
  pod detail panel's own mechanism. Main's PR #2153 removed the panel's
  error-driven banner and auto-tab-switch entirely (it always opens on
  General now; the Logs tab already carries the error with context) and
  removed the panel's fan-out fetch of every container's logs on open.
  The probe survives only for `PodTile.tsx`'s error dot. Doc corrected to
  state this precisely instead of ambiguously.
- `auditlog.md`'s list of handlers that write through the audit path was
  missing `handlers/payment_methods.go`, now true as of main's PR #2156
  (records `payment_method.add`/`.remove`). The doc's own action-list
  section already correctly defers to `actions.go` as source of truth
  for the full action set, so only the handler-file list needed the
  addition, not a rewrite.

**Coverage gaps, deferred (need their own pass, not a rebase-blocking fix):**
- **Apps / machine credentials** — a substantial new subsystem landed on
  main (`internal/appstore`, `internal/connectapps`, new machine-token
  auth middleware, an OAuth-apps scope-picker UI), with main's own
  `01-spec/machine-credentials-spec.md` already describing it ("Apps and
  their credentials implemented; token validation not yet"). Nothing in
  this repo's area map covers it. Real gap, comparable in shape to the
  AI Gateway gap before it got its own doc — worth a dedicated pass.
- `traces-to-eval-dataset.md` doesn't yet mention the dataset-item
  edit/delete endpoints main's PR #2143 added (`PUT
  .../dataset/items/:trace_id/evaluator-outputs`, `DELETE
  .../dataset/items/:trace_id`) to the evaluator flow. Doesn't contradict
  anything the doc currently says (it already frames the evaluator flow as
  "additive, migration in progress"), just doesn't cover the new surface
  yet.

**Real code issue found on main, not fixed (not this branch's code to
touch):** `use-container-log-errors.ts`'s own file-level doc comment still
says it's "Shared by the pod tile (indicator) and the pod detail panel
(banner)" after PR #2153 removed the panel's consumption of it — stale
now, harmless, a one-line fix next time that file is touched.

No other landed commit (billing payment-method removal holds, spend-ceiling
toast copy, usage-refresh timing, evals dataset-item audit fields) touches
a claim in this branch's docs; checked and left alone rather than assumed
clean.

## 2026-08-26 — three follow-up fixes from an external review

**Status field was pure boilerplate.** All 30 `03-architecture/` docs said
the identical "Authoritative — describes the shipped system," which can't
inform a decision since it never varies. Tightened `docs/README.md`'s rule:
Status is now a deliberate per-doc call, with a precise alternative phrase
expected when part of a doc is thinner or more volatile than the rest, not
a default. Applied it to the two docs whose own prose already admitted a
caveat their Status line didn't reflect: `cluster-configuration.md` ("still
settling") and `ai-gateway.md` (the Bifrost-otel section is a short
overview, not a full pass). The rest stay "Authoritative" because that's
still the honest call for them, not because the field defaults there.

**Nothing enforced the stamp.** Built both the cheap and the valuable
version. `scripts/check-doc-stamps.mjs` fails a PR if any
`03-architecture/*.md` lacks the two stamp lines, wired into the existing
`check-doc-links.yml` workflow. `scripts/check-doc-freshness.mjs` compares
each area's Last-verified date against the most recent commit touching its
mapped code paths (reusing `parseAreaTable` from the hook script, per the
review's own observation that the area map already holds what's needed);
runs on a weekly schedule rather than per-PR, since it reports cumulative
drift, not one diff, and never blocks a merge, only flags. Verified the
staleness branch actually fires (not just the empty-state path) by
temporarily backdating one doc's stamp, confirming the flag, and restoring
it before committing anything.

**The `<Card>` gap was labeled but not shrinking.** Built the grep-based
ratchet the review suggested instead of a real lint rule: a line counts as
hand-rolled chrome when it has `bg-card`, `border`, and a `rounded` token
together, same coarse-but-stable heuristic used to establish the baseline.
First pass caught it counting `components/ui/card.tsx` itself (the
primitive's own legitimate implementation) and two Storybook demo files as
violations — excluded `components/ui/**` and `stories/**`, matching the
existing ratchet's own allowlist categories, before settling on the real
baseline (30, not the review's estimated 32). Wired into CI alongside the
other two ratchets.

## 2026-08-27 — CI failure: broken link, plus a real byte-level corruption found fixing it

**Caught by:** other (CI, `check-doc-links.mjs` failing a PR).

`check-doc-links.mjs` correctly failed a PR on `docs/01-spec/machine-credentials-spec.md`'s
link to `access-audiences-spec.md`, a forward reference to a spec that
doesn't exist yet (not this branch's content, landed on `main`). Added it
to `KNOWN_BROKEN_LINKS`, same as the other 13 known exceptions.

While fixing it, found `scripts/check-doc-links.mjs` and
`.claude/hooks/docs-map-check.mjs` each had two literal NUL bytes (`\x00`)
standing in for what should be plain space characters — invisible to a
normal read/diff, only surfaced by `od`/`perl -ne '/\x00/'`. Origin unknown
(predates this session's own scripts, which are clean; a `sed`/shell
heredoc encoding mishap from earlier in this branch's history is the likely
culprit, consistent with the "prefer Edit over Bash for mutations" lesson
already on file). Real, not cosmetic: naively deleting the NUL bytes
(a first attempt, caught before committing) would have silently broken
`globToRegExp`'s `**`-placeholder swap into a no-op and an empty-regex
match-everything bug, and broken the link checker's own `file target` key
format. Fixed by replacing NUL with a real space instead of deleting it,
verified byte-for-byte identical to the working version otherwise, and
re-ran both files' test suites plus a live glob-match check before trusting
it. Swept the rest of `.claude/`, `scripts/`, `apps/*/scripts`,
`apps/*/eslint-rules`, and `docs/` for the same corruption; nothing else
affected.

## 2026-08-27 — pre-rebase check against 19 more commits landed on main

**Caught by:** other (manual pre-rebase review of main's new commits).

Same policy as the previous pre-rebase check: read every commit main
gained since the last rebase point, checked each against this branch's
docs, left code untouched either way.

**Fixed on the spot:** `astro-queen.md`'s feature table was missing a real
new admin capability, authorization resource inventory and guarded reset
(`internal/authorizationadmin`, `admingrpc/authorization_admin.go`, a new
River worker in the maintenance queue). Added the row, plus a note that
it's environment-gated via `FGA_AUTHORIZATION_RESET_ENABLED` and, as of
this writing, on in preview but off in prod — the reset button exists in
the UI everywhere, the RPC itself refuses in production.

**Checked, no fix needed:** the new `authz.AuthorizationResourceCatalog`
interface backing that feature is purely additive (list/delete for admin
cleanup, not part of request-time authorization) and doesn't contradict
anything `fine-grained-access-control.md` claims about the normal
permission-check path; covered sufficiently by the astro-queen.md addition
and its cross-reference to the implementing package. Seven `deploy`-prefixed
commits refining `ClusterPicker.tsx` (dark-mode tint, badge contrast,
exposing a read-only region as a radio) touch only that one existing
frontend component, no backend code, and don't contradict
`cluster-configuration.md`'s claim that no backend code path enforces a
region constraint (that claim is specifically about the backend; this is
frontend polish on an existing picker).

**Real bug found post-rebase, in this session's own tooling:** the rebase
itself was the test case that caught it. `check-doc-freshness.mjs` used git
`%cd` (committer date) to find each area's most recent commit; a rebase
bumps every replayed commit's committer date to the rebase time regardless
of when the change was actually authored, so it flagged four docs as stale
purely because this branch had just been rebased, not because any of their
code genuinely changed after their Last-verified date. Confirmed by
comparing `%ad` (author date, 2026-08-26) against `%cd` (2026-08-27, the
rebase's own run) for two of the four. Fixed to use `%ad`; re-ran and all
four false positives cleared. `astro-queen.md`'s stamp was bumped to
2026-08-27 anyway, on its own merits, since its content really was
re-verified and edited today against today's new admin-reset commits.

## 2026-08-27 — a real merge conflict, and git silently resolved it wrong

**Caught by:** other (manual post-rebase file-list diff against main).

An earlier pass on this branch (`b199552d2`, "Consolidate org/FGA docs")
judged `docs/01-spec/private-by-default-fgac-rollout.md` a "wholly unbuilt
proposal" and moved it to `docs/06-plan/`. That judgment is now stale: main
has been actively developing the same spec in place at `01-spec/` since
(a full resource inventory, a four-rung role ladder, account permission
tables — real, substantial, ongoing work by another engineer, not a stale
draft). Rebasing hit a real delete-vs-modify conflict here, which is what
GitHub's "branch has conflicts" surfaced, but `git rebase --continue`
resolved it *silently*, with no prompt, by keeping this branch's deletion —
discarding main's active work entirely rather than flagging it. Caught only
by manually diffing the file list against main's newest commit and noticing
it was missing, not by any error message.

Resolved by restoring `01-spec/private-by-default-fgac-rollout.md` from
main's current content, deleting this branch's now-stale `06-plan/` copy,
and repointing `deployment-fgac-rollout.md`'s cross-link back to `01-spec/`.
The earlier "unbuilt, belongs in 06-plan" judgment no longer holds given how
much has landed since; deferring to where the actual work is happening is
the right call, not re-asserting an outdated filing decision against an
active author. Added the file's own new forward-reference to a not-yet-
written `access-audiences-spec.md` to `check-doc-links.mjs`'s known
exceptions, same as `machine-credentials-spec.md`'s identical reference.

**Process note:** a rebase can silently pick the wrong side of a real
conflict with no error at all, not just misapply an already-clean patch
(the committer-date and NUL-byte issues found earlier this branch). After
any rebase that reports even one conflict, diff the full file list against
the target branch's newest commits and check for anything present upstream
but missing locally, rather than trusting a "Successfully rebased" message
on its own.

## 2026-08-27 — ratchets nudge on improvement, staff-engineer feedback

**Caught by:** other (staff engineer feedback, relayed directly, not a PR
comment).

Two pieces of feedback, both applied. First, confirmed the existing split
is already right: the three ESLint ratchets and the stamp-presence check
stay blocking PR gates; only `check-doc-freshness.mjs` (cumulative drift,
not attributable to one PR) is the non-blocking weekly report. No change
needed there.

Second, considered and partly built: should a ratchet auto-tighten when the
real count drops below baseline, so improvements can't be forgotten?
Rejected two versions before landing on one. Auto-committing a lowered
baseline from CI was rejected: CI runners are ephemeral, making it stick
needs a bot commit step (git identity, push permissions, fork-PR token
handling, risk of the commit re-triggering CI), real infrastructure for a
small win. Hard-failing on any mismatch (not just growth) was rejected: the
count is a repo-wide snapshot, not scoped to the PR's own diff, so an
unrelated PR shifting the count would fail every subsequent PR until
someone noticed and bumped the constant, exactly the "innocent PR blocked
by someone else's change" failure mode this system has tried to avoid
elsewhere. Landed on: when the count improves, don't fail, print an
unmissable reminder to lower the baseline in the same PR. Never blocks, but
makes forgetting a lot harder. All three ratchet scripts updated;
`apps/astro-client/CLAUDE.md`'s enforcement notes mention it. A stronger,
per-PR-diff-aware version (hard-block only the PR that caused the
improvement, comparing against the base branch rather than an absolute
number) would close the gap entirely without the bystander risk, but needs
real engineering to diff reliably against a base ref in CI — noted as a
possible future upgrade, not built now.

## 2026-08-27 — eval dataset spec review comments (jessyec-s)

**Caught by:** PR review.

Two PR review comments on the eval dataset specs, from the domain owner.

First, on `docs/01-spec/eval-dataset-v2-spec.md`: the "Partially stale"
banner undersold it. Per the reviewer, this is an old implementation
record, not a spec still worth reading mechanic-by-mechanic, and
`eval-dataset-evaluation-spec.md` is the current design. Reworded the
banner to say "Out of date" plainly and point at the current spec first,
while keeping the one fact worth preserving: the judgment review flow
(`good`/`bad`/`i don't know`, the queue ordering) this doc describes is
still the live write path per `traces-to-eval-dataset.md`, even though the
spec itself is superseded. Also updated `docs/README.md`'s Evals row Notes
to match: `eval-dataset-v2-spec.md` is now described as an older
implementation record, not a peer "similarly annotated" doc.

Second, on `docs/01-spec/eval-dataset-evaluation-spec.md`, the reviewer
noted the per-agent custom evaluation set (`agent_evaluations`,
`EVALUATION.yaml`, publishing) is in active development, not shipped yet.
No doc change made: the doc already states plainly that this doesn't exist
in code today, which is still accurate. Adding a transient "coming soon"
note would itself go stale the moment it ships and nothing would prompt
removing it — the doc's job is to describe what's true now, not to track
in-flight work.

## 2026-08-27 — rule/folder-table contradiction, missing sibling banner

**Caught by:** PR review.

Two more review comments, both verified before acting on them.

First: `docs/README.md`'s "The rule" said a `00/01/03/04/05/09` doc
"describes the system as it is now" and "gets fixed as part of the same
change" when code disagrees, while the folder table right above it says the
opposite for `01-spec` ("can drift... treat it as intent") and `06-plan`
(banner, don't rewrite). Practice all session already followed the folder
table, not the literal rule text. Narrowed "The rule" to the three folder
types where "fixed in place, describes it now" is actually true
(`03-architecture`, `04-guides`, `09-reference`), and added a paragraph for
`00-RFC`, `01-spec`, and point-in-time `05-implementation`: fixing one of
these means bannering and pointing at what's current, not rewriting the
design record, matching what every superseded spec in the area map already
does.

Second: `docs/01-spec/eval-dataset-v2-judgment-reasons-spec.md` had no
banner while its siblings did, despite `eval-dataset-evaluation-spec.md`'s
own header declaring it supersedes this doc's human-judgment contract.
Verified against real code before concluding which side was right: the
judgment-criteria feature (`internal/judgmentstore`'s
`SetVerdictAndReasons`, the client's `JudgmentCriteriaPanel`,
`judgment_criteria` actually landing in Langfuse metadata) is genuinely
live today, so this was a real gap in the earlier banner sweep, not a case
of the sibling overclaiming. Added the same "superseded as a design, still
live in product" framing used on `eval-dataset-v2-spec.md`. Also found and
flagged in the same banner, not fixed in the body per the point above: the
doc's own "existing Langfuse metadata" table lists `verdict` and
`confidence` as real fields; grep against `dataset_judgments.go` confirms
neither was ever actually written, an inaccuracy inherited uncorrected from
`eval-dataset-v2-spec.md`.

## 2026-08-27 — first scheduled doc honesty audit

**Caught by:** scheduled audit.

First real run of [`../04-guides/doc-honesty-audit.md`](../04-guides/doc-honesty-audit.md),
run early (same day it was built) given the volume of new docs/tooling from
today's other work. Four adversarial subagents covered: the honesty-check
system and ratchets built today, the eval-dataset spec cluster, a sample of
6 `03-architecture` docs, and a spot-check of 8 area-map Verify commands
plus `docs/README.md`'s overall internal consistency. Every finding was
independently re-verified against real code/tests before acting, per the
runbook; nothing was fixed on an audit agent's word alone.

**Fixed on the spot:**

- `agents.md`'s Documentation section still told readers to fix a wrong
  `01-spec` doc "as part of the same change," the exact conflation
  `docs/README.md`'s "The rule" was rewritten today to stop making. Missed
  because that day's rule-rewrite commit only touched the writing-style and
  delegation-note paragraphs, not this one. Narrowed it to match.
- Three same-day drift-log entries were missing the `Caught by:` tag the
  file's own header now requires; backfilled them and tightened the
  header's cutoff wording, which had an ambiguous same-day boundary.
- `docs/README.md`'s "The rule" claimed the Status/Last-verified stamp must
  be a doc's literal first two lines; 28 of 30 real `03-architecture` docs
  put the H1 title first, and `check-doc-stamps.mjs`'s own header comment
  already documents that only presence is checked, not position. Reworded
  to match reality.
- `docs/README.md` said "two CI checks" back up the docs system; there are
  three, `check-doc-links.mjs` runs per-PR too, in the same workflow as
  `check-doc-stamps.mjs`. Reworded to three and named all of them.
- Quota row's Verify command applied a `-run QuotaIncrease` filter to
  `internal/quota` too, silently matching zero of its 11 real tests
  (`TestCheck_*`, `TestBlueprintExists`, `TestWrapRegister_*`,
  `TestLimitResponse_*`) while exiting 0 — exactly the silent-pass failure
  mode this table's own intro warns against, in the table itself. Split the
  command so `internal/quota` runs unfiltered.
- `agents.md` used "specific factual claim" where `docs/README.md` and
  `docs/04-guides/doc-honesty-audit.md` both say "load-bearing claim" for
  the same concept; standardized on "load-bearing."
- "The doc-freshness hook" was two docs' name for `.claude/hooks/docs-map-check.mjs`,
  which has nothing to do with `scripts/check-doc-freshness.mjs` (a
  same-repo, differently-behaved script with a confusingly similar name).
  Renamed to "the docs-map hook" in both `docs/README.md` and `agents.md`.
- `eval-dataset-evaluation-spec.md`: "As shipped" schema block still said
  `added_by_user_id`; the real, currently-used column is
  `verified_by_user_id` (`internal/evalitemstore`). Fixed directly, since
  this section already presents itself as corrected-to-reality, not
  original design intent.
- `eval-dataset-evaluation-spec.md`'s banner said "the evaluator engine
  shipped," the only place in the whole cluster using "engine" instead of
  "flow." Fixed to match.
- `eval-dataset-v2-spec.md`'s banner: extended rather than rewrote the
  body, per the 01-spec banner-not-rewrite rule. Now also notes the Server
  API table's endpoints were never real (`/eval/queue`, `/eval/judge` vs.
  actual `/dataset/review-queue`, `/dataset/judgments`), "handlers live in
  `handlers/dataset.go`" undersells it (real handlers span 6+ files),
  `sourceTraceID` should be `sourceTraceId`, and made the supersession
  claim with `eval-dataset-evaluation-spec.md` explicitly bidirectional.
  Also removed an em dash from the banner's own prior wording, a rule the
  writing-style section says applies to specs.
- `docs/README.md`'s Evals row glob was `dataset_*.go`, which doesn't match
  `handlers/dataset.go` itself (real, actively-used shared helpers).
  Widened to `dataset*.go`; checked no unrelated file matches.
- `blueprint-lifecycle.md` claimed `github_builds`/`github_connections`
  have FK `ON UPDATE CASCADE` so `Transfer` re-keys them automatically. No
  such FK exists (checked `sql/astro-server/schema.sql`), and `Transfer`
  doesn't touch either table at all. See "Logged, not fixed" below, this
  is a real code gap, not just a doc error.
- `blueprint-lifecycle.md`'s publisher-list description said "the actor who
  last called `agent.register`," singular/most-recent. The real query
  groups by every distinct actor and orders by first action
  (`MIN(created_at)`), plural/earliest-first — backwards on both axes.
- `blueprint-lifecycle.md` claimed both `BatchLatestBuildIDs` and
  `BatchLatestBuildInfo` use a `ROW_NUMBER()` window function; only the IDs
  variant does, `BatchLatestBuildInfo` uses `JOIN LATERAL ... LIMIT 1`.
- `traces-to-eval-dataset.md` had a real self-contradiction: its trace-field
  table and "Build the review queue" section described the live queue as
  prioritizing by an inferred reaction signal, while a different section
  100+ lines down correctly calls that same reaction-signal logic
  "built-but-inert." Verified against the real handler
  (`GetDatasetReviewQueue`): the only real ordering is hardcoded
  `timestamp.desc`, no reaction/sentiment logic runs in the live path at
  all. Fixed both sections to agree.
- `authentication-flow.md` quoted a fabricated frontend snippet
  (`refreshIfNeeded()`, `handleAuthError()`, neither exists anywhere in
  `apps/astro-client/src`). Replaced with the real `AuthProvider.tsx`
  logic (deduped `checkAuth`, hydration skip, visibility + focus
  listeners). Also added the machine-token (M2M) authentication branch
  entirely missing from the Bearer-token flow and diagram, added the
  missing `AUTH_JWT_ISSUER` env var to the Configuration table, and fixed
  the `Session` struct diagram to show its real fields
  (`WorkOSMembershipID`, `Permissions`, `CreatedAt`) instead of silently
  omitting them with no truncation marker.
- `auditlog.md`'s handler list omitted `handlers/apps.go` entirely, which
  logs 5 real audit actions (M2M app create/update-scopes/delete, secret
  create/delete) through the same path the doc otherwise documents.
- `agent-runtime-panels.md` claimed all 4 of `AgentIdentity.tsx`'s overflow
  actions are "each behind its own dialog"; "View blueprint" is a plain
  `Link` navigation, not a dialog.
- `docs/README.md`'s Agent-runtime-panels row described the area's ~43%
  test coverage as "concentrated in orchestration/visual code" — backwards;
  the *covered* slice is the pure logic/presentational code, the
  *untested* remainder is what's concentrated in orchestration/visual
  code, per the architecture doc's own later section.
- Bumped `Last verified` on every `03-architecture` doc actually
  re-verified above (`authentication-flow.md`, `traces-to-eval-dataset.md`,
  `blueprint-lifecycle.md`, `auditlog.md`, `quota.md`, `agent-runtime-panels.md`)
  to 2026-08-27, per the "update it any time you verify the doc" rule.

**Logged, not fixed (real code issue):**

`agentindex.Index.Transfer` re-keys `agents`, bumps `agent_versions.updated_at`,
and repoints `deployments.source_account_id`, but never touches
`github_builds`/`github_connections`, and no FK cascades them either. After
a real account-to-account agent transfer, both tables keep pointing at the
source account, which silently breaks `accountBlueprintLatestCommitJoin`'s
`gb.account_id = a.account_id` join, losing GitHub commit metadata for the
transferred agent. Fixing this needs a product decision (should a GitHub
connection even follow an agent across accounts, or should it be treated as
severed on transfer, prompting reconnection) that's out of scope for a docs
pass. The doc now describes this gap accurately instead of asserting a
cascade that doesn't exist.

**Logged, not fixed (minor doc scoping):**

Auth row's Verify command (`go test ./internal/auth/... ./internal/middleware/...`)
also runs unrelated `internal/middleware` tests owned by the Billing and
Org/FGA rows (`entitlement_test.go`, `deployment_authz.go`'s tests) — it
still passes and is fast, just scoped wider than the row it's attached to.
Not fixed because `internal/middleware` is a shared package with no clean
per-row subdivision; a `-run` filter narrow enough to exclude those would
be brittle against new middleware tests. Noted here rather than in the
table itself, since the table intro's warning is about *silent* failures,
and this one isn't silent, just imprecise.
