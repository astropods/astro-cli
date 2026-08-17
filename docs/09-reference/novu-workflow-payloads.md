# Novu Workflows — Payloads & Messages

Reference for every Novu workflow astro-server triggers: the payload it pushes and what the resulting message looks like.

## How it works

- All triggers go through one path: `notify.Deliverer.Deliver` → `novu.Client.Trigger` → `POST /v1/events/trigger`. Source: `apps/astro-server/internal/notify/`.
- The **workflow identifier == the event `Type`** string (e.g. `build.failed`). The only override is local dev (`NOVU_TEST_WORKFLOW_ID`).
- Subjects, bodies, and CTA labels are authored in the Novu dashboard templates and composed from payload variables via `{{payload.<key>}}`.
- The backend pushes structured data, with two exceptions: `reason` and `details` are prose, authored in this repo. A template cannot phrase them, because one workflow covers many conditions and the wording depends on which one fired and on the observed value. Treat those two strings as product copy, not as diagnostics.
- `ctaUrl` values starting with `/` are relative app paths, absolutized at delivery against `FrontendURL`. Absolute URLs (e.g. Stripe 3DS links) pass through unchanged.
- Idempotency: each trigger carries a `transactionId` (from `DedupeKey`, else `Type:EntityID`) so Novu drops duplicates.

The **Example message** column is a representative rendering composed from the payload variables — the canonical copy lives in the Novu templates, not the repo. Payloads are authoritative (from `internal/notify/payload.go` and `event.go`). `†` marks `critical` workflows (locked-on, ignore user opt-out).

## Workflows

