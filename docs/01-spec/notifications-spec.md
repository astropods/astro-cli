# User Alerts &amp; Notifications — Novu Integration

Builds on `modules/astro-infra/docs/architecture/13-novu-notifications.md`. References `docs/01-spec/metronome-billing-spec.md` (billing signals), `docs/04-guides/river-queue.md` (job transport).

## Summary

Astro emits no user-facing alerts. Novu is deployed and functional (self-hosted CE `3.18.0`) with SES email transport and authored workflows — it can send. What is missing is the backend integration: astro-server has no Novu client, no subscriber management, and no trigger calls. The backend already detects most alert-worthy events (deploy failures, pod crashes, billing signals, build failures, team changes) and persists them, but nothing reaches Novu.

This spec defines the path from a backend state change to a delivered alert: a typed emit seam, a River-backed delivery worker, recipient resolution from existing member-email state, and a Novu client that triggers Novu's email + in-app workflows.

Two producers feed one delivery path. Discrete events (build failed, billing signals, team/account changes) emit at existing choke points. Running-agent health and resource budget (OOM, crash loop, memory/compute over budget) run through a dedicated observation evaluator with thresholds and firing state, so only real, sustained problems mail. Deploy lifecycle deliberately emits nothing — it stays in the UI.

---

## Goals

- One typed `Emit` seam; delivery, retry, and provider details sit behind it.
- Durable, retryable, deduplicated delivery — an emit that commits delivers once per recipient across restarts and provider retries.
- Email + in-app feed, with a per-user settings page: platform-default per notification, user-overridable per channel; billing/security locked on.
- Recipients resolved from the member-email mirror; org-role audiences (`managers`) query WorkOS at delivery since roles aren't mirrored locally.
- Every source cataloged with trigger point, audience, and channel.
- Email only for genuine failures/breaches; lifecycle chatter stays in-app.
- Resource-budget and health conditions (memory/compute over budget, OOM, error spikes) run through a dedicated observation evaluator with thresholds and firing/resolve state, not per-event mail.

## Non-goals

- No anomaly/ML/SLO engine in v1. The observation evaluator uses static per-condition thresholds + sustained windows; adaptive baselines are later.
- No changes to how Novu workflows are authored. They live in Novu; the backend triggers them by identifier. New event `Type`s need a workflow authored in Novu first.
- No SMS/push/Slack channels in v1.
- No sends on OSS/self-hosted when Novu is unconfigured — no-op provider, per the billing `noop` pattern.
- No replacing WorkOS transactional mail (login links, invites).

---

## Current state

**Novu (deployed, can send).** `novu-api` (`:3000`), `novu-worker`, `novu-ws` (`:3002`, in-app feed), `novu-dashboard`, Mongo, Redis. Secrets in the `novu-app` K8s Secret. SES email transport is configured and workflows are authored. Public API `https://api.novu.astroids.ai` sits behind the admin WAF IP-allowlist — reachable by admins, not yet by the app cluster.

**Backend (no send path).** No SMTP/SES client, no `NOVU_*` config, no Novu SDK. Recipient email available via the `memberemails` mirror (refreshed at login + daily `MemberEmailReconcileArgs` cron) and `account.GetOwnerEmail`.

**Transport.** River (`internal/riverqueue`, Postgres-backed): typed args, retry with backoff, unique/dedupe, periodic jobs, per-domain queues (`deploy`, `build`, `billing`, `metering`, `insights`, `maintenance`). No `notifications` queue.

**Existing ledgers.** `deployment_events` (every transition, one tx via `deploymentstore.updateStatusTx`), `audit_logs` (user mutations), billing status machine (`billing.ApplySignal`).

---

## Architecture

