# Novu Workflows — Payloads & Messages

Reference for every Novu workflow astro-server triggers: the payload it pushes and what the resulting message looks like.

## How it works

- All triggers go through one path: `notify.Deliverer.Deliver` → `novu.Client.Trigger` → `POST /v1/events/trigger`. Source: `apps/astro-server/internal/notify/`.
- The **workflow identifier == the event `Type`** string (e.g. `build.failed`). The only override is local dev (`NOVU_TEST_WORKFLOW_ID`).
- The backend pushes **structured data only** — no prose. All wording is authored in the Novu dashboard templates and composed from payload variables via `{{payload.<key>}}`.
- `ctaUrl` values starting with `/` are relative app paths, absolutized at delivery against `FrontendURL`. Absolute URLs (e.g. Stripe 3DS links) pass through unchanged.
- Idempotency: each trigger carries a `transactionId` (from `DedupeKey`, else `Type:EntityID`) so Novu drops duplicates.

The **Example message** column is a representative rendering composed from the payload variables — the canonical copy lives in the Novu templates, not the repo. Payloads are authoritative (from `internal/notify/payload.go` and `event.go`). `†` marks `critical` workflows (locked-on, ignore user opt-out).

## Workflows

| Workflow (`Type`) | Trigger | Audience | Payload (sample JSON) | Example message |
|---|---|---|---|---|
| `system.test` † | "Send test" on settings page (`handlers/notifications.go`) | actor | `{"account": "acme"}` | **Test notification** — This is a test notification for **{account}**. Your notifications are working. |
| `account.welcome` | New account created (`handlers/accounts.go`) | actor | `{"account": "acme", "ctaUrl": "/agents"}` | **Welcome to Astro AI** — Your account **{account}** is ready. Deploy your first agent to get started. → Open Astro |
| `build.failed` | Agent container build fails (`riverqueue/github_build.go`) | members | `{"agent": "chatbot", "reason": "go build: exit 1", "ctaUrl": "/acme/chatbot"}` | **Build failed: {agent}** — The container build for **{agent}** failed: {reason}. → View agent |
| `billing.payment_failed` † | Stripe `invoice.payment_failed` (`riverqueue/webhook_jobs.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` | **Payment failed** — We couldn't process the payment for **{account}**. Update your payment method to avoid interruption. → Manage billing |
| `billing.action_required` † | Stripe `invoice.payment_action_required` (3DS) | managers | `{"account": "acme", "ctaUrl": "https://invoice.stripe.com/i/acct_.../..."}` | **Action required to complete payment** — The payment for **{account}** needs additional confirmation. → Complete payment |
| `billing.spend_threshold` | Metronome `alerts.spend_threshold_reached` | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` | **Spend threshold reached** — **{account}** has reached its configured spend threshold. → Review usage |
| `billing.dunning_suspended` † | Dunning sweep suspends account (`riverqueue/billing_dunning.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` (`account` may be `""`) | **Account suspended** — **{account}** has been suspended for non-payment. Update your payment method to restore service. → Manage billing |
| `billing.recovered` | Stripe `invoice.paid` | managers | `{"account": "acme"}` | **Payment recovered** — The outstanding balance for **{account}** has been paid. Service is fully restored. |
| `team.member_changed` (`added`) | Member added (`handlers/org.go`) | subject | `{"account": "acme", "role": "admin", "action": "added", "ctaUrl": "/acme"}` | **You were added to {account}** — You've been added to **{account}** as **{role}**. → Open account |
| `team.member_changed` (`role_changed`) | Member role changed (`handlers/org.go`) | subject | `{"account": "acme", "role": "member", "action": "role_changed", "ctaUrl": "/acme"}` | **Your role changed in {account}** — Your role in **{account}** is now **{role}**. → Open account |
| `team.member_changed` (`removed`) | Member removed (`handlers/org.go`) | subject | `{"account": "acme", "action": "removed"}` | **You were removed from {account}** — You no longer have access to **{account}**. |
| `account.ownership_transferred` | Agent ownership transferred (`handlers/transfer.go`) | managers (of target acct) | `{"account": "acme", "agent": "chatbot", "ctaUrl": "/acme/chatbot"}` | **Agent transferred to {account}** — **{agent}** was transferred to **{account}**. → View agent |
| `security.key_changed` (`created`) † | OTel ingest token created (`handlers/otel_ingest_tokens.go`) | managers | `{"keyKind": "OTel ingest", "keyName": "prod-collector", "action": "created", "ctaUrl": "/settings/api-keys"}` | **New {keyKind} key created** — A new {keyKind} key "**{keyName}**" was created. If this wasn't you, revoke it immediately. → Manage API keys |
| `security.key_changed` (`revoked`) † | OTel ingest token revoked (`handlers/otel_ingest_tokens.go`) | managers | `{"keyKind": "OTel ingest", "keyName": "prod-collector", "action": "revoked", "ctaUrl": "/settings/api-keys"}` (`keyName` may be `""`) | **{keyKind} key revoked** — The {keyKind} key "**{keyName}**" was revoked. → Manage API keys |
| `observation.critical` | Failing-agent condition fires — crash loop, OOM, unschedulable (`observation/evaluator.go`) | members | `{"agent": "chatbot", "reason": "Out of memory", "details": "A container was killed for exceeding its memory limit.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is failing** — {reason}. {details} → View deployments |
| `observation.warning` | Degraded-agent condition fires — restart storm, memory/compute over budget, error/latency spike | members | `{"agent": "chatbot", "reason": "Restart storm — worker", "details": "A container restarted many times in a short window.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is degraded** — {reason}. {details} → View deployments |
| `observation.info` | Over-provisioning condition fires — `cpu_over_provisioned`, `memory_over_provisioned` (usage far below request) | members | `{"agent": "chatbot", "reason": "CPU over-provisioned", "details": "CPU usage stayed far below its request — consider lowering it.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is over-provisioned** — {reason}. {details} → View deployments |

Observation workflows default to in-app only (email opt-in). The three severities share one payload shape; the specific condition rides in `reason`. `info` is the cost/waste tier — a healthy agent reserving more than it uses — kept separate from `warning` (a health problem) so users can toggle it independently. A per-episode `DedupeKey` (`{condition}:{deploymentID}:{workload}:{active_since}`) keeps a re-fire after resolve distinct.

### Observation `reason` values

Each observation alert sends two text fields. `reason` is the breaching condition's short title (`Condition.Title` in `internal/observation/conditions.go`) — when the evaluator resolves the breaching workload it appends it as `"{title} — {workload}"` (e.g. `Restart storm — worker`); otherwise just the title is sent. `details` is the condition's one-line explanation of what happened (`Condition.Description`), so the template can tell the user the *what* under the *label*. All observation conditions currently run on the `promql` engine (VictoriaMetrics). Windows are the sustained `for` duration a pod must stay breaching before firing (`0` = the query itself already spans a window).

| `reason` (title) | `details` (description) | Condition | Workflow | Fires when | Window |
|---|---|---|---|---|---|
| Crash loop | A container keeps crashing and restarting, and can't stay running. | `crash_loop` | `observation.critical` | A container is stuck in CrashLoopBackOff | 5m |
| Out of memory | A container was killed for exceeding its memory limit. | `oom_killed` | `observation.critical` | A container's last termination was OOMKilled | on detect |
| Cannot schedule | Pods can't be scheduled — insufficient capacity or quota. | `unschedulable` | `observation.critical` | Pods stuck Pending — insufficient capacity/quota | 10m |
| Restart storm | A container restarted many times in a short window. | `restart_storm` | `observation.warning` | >5 restarts in a 5m window | on detect |
| Memory over budget | Memory use stayed above 90% of its limit. | `memory_over_budget` | `observation.warning` | Working set >90% of the memory limit | 10m |
| Compute over budget | CPU was throttled at its limit most of the time. | `compute_over_budget` | `observation.warning` | CPU CFS-throttled >50% of periods | 10m |
| CPU over-provisioned | CPU usage stayed far below its request — consider lowering it. | `cpu_over_provisioned` | `observation.info` | Peak CPU usage <10% of request | 6h |
| Memory over-provisioned | Memory usage stayed far below its request — consider lowering it. | `memory_over_provisioned` | `observation.info` | Working set <40% of request | 6h |

**Planned (not yet emitted)** — awaiting their query engine/source, so no `reason` ships for them today: `error_spike` and `latency_high` (Langfuse engine, unwired) and `storage_near_full` (messaging sidecar). All three are `warning` → `observation.warning` per the spec.

## Audience policy

| Audience | Resolves to |
|---|---|
| actor | The triggering user |
| subject | The user the event is about |
| members | All account members (actor excluded from broadcasts) |
| managers | Org owner + admins via WorkOS `org:manage`; falls back to account owner |

## Payload variable glossary

| Key | Meaning |
|---|---|
| `account` | Account name (URL handle where used for links) |
| `agent` | Agent name |
| `reason` | Short human-readable failure / condition title (observation: `Title`, may carry `— {workload}`) |
| `details` | One-line explanation of what happened (observation only; the condition `Description`) |
| `role` | Member role (team changes) |
| `action` | Sub-event discriminator: `added`/`role_changed`/`removed` or `created`/`revoked` |
| `keyKind` | Type of key (currently `OTel ingest`) |
| `keyName` | Key display name (may be empty on revoke) |
| `ctaUrl` | Relative app path (absolutized at delivery) or absolute external URL |