| Workflow (`Type`) | Trigger | Audience | Payload (sample JSON) | Example message |
|---|---|---|---|---|
| `system.test` † | "Send test" on settings page (`handlers/notifications.go`) | actor | `{"account": "acme"}` | **Test notification** — This is a test notification for **{account}**. Your notifications are working. |
| `account.welcome` | New account created (`handlers/accounts.go`) | actor | `{"account": "acme", "ctaUrl": "/agents"}` | **Welcome to Astro AI** — Your account **{account}** is ready. Deploy your first agent to get started. → Open Astro |
| `build.failed` | Agent container build fails (`riverqueue/github_build.go`) | members | `{"agent": "chatbot", "reason": "The container build failed. Check the build log for the error.", "ctaUrl": "/acme/chatbot"}` | **Build failed: {agent}** — The container build for **{agent}** failed: {reason}. → View agent |
| `billing.payment_failed` † | Stripe `invoice.payment_failed` (`riverqueue/webhook_jobs.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` | **Payment failed** — We couldn't process the payment for **{account}**. Update your payment method to avoid interruption. → Manage billing |
| `billing.action_required` † | Stripe `invoice.payment_action_required` (3DS) | managers | `{"account": "acme", "ctaUrl": "https://invoice.stripe.com/i/acct_.../..."}` | **Action required to complete payment** — The payment for **{account}** needs additional confirmation. → Complete payment |
| `billing.spend_threshold` | Metronome `alerts.spend_threshold_reached`, the account's own limit (`riverqueue/webhook_jobs.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing", "threshold": "$100.00", "spent": "$101.00"}` | **You've reached a spend threshold.** **{account}** crossed a configured spend threshold this period. → View usage |
| `billing.spend_warning` | Metronome `alerts.spend_threshold_reached`, the account's own warning (`riverqueue/webhook_jobs.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing", "threshold": "$80.00", "spent": "$81.00"}` | **Approaching your spend limit.** **{account}** has spent {spent} against a warning set at {threshold}. Agents keep running. → View usage |
| `billing.dunning_suspended` † | Dunning sweep suspends account (`riverqueue/billing_dunning.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` (`account` is best-effort and renders `""` if the account lookup fails) | **Account suspended** — **{account}** has been suspended for non-payment. Update your payment method to restore service. → Manage billing |
| `billing.credits_exhausted` | Metronome `alerts.low_remaining_contract_credit_balance_reached` (`riverqueue/webhook_jobs.go`) | managers | `{"account": "acme", "ctaUrl": "/settings/billing"}` | **Free credit used up.** **{account}** has used its free credit. Add a payment method to keep your agents running. → Manage billing |
| `billing.recovered` | Stripe `invoice.paid` | managers | `{"account": "acme"}` | **Payment recovered** — The outstanding balance for **{account}** has been paid. Service is fully restored. |
| `team.member_changed` (`added`) | Member added (`handlers/org.go`) | subject | `{"account": "acme", "role": "admin", "action": "added", "ctaUrl": "/acme"}` | **You were added to {account}** — You've been added to **{account}** as **{role}**. → Open account |
| `team.member_changed` (`role_changed`) | Member role changed (`handlers/org.go`) | subject | `{"account": "acme", "role": "member", "action": "role_changed", "ctaUrl": "/acme"}` | **Your role changed in {account}** — Your role in **{account}** is now **{role}**. → Open account |
| `team.member_changed` (`removed`) | Member removed (`handlers/org.go`) | subject | `{"account": "acme", "action": "removed"}` | **You were removed from {account}** — You no longer have access to **{account}**. |
| `account.ownership_transferred` | Agent ownership transferred (`handlers/transfer.go`) | managers (of target acct) | `{"account": "acme", "agent": "chatbot", "ctaUrl": "/acme/chatbot"}` | **Agent transferred to {account}** — **{agent}** was transferred to **{account}**. → View agent |
| `security.key_changed` (`created`) † | OTel ingest token created (`handlers/otel_ingest_tokens.go`) | managers | `{"keyKind": "ingestion", "keyName": "prod-collector", "action": "created", "ctaUrl": "/settings/api-keys"}` | **New {keyKind} key created** — A new {keyKind} key "**{keyName}**" was created. If this wasn't you, revoke it immediately. → Manage API keys |
| `security.key_changed` (`revoked`) † | OTel ingest token revoked (`handlers/otel_ingest_tokens.go`) | managers | `{"keyKind": "ingestion", "keyName": "prod-collector", "action": "revoked", "ctaUrl": "/settings/api-keys"}` | **{keyKind} key revoked** — The {keyKind} key "**{keyName}**" was revoked. → Manage API keys |
| `observation.critical` | Failing-agent condition fires — crash loop, OOM, unschedulable (`observation/evaluator.go`) | members | `{"agent": "chatbot", "reason": "Out of memory", "details": "The agent used more memory than its limit allows, so it stopped. Raise the memory limit to keep it running.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is failing** — {reason}. {details} → View deployments |
| `observation.warning` | Degraded-agent condition fires — frequent restarts, near memory limit, slowed by CPU limit, error/latency spike | members | `{"agent": "chatbot", "reason": "Frequent restarts (model-x)", "details": "The agent keeps restarting, which interrupts any request it is handling. It restarted 7 times in the last 5 minutes. Check the agent's logs for the cause.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is degraded** — {reason}. {details} → View deployments |
| `observation.info` | Over-provisioning condition fires — `cpu_over_provisioned`, `memory_over_provisioned` (usage far below the reservation). Both conditions are disabled, so nothing emits this today | members | `{"agent": "chatbot", "reason": "Unused CPU", "details": "The agent uses far less CPU than you reserved for it. At its busiest it used 18% of the reserved CPU. You can lower the reserved CPU to cut cost.", "ctaUrl": "/acme/agents/dep_123/deployments"}` | **{agent} is over-provisioned** — {reason}. {details} → View deployments |

**The two spend controls are separate workflows on purpose.** They fire on the
same provider alert type at different amounts, told apart by the alert name. The
limit suspends the account, so its message says agents stopped. The warning
changes nothing, so reusing the limit's workflow would tell an owner their agents
are down while they are still running. A warning also produces no gating signal,
which is why it reaches the notifier through its own path rather than through
`billingAlert`.

Both amounts arrive pre-formatted as currency, because a template cannot divide
minor units into a currency string.

**One unsourced variable.** The `billing.spend_threshold` email template also
references `{{payload.period}}`, which no event carries. Handlebars renders an
absent variable as empty, so that line arrives blank. Either the template drops
it or the payload grows to carry it.

Observation workflows default to in-app only (email opt-in). The three severities share one payload shape; the specific condition rides in `reason`. `info` is the cost/waste tier — a healthy agent reserving more than it uses — kept separate from `warning` (a health problem) so users can toggle it independently. A per-episode `DedupeKey` (`{condition}:{deploymentID}:{workload}:{active_since}`) keeps a re-fire after resolve distinct.

**Daily cap.** On top of the per-episode dedup, observation alerts are capped to **one send per (deployment, condition) per rolling 24h**. A flapping deployment that resolves and re-breaches repeatedly would otherwise emit one alert per episode; the cap collapses that to one per day. The cap is a persistent ledger (`deployment_alert_notifications`) that survives resolves — the evaluator only sends when the last send for that (deployment, condition) is older than 24h, otherwise it suppresses and logs. Distinct conditions on the same deployment are capped independently, so a crash loop and an OOM on the same agent can both alert the same day.

### `build.failed` `reason` values

`buildFailureReason` (`riverqueue/github_build.go`) classifies the build error by type. Most buckets send a fixed sentence, because the underlying error is compiler or infrastructure output that helps nobody in an inbox.

A spec failure is the exception. `githubbuild.SpecError` carries two phrasings: `Reason` for the reader and `Err` for the log and the build record. `Reason` passes through to the notification unchanged, because it names the commit or the offending line, and that detail is the whole point of the message.

| Error type | `reason` |
|---|---|
| `githubbuild.BuildFailedError` | The container build failed. Check the build log for the error. |
| `githubbuild.SpecError` | `SpecError.Reason`, verbatim (see below) |
| `githubbuild.PermanentError` (any other) | The build stopped on a problem Astro can't retry. Check the build log. |
| anything else (retries exhausted) | The build didn't finish after several tries. Check the build log or try pushing again. |

| Spec failure | `SpecError.Reason` |
|---|---|
| astropods.yml absent at the built commit | No astropods.yml found at commit abc1234. |
| astropods.yml is not valid YAML | astropods.yml has a syntax error on line 4: mapping values are not allowed in this context. |
| astropods.yml fails structural validation | astropods.yml is invalid. agent: must specify either image or build. (At most 3 problems are listed, then "The build log lists N more.") |

Classification is by `errors.As`, not by string matching, so a reworded error message cannot silently change the bucket. The YAML case lifts the line number into the prose (`yamlSyntaxReason`), so the sentence does not stack a second colon in front of the parser's message. It forwards the parser's message only for a syntax error, whose text is prose. A type error reads `cannot unmarshal !!seq into map[string]interface {}`, which would put a Go type, a YAML tag, and a fragment of the reader's own file in their inbox, so those keep the line number and drop the rest: `astropods.yml is not valid YAML. Check line 1.`

### Observation `reason` and `details` values

Each observation alert sends two text fields, both authored in `internal/observation/conditions.go`.

`reason` is the breaching condition's short label (`Condition.Title`). When the evaluator resolves the breaching workload it appends the workload in parentheses (`"Frequent restarts (model-x)"`); otherwise it sends the title alone.

`details` is composed at fire time and reads as claim, then evidence, then fix:

1. `Condition.Description`: what is happening, in one sentence.
2. `Condition.DetailsFor(value)`: the observed number for the worst pod of the workload. Omitted when the condition has no formatter, or when the value is out of range.
3. `Condition.Guidance`: the fix, in one sentence. Every condition sets it.

`Guidance` is a separate field from `Description` because `GET /deployments/:id/alerts` serves `Description` for every condition in the catalog, including conditions that are not firing. Advice belongs in an alert, not in a rule that is currently green.

Copy follows the house rule of no em dashes, so neither field contains one. All observation conditions currently run on the `promql` engine (VictoriaMetrics). Windows are the sustained `for` duration a pod must stay breaching before firing (`0` = the query itself already spans a window).

| `reason` (title) | `details` (description + guidance) | Condition | Workflow | Fires when | Window |
|---|---|---|---|---|---|
| Crash loop | The agent crashes every time it starts, so it can't serve requests. Check the agent's logs for the error that prevents it from starting. | `crash_loop` | `observation.critical` | A container is stuck in CrashLoopBackOff | 5m |
| Out of memory | The agent used more memory than its limit allows, so it stopped. Raise the memory limit to keep it running. | `oom_killed` | `observation.critical` | A container's last termination was OOMKilled | on detect |
| Can't schedule | The agent can't start because Astro has nowhere to run it right now. Check the deployment's events to see what is blocking it. | `unschedulable` | `observation.critical` | Pods stuck Pending, insufficient capacity or quota | 10m |
| Frequent restarts | The agent keeps restarting, which interrupts any request it is handling. Check the agent's logs for the cause. | `restart_storm` | `observation.warning` | >5 restarts in a 5m window | on detect |
| Near memory limit | The agent is close to its memory limit, which will stop it if it goes over. Raise the memory limit to give it room. | `memory_over_budget` | `observation.warning` | Working set >90% of the memory limit | 10m |
| Slowed by CPU limit | The agent keeps hitting its CPU limit, which slows it down. Raise the CPU limit to speed it up. | `compute_over_budget` | `observation.warning` | CPU CFS-throttled >50% of periods | 10m |
| Unused CPU *(disabled)* | The agent uses far less CPU than you reserved for it. | `cpu_over_provisioned` | `observation.info` | P95 peak CPU <40% of the reservation | 6h |
| Unused memory *(disabled)* | The agent uses far less memory than you reserved for it. | `memory_over_provisioned` | `observation.info` | Working set <50% of the reservation | 6h |

The five conditions with a `DetailsFor` formatter insert one more sentence between the description and the guidance:

| Condition | Inserted sentence (sample value) |
|---|---|
| `restart_storm` | It restarted 7 times in the last 5 minutes. |
| `memory_over_budget` | Memory use peaked at 94% of the limit. |
| `compute_over_budget` | It spent 62% of the time waiting for CPU. |
| `cpu_over_provisioned` | At its busiest it used 18% of the reserved CPU. |
| `memory_over_provisioned` | At its busiest it used 43% of the reserved memory. |

The over-provisioned pair quotes the peak as a share of the reservation and stops there. It does not name a smaller reservation: the alert knows the ratio, not the configured value, so any target it named would leave the reader to multiply. Sizing belongs on the deployment page, which the CTA opens.

**Disabled** — `cpu_over_provisioned` and `memory_over_provisioned` stay in the condition catalog but are marked `Disabled`, so nothing emits `observation.info` today. A fixed utilization floor reads an idle agent as waste, so both fired on healthy deployments with nothing to fix.

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
| `reason` | Short human-readable failure or condition label (observation: `Title`, may carry `({workload})`; `build.failed`: one of three classified sentences) |
| `details` | What happened, the observed number, and the fix (observation only; `Description` + `DetailsFor` + `Guidance`) |
| `role` | Member role (team changes) |
| `action` | Sub-event discriminator: `added`/`role_changed`/`removed` or `created`/`revoked` |
| `keyKind` | Type of key (currently `ingestion`, matching the "Ingestion key" label on the settings page) |
| `keyName` | Key display name (may be empty on revoke) |
| `threshold` | The amount the customer set, pre-formatted (`$80.00`). Zero is a real setting and renders `$0.00` |
| `spent` | Period spend that crossed the threshold, pre-formatted. Rounded to whole minor units |
| `ctaUrl` | Relative app path (absolutized at delivery) or absolute external URL |