```
discrete events                          observation conditions
handler / controller / webhook / cron    ObservationSweep (periodic)
  └─ notify.Emit(ctx, tx, Event)           └─ evaluate budgets vs signals
                                              └─ firing-state edge → notify.Emit
        │                                            │
        └────────────► "notifications" queue ◄───────┘
                          └─ NotifyWorker.Work()
                               ├─ resolve recipients (audience → user ids → emails)
                               ├─ throttle/digest
                               └─ Novu POST /v1/events/trigger (workflowId, to:[{subscriberId,email,name}], payload, transactionId)
                                     └─ subscriber upserted inline; Novu applies per-user preferences → email (SES) + in-app (ws)
```

Two producers, one delivery path. Discrete events emit at the writer; observation conditions emit only on a firing/resolve edge. Both land on the `notifications` queue and share recipient resolution, preferences, dedupe, and the Novu trigger.

**Emit seam.** `notify.Emit(ctx, tx, Event)` inserts one `NotifyArgs` River job. Sources already inside a DB tx (audit write, billing status write) pass it so job and state change commit atomically. Cron/webhook sources insert non-transactionally.

**Delivery worker.** `NotifyWorker` on a dedicated `notifications` queue (isolated from deploy/billing backlogs). Owns recipient resolution, throttle/digest, and the trigger call. Per-user channel preferences are applied by Novu at delivery, not here (§Notification preferences). Novu 5xx/network errors retry with backoff; `transactionId` makes retries idempotent at Novu.

**Subscribers.** No standalone upsert. The trigger's `to` array (`subscriberId` = WorkOS user id, `email`, `name`) upserts the subscriber inline on every send, so a first-time recipient is created by the same call that notifies them. An explicit `POST /v1/subscribers` is used only to pre-seed data ahead of a trigger — notification preferences at signup, or a one-time bulk backfill of existing members — not on the hot path.

**No new event bus.** Emit is an explicit call at the writer, not a ledger tail. Continuous conditions are handled by the observation evaluator (§Observation alerts), not by tapping every metric sample.

**OSS/no-op.** `NOVU_API_URL` unset ⇒ provider logs and drops.

---

## Event contract

| Field | Meaning |
| --- | --- |
| `Type` | `<domain>.<event>` identifier; maps 1:1 to a Novu workflow id (the preference + dedupe unit). |
| `AccountID` | Owning account; drives audience resolution. |
| `Audience` | Recipient policy; resolved at delivery. |
| `ActorUserID` | Causing user, if any; excludes self, drives copy. |
| `SubjectUserID` | User the event is about (e.g. role changed). |
| `EntityID` | Deployment / build / invoice id concerned. |
| `Payload` | Workflow template variables (names, URLs, reasons). |
| `DedupeKey` | Idempotency key → Novu `transactionId`. Default `Type + EntityID + status`. |

`Type` is the contract shared by emit, the Novu workflow id, and the preference unit — stable once shipped. `DedupeKey` collapses provider redeliveries, River retries, and repeated sweeps: build failure = `build.failed` + build id; billing = provider event id; observation = condition + deployment id + firing-since.

---

## Recipient resolution

Audience is a policy resolved in the worker against org membership + the member-email mirror. Novu `subscriberId` = WorkOS user id; email from the mirror. The resolved `{subscriberId, email, name}` rides in the trigger's `to` array, which upserts the subscriber — no separate creation step.

| Policy | Resolves to | Used by |
| --- | --- | --- |
| `owner` | Account owner (`GetFirstMemberUserID`) | — |
| `managers` | Org managers — owner + admins (`org:manage`) | billing, security, account transfer |
| `members` | All account members | deploy/build outcomes |
| `actor` | Triggering user | confirmations |
| `subject` | Affected user | team membership changes |

`managers` is resolved by **querying WorkOS** org memberships (roles aren't mirrored locally — `account_members` has no role column) and filtering to `owner`/`admin` role slugs. It falls back to the `owner` when there is no org (personal account), no configured org client, or the WorkOS lookup fails — so a critical alert always reaches at least the owner. `actor` is excluded from `members` broadcasts so a user is not alerted about their own action.

---

## Notification preferences

**Source of truth: Novu, per-user.** Novu already models exactly this — each workflow carries default channel enablement plus a `critical` flag, and each subscriber (`subscriberId` = WorkOS user id) holds per-workflow, per-channel overrides. We use it directly: no catalog in code, no `notification_preferences` table, no worker-side channel computation. Novu enforces preferences at delivery, dropping the channels a subscriber disabled. This is why `Type` maps **1:1 to a Novu workflow id** — the workflow is the preference unit (§Novu workflow configuration).

Preferences are **per-user**: every member configures their own from the settings page. There is no account-wide toggle — that would require a shared account subscriber (one inbox feed for the whole team) or a custom layer, both rejected. Account-level admin defaults are a possible later layer.

**Defaults + `critical` live on the workflow in Novu** (per-workflow channel enablement + the `critical` flag), authored once per §Novu workflow configuration. `critical` workflows (billing failures, security) are locked-on in the preferences UI and ignore subscriber overrides — a user silencing "payment failed" or "new API key" is a support/security hazard.

**Settings page (astro-client).** A Notifications section renders the subscriber's Novu preferences: `GET /accounts/:id/notification-preferences` proxies Novu's `GET /v1/subscribers/:subscriberId/preferences` (workflow list + effective per-channel state — subscriber-independent, so the full list always shows even before a first send). Per-channel **checkboxes**; `critical` rows render disabled-on. `PATCH` proxies the Novu preference update so the Novu key never reaches the browser. A "Send test" button delivers `system.test` end to end. Per-deployment observation threshold overrides (§Observation alerts) live on the deployment surface, not here — this page governs *whether and how*, not *when a condition fires*.

---

## Alert source catalog

Two paths. **Discrete events** (this section) fire on a single state change and emit directly. **Observation conditions** (next section) are continuous — resource budget, health, error rate — and go through the observation evaluator, never the direct emit path.

Deploy lifecycle emits nothing. Deploy-time failures (failed schedule, image pull, stuck rollout) stay in-app on the existing stuck-deploy banner — the user is watching the deploy when they happen. Alerting is reserved for *running* agents silently degrading, which is the observation path (OOM, crash loop, over budget), not a lifecycle event.

**Ready** = detected/persisted, emit only. **New** = detection must be built.

| Type | Trigger point | Audience | Channels | Detection |
| --- | --- | --- | --- | --- |
| `system.test` | "Send test" on the settings page → `POST /accounts/:id/notification-preferences/test` | actor | email + in-app | Ready |
| `build.failed` | `riverqueue/github_build.go` fail/failOrRetry | members | email + in-app | Ready |
| `billing.payment_failed` | Stripe `invoice.payment_failed` | managers | email + in-app | Ready |
| `billing.action_required` | Stripe `invoice.payment_action_required` (3DS `HostedInvoiceURL`) | managers | email + in-app | Ready |
| `billing.uncollectible` | Stripe `invoice.marked_uncollectible` | owner | email | Ready |
| `billing.spend_threshold` | Metronome `alerts.spend_threshold_reached` | managers | email + in-app | Ready |
| `billing.dunning_suspended` | `billing_dunning.go` grace-expired `past_due`→`suspended` | managers | email + in-app | Ready |
| `billing.recovered` | Stripe `invoice.paid` | managers | in-app | Ready |
| `billing.card_expiring` | — | owner | email | New (poll Stripe PM expiry) |
| `team.member_added` / `role_changed` / `removed` | `handlers/org.go` | subject + admins | email + in-app | Ready |
| `team.invitation_created` | `handlers/org.go` CreateInvitations | invitee | email | Ready (else WorkOS mail) |
| `account.ownership_transferred` | `handlers/transfer.go` (agent transfer) | managers (of target) | email + in-app | Ready |
| `account.deletion_scheduled` | account soft-delete; purge cron nears retention | owner + admins | email | Ready |
| `quota.reached` | `internal/quota/quota.go` Check hits limit | admins | in-app | Ready (hook) |
| `quota.increase_decided` | admin approve/deny on `quota_increase_requests` | requester | email + in-app | Ready |
| `security.key_changed` | `otel_ingest_tokens.go` create/revoke | managers | email + in-app | Ready (wired) |

- `system.test` lets a user verify their channels are working from the settings page. It sends to the actor on both channels and ignores preferences (the point is to test delivery, not honor an opt-out). It is the end-to-end canary in PR 1.
- No deploy-lifecycle event emits — deploy status (including `failed`) is surfaced in-app by existing UI, not mailed. Runtime degradation is the observation path.
- Build success is omitted; only `build.failed` mails.
- Key-lifecycle events are not in the audit vocabulary; auditing them is a prerequisite in that slice.

---

## Observation alerts (deep-insight path)

Resource-budget and health conditions are continuous, not discrete events. Emitting one alert per K8s signal (a restart, a scrape sample) would be exactly the mail spam we are avoiding. They need thresholds, a sustained window, and firing/resolve state so a flapping deployment yields one alert, not a stream. This is a separate subsystem that feeds the same emit seam only on a state edge.

**Evaluator (implemented — `internal/observation`).** A periodic River job, `ObservationSweep` (5-min interval), runs each condition's query on the engine named by its `Engine` field — `promql` (VictoriaMetrics via the existing `promquery` client) today; `langfuse` (error rate / latency) is a registered-later engine behind the same `Querier` interface. Each condition query returns the currently-breaching `namespace`s; a namespace resolves to a deployment (`GetLatestDeploymentByNamespace`) → account + agent. The evaluator diffs breaching-vs-tracked state and emits only on the firing edge.

It is a **lightweight in-process evaluator**, not an embed of `prometheus/prometheus/rules`: that package pulls ~320 modules (the full Azure/AWS/GCP/DO service-discovery tree) into a server that has zero Prometheus deps today — disproportionate for a handful of threshold rules. The `for` window, edge-only firing, dedup, and resolve are ~150 lines over `promquery` + a Postgres state table.

**Inputs.**

| Signal | Source |
| --- | --- |
| Memory / CPU utilization, restart counts | VictoriaMetrics / Prometheus via `promquery` |
| OOMKilled, CrashLoopBackOff, Unschedulable | `deploycontroller` K8s informer (`pods.go` classifications) surfaced into the runtime snapshot |
| Error rate, p95 latency | Langfuse via the existing insights fan-out |
| Disk usage | messaging sidecar `files/usage` |

**Budgets.** Per-deployment resource limits come from the deploy spec sizing (memory/CPU requests+limits, response timeout); platform defaults fill the rest. Thresholds are overridable per deployment so a large agent is not perpetually "over budget".

**Evaluation.** Each condition = threshold + sustained window (e.g. memory > 90% of limit for 10 min). Transient spikes are ignored. Recovery uses a lower clear threshold (hysteresis) so a value hovering at the line does not flap.

**Firing state.** `deployment_alert_state` (deployment_id, workload, condition, active_since, notified) — one row per breaching (deployment, workload, condition); the evaluator resolves each breaching pod to a deployment + workload (the pod's `app.kubernetes.io/component`) before tracking, so the UI can attribute an alert and a redeploy is a distinct episode. `active_since` drives the `for` sustained window; `notified` marks the workload's firing edge as handled. Rows are deleted on resolve (v1 does a **silent resolve**). Per-workload state does **not** multiply mail: the evaluator emits only when a workload fires and no other workload of the same deployment has already notified this condition — one notification per (deployment, condition) episode, its `reason` naming the workload. A per-episode `DedupeKey` (`name:deployment:workload:active_since`) means a re-breach after a resolve is a distinct alert at Novu, not a suppressed duplicate. This is the choke point that keeps observation alerts rare.

**Two workflows by severity.** Conditions do **not** map 1:1 to Novu workflows. Every condition carries a `Severity` (`critical` or `warning`) and collapses to one of two workflows: `observation.critical` ("Agent failing" — crash loop, OOM, unschedulable) or `observation.warning` ("Agent degraded" — restarts, memory/compute pressure, error spikes). The specific condition rides in the payload `reason` (e.g. "Out of memory"), so two shared templates render every condition. This gives the user **two preference toggles**, not one per condition, and adding a condition needs no new workflow. Firing state stays keyed on the granular condition name so two same-severity conditions on one deployment don't collide.

**Emit.** On an edge, the sweep calls `notify.Observation(severity→type, account, agent, deploymentID, title)`, reusing delivery, preferences, and dedupe. `DedupeKey` = condition name + deployment id + firing-since, so a retry/re-run of the sweep cannot double-send.

**Conditions.** All shipped conditions are VM/Prom (`promql` engine); each maps to a severity workflow.

| Condition | Fires when | Severity → workflow |
| --- | --- | --- |
| `crash_loop` | CrashLoopBackOff sustained 5m | critical → `observation.critical` |
| `oom_killed` | container's last termination was OOMKilled | critical → `observation.critical` |
| `unschedulable` | pods unschedulable past 10m grace | critical → `observation.critical` |
| `restart_storm` | restarts over N in a 5m window | warning → `observation.warning` |
| `memory_over_budget` | memory util over threshold, sustained (OOM precursor) | warning → `observation.warning` |
| `compute_over_budget` | CPU at limit / throttled, sustained | warning → `observation.warning` |
| `cpu_over_provisioned` | CPU usage far below its request, sustained (waste) | warning → `observation.warning` |
| `memory_over_provisioned` | memory usage far below its request, sustained (waste) | warning → `observation.warning` |
| `error_spike` *(Langfuse, unshipped)* | error rate over threshold, sustained | warning → `observation.warning` |
| `latency_high` *(Langfuse, unshipped)* | p95 over threshold, sustained | warning → `observation.warning` |
| `storage_near_full` *(sidecar, unshipped)* | disk > 85% / 95% | warning → `observation.warning` |

Audience `members`; per-user opt-out applies. Both observation workflows deliver **in-app by default with email off** (opt-in per user), since these can be higher-volume than discrete events. Resolve notifications are in-app only. The eight VM/Prom conditions are implemented; the Langfuse and sidecar rows await their engine/source.

---

## Novu workflow configuration

Novu owns the catalog, defaults, and `critical` flags, so there is **one workflow per `Type`**. Each is authored once in the Novu dashboard; the workflow **identifier equals the `Type`** (so the backend triggers `build.failed`, `billing.payment_failed`, … directly — no `WorkflowID` override except local dev). Adding a notification means authoring a workflow, nothing in code. This section is the exact content to enter.

### Per-workflow settings (in Novu)

For every workflow set, in the dashboard:

- **Identifier / trigger name:** the `Type` (e.g. `build.failed`).
- **Name:** the display string shown in the preferences page (below).
- **Group / tag:** the category (below) — Novu groups the preferences UI by tag.
- **Channel steps:** an **In-App** step and an **Email** step (order: In-App, then Email).
- **Default channels:** enable the channels marked in the table; disable the others in the workflow's channel defaults.
- **Critical:** toggle **on** for the rows marked `critical` — those become locked-on in the preferences UI and ignore subscriber overrides.

Preferences and channel gating are Novu's job — the backend no longer sends `send_email` / `send_in_app` flags, and there is no per-workflow step condition. The payload carries **only data** (names, amounts, urls); all copy is authored in the workflow templates below.

### Payload contract (data only)

The backend pushes **structured data only** — never prose. All message wording (in-app subject/body, email subject/body) is authored in the Novu workflow templates, which compose the message from the payload properties via `{{payload.<key>}}`. Each workflow declares a **payload JSON schema** (properties typed as strings) so the dashboard editor knows the available variables. Workflows are authored and maintained directly in Novu (v2 API / dashboard); `notify/payload.go` is the single source of truth for the property set and the emit-side values, and `notify.PayloadProperties(type)` is the contract each workflow's schema must match.

`ctaUrl` is the one derived property: the backend sends a relative app path that the Deliverer absolutizes against the app base URL (`FrontendURL`), or an absolute URL for external links (e.g. Stripe's 3DS page). Everything else is raw data. Channel gating and preferences are Novu's — no `send_*` conditions in templates.

Authoring guidance (per workflow, in the dashboard): compose the in-app subject/body and email subject/body from the properties; point the in-app row redirect (and any button) and the email CTA button at `{{payload.ctaUrl}}`.

### Per-type payload properties

Each workflow receives exactly these properties (this is the schema uploaded to Novu):

**Deployments**

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `build.failed` | no | email + in-app | `agent`, `reason`, `ctaUrl` |

**Billing** — owner-addressed

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `billing.payment_failed` | yes | email + in-app | `account`, `ctaUrl` |
| `billing.action_required` | yes | email + in-app | `account`, `ctaUrl` (Stripe 3DS link) |
| `billing.spend_threshold` | no | email + in-app | `account`, `ctaUrl` |
| `billing.dunning_suspended` | yes | email + in-app | `account`, `ctaUrl` |
| `billing.recovered` | no | in-app | `account` |

**Team / account**

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `team.member_changed` | no | in-app | `account`, `role`, `action` (`added`\|`role_changed`\|`removed`), `ctaUrl` |
| `account.ownership_transferred` | no | email + in-app | `account`, `agent`, `ctaUrl` |

The member-change template branches on `action` (a Novu conditional block) for the wording.

**Security** — owner-addressed

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `security.key_changed` | yes | email + in-app | `keyKind`, `keyName`, `action` (`created`\|`revoked`), `ctaUrl` |

**Observability** (not yet emitted) — each adds a leading digest step (window 15 min, key `deployment_id`)

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `observation.critical` | no | in-app (email opt-in) | `agent`, `reason`, `ctaUrl` |
| `observation.warning` | no | in-app (email opt-in) | `agent`, `reason`, `ctaUrl` |

**System**

| Identifier | Critical | Default channels | Payload properties |
| --- | --- | --- | --- |
| `system.test` | yes | email + in-app | `account` |

`system.test` is `critical` so it stays off the configurable preferences list (delivery canary, not a real alert).

---

## Delivery semantics

**Idempotency.** `DedupeKey`→`transactionId`; Novu rejects duplicates. River `UniqueOpts.ByArgs` collapses same-key emits before Novu.

**Preferences.** See §Notification preferences — per-user, per-channel, held in Novu and applied by Novu at delivery.

**Throttle / digest.** Observation conditions are rate-limited primarily by their firing-state machine (edge-only, no re-emit while firing). Where a single deployment can trip several conditions at once, a Novu digest step (or worker-side throttle keyed on `subscriber + deployment + window`) rolls them into one notification. Window per-type (e.g. 15 min).

**Failures.** Novu 5xx/network → River retry; exhausted attempts land `discarded` in `river_job`. Missing recipient email drops that recipient, does not fail the job.

---

## Configuration

New `astro-server` env, all optional — unset ⇒ no-op provider:

| Var | Purpose |
| --- | --- |
| `NOVU_API_URL` | Novu API base; in-cluster address, not the WAF-gated public host. |
| `NOVU_SECRET_KEY` | Novu API key; from the `novu-app` Secret. |
| `NOTIFY_DEFAULT_FROM` | Email from-identity (template owns it). |

---

## Infrastructure dependencies

1. **App→Novu path.** Novu runs on `infra-observability-eks`; astro-server runs on the app clusters. `novu-api` is `ClusterIP` (infra-cluster-internal only) and the public `api.novu.astroids.ai` is WAF-allowlisted for admins — neither is reachable from the app cluster. Expose `novu-api` cross-cluster via an internal NLB over the existing infra↔app VPC peering, mirroring `loki.tf`/`tempo.tf`/`prometheus.tf`, and point `NOVU_API_URL` at it. (astro-infra)
2. **Workflow coverage.** One workflow per `Type`, authored in Novu with identifier = `Type` and the copy/defaults/`critical` from §Novu workflow configuration. Novu owns the catalog and preferences; there is no catalog or preference table in code. SES transport already exists. Author the workflows for a slice before wiring its emit source.

(Email transport is already configured in Novu — no infra work needed.)

---

## Other solutions considered

- **Tail `deployment_events` / `audit_logs` via pgnotify.** Rejected as primary: audit_logs omits key-lifecycle and workload events; reconstructing audience/payload from a generic row is lossy. pgnotify stays for its deploy-reconcile nudge.
- **Direct SES from astro-server.** Skips Novu — loses in-app feed, preferences, digest, and the delivery dashboard already deployed and configured.

---

## Rollout

**PR 1 — Foundation + settings page + test send.** The whole config surface and a working end-to-end path, with no product alert sources yet. Includes:
- `internal/notify`: `Event`, `Notifier`, `NotifyArgs` + `NotifyWorker` on a new `notifications` queue; no-op provider for OSS/unconfigured.
- `internal/novu`: client + `events/trigger` (inline `to`, `transactionId`); real provider on the hosted path. Dependency: app→Novu network path.
- Author the `system.test` workflow in Novu (§Novu workflow configuration) and point `NOVU_TEST_WORKFLOW_ID` at it.
- Novu owns catalog + preferences + `critical` — no catalog or `notification_preferences` in code. The worker triggers by `Type` and sends data-only payloads; Novu enforces per-user channel preferences.
- Backend proxy `GET`/`PATCH /accounts/:id/notification-preferences` over Novu's subscriber-preferences API (list is subscriber-independent, so it always renders); `POST …/test`.
- astro-client Notifications settings section: renders the subscriber's Novu workflow preferences grouped by tag, per-channel checkboxes, `critical` locked, and a "Send test" button that delivers `system.test` end to end (email + in-app).

**PR 2 — build.failed.** First product alert. Author the `build.failed` workflow in Novu (§Novu workflow configuration), then wire `Emit` into `github_build.go` with a data-only payload.

**PR 3 — Billing alerts.** Emit from `webhook_jobs.go` and `billing_dunning.go`: `payment_failed`, `action_required`, `spend_threshold`, `dunning_suspended`, `recovered`. Non-disableable billing defaults.

**PR 4 — Observation evaluator (done).** `internal/observation` + `ObservationSweep` periodic job + `deployment_alert_state` table. Engine-routed evaluator (`Engine`→`Querier`) with a `for` window + edge-only firing + per-episode dedup; `promql` engine over `promquery` shipped. Wired conditions (all VM/Prom): `crash_loop` (CrashLoopBackOff), `oom_killed`, `restart_storm`, `unschedulable`, `memory_over_budget`, `compute_over_budget` (CFS throttling), `cpu_over_provisioned`, `memory_over_provisioned` (usage far below request). PromQL exprs are best-effort and need validation against the deployed exporter label set.

**PR 5 — Observation follow-ups.** A `langfuse` engine (implements `Querier`) for `error_spike` / `latency_high`, plus `storage_near_full`, resolve notifications (in-app), and per-deployment threshold overrides.

**PR 6 — Team / account / quota.** Emit alongside audit writes in `org.go`, `transfer.go`, account deletion, `quota_increase` decisions. Add `quota.reached` hook in `quota.Check`.

**PR 7 — Security detection.** Audit + emit key lifecycle. (`new_login` is out of scope — no web device-diff signal exists.)

---

## Open questions

- Team/observation copy **variants**: use Novu conditional blocks keyed on `{{payload.variant}}`, or have the backend pre-render the final `subject`/`body` into the payload (keeps workflow templates trivial)? Leaning pre-render.
- Account-level preference defaults: should admins set org-wide defaults that seed each member's (per-user) preferences? Deferred past v1.
- Default observation thresholds: specify concrete numbers (e.g. memory >90% for 10 min) in the spec, or leave to implementation?

Resolved: digest uses Novu's native digest step (§Novu workflow configuration, observation workflows); in-app feed auth uses a backend-minted HMAC subscriber hash (§infra `13-novu-notifications.md`).
