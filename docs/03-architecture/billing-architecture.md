# Astro billing architecture

For a short version, read
[`billing-overview.md`](billing-overview.md) first. For the function-by-function
view with file references, read
[`billing-data-flow.md`](billing-data-flow.md).

This document explains how Astro meters usage, invoices customers, collects
payment, and stops accounts that do not pay. It covers the server, the client,
the CLI, the operator tooling, and the three external systems the design depends
on.

Read it before you change anything under `apps/astro-server/internal/billing`,
`internal/riverqueue/billing_*`, or `internal/middleware/entitlement.go`.

`billing-primer/README.md` covers the same ground in a more narrative form and
goes further on astro-infra. It predates the spend-control and recovery work, so
prefer this document where they disagree.

Status as of 2026-08-17: preview enforces the gate, production does not. See
[Environments and configuration](#environments-and-configuration).

---

## Contents

- [The one-paragraph version](#the-one-paragraph-version)
- [Three external systems, three jobs](#three-external-systems-three-jobs)
- [What we send to Metronome](#what-we-send-to-metronome)
- [Customer identity](#customer-identity)
- [Data model](#data-model)
- [Lifecycle 1: provisioning](#lifecycle-1-provisioning)
- [Lifecycle 2: metering](#lifecycle-2-metering)
- [Lifecycle 3: invoicing and collection](#lifecycle-3-invoicing-and-collection)
- [Lifecycle 4: signals and the status machine](#lifecycle-4-signals-and-the-status-machine)
- [Lifecycle 5: gating](#lifecycle-5-gating)
- [Lifecycle 6: recovery](#lifecycle-6-recovery)
- [Customer-set spend controls](#customer-set-spend-controls)
- [Notifications](#notifications)
- [The client](#the-client)
- [The CLI](#the-cli)
- [Operator tooling](#operator-tooling)
- [Environments and configuration](#environments-and-configuration)
- [Testing](#testing)
- [Design rules](#design-rules)
- [Known gaps](#known-gaps)

---

## The one-paragraph version

Astro measures compute reservations every five minutes and sends them to
Metronome as usage events. Metronome rates that usage against a contract, closes
a billing period, and produces an invoice, which it delivers to Stripe. Stripe
charges the card the account vaulted. When a charge fails or a threshold is
crossed, the provider sends a webhook. astro-server maps the webhook to an
account, writes one cached row, and recomputes a status from that row. The
status is the only thing the request path reads, and a suspended status refuses
deploys and stops running workloads.

Astro never charges a card and never computes a price.

---

## Three external systems, three jobs

Each system owns one thing. The boundaries matter, because the most common
mistake in this area is re-deriving a fact one of them already owns.

| System | Owns | Astro never |
|---|---|---|
| **Metronome** | Rates, contracts, credit balances, period spend, invoicing, customer-set thresholds | Computes a price, stores a balance, stores a threshold |
| **Stripe** | The card vault, the charge, retry schedules, invoice PDFs | Charges a card, decides when to retry |
| **Novu** | Notification templates, channels, per-subscriber preferences | Writes message copy, decides a channel |

Astro owns the mapping between them, the cached gating status, and the decision
to stop an account.

```
                  usage events (5 min)
  astro-server  ──────────────────────>  Metronome
       ▲                                     │
       │                                     │ invoice
       │ webhooks                            ▼
       │ (alerts, credit)                 Stripe  ── charge ──> customer card
       │                                     │
       └──────────── webhooks ───────────────┘
                (payment succeeded / failed)
```

---

## What we send to Metronome

Two senders, one wire format. Both POST to `/v1/ingest`, batched at 100 events per
request, with the same envelope:

```json
{
  "transaction_id": "…",              // idempotency key, 34-day dedupe window
  "customer_id": "<astro account id>", // resolved by ingest alias
  "event_type": "deployment_compute_usage",
  "timestamp": "2026-08-17T14:05:00Z", // RFC3339, UTC
  "properties": { "cu_hours": 0.083, "…": "…" }
}
```

| | Compute | LLM |
|---|---|---|
| `event_type` | `deployment_compute_usage` | `ai_gateway_llm_usage` |
| Sender | astro-server `MeteringWorker`, every 5 minutes | `bifrost-otel` collector, per LLM request |
| `transaction_id` | `deployment:component:windowStart` | the Bifrost request ID, or the trace ID when absent |
| `properties` | `cu_hours`, `agent_name`, `deployment_id`, `component`, `cpu`, `memory`, `replicas` | `model`, `provider`, `input_tokens`, `output_tokens`, `total_tokens`, `cached_read_tokens`, `reasoning_tokens`, `virtual_key_id`, and `cost_usd` when the span carries a cost |
| Billable metric | Compute Units, `sum(cu_hours)` | LLM Usage, `sum(cost_usd)` |

**Only the property a metric names is billed.** Everything else in `properties`
rides along for the invoice breakdown and for debugging. That is why a gateway
request whose span has no cost attribute bills nothing: the metric filters on
`cost_usd` existing.

### How each sender delivers

Both authenticate with the same `METRONOME_API_KEY` as a bearer token, and each
environment has its own key.

**Compute, in-process.** `MeteringWorker` calls `IngestUsage` through the Metronome
Go SDK, in the same pass that advances `last_emitted_at`. Nothing buffers the push:
a failure leaves the anchor where it was, so the next tick re-emits the same
windows, which Metronome recognises and ignores.

River is the background-job system astro-server runs on, and every scheduled or
deferred piece of billing work is a River job: provisioning, the dunning sweep,
suspend and resume, webhook handling, and this metering tick. Two properties of it
matter here.

- **Jobs live in Postgres**, in the same database as the billing tables, and are
  claimed by whichever astro-server replica gets there first. There is no separate
  broker to run, and one tick executes once across the fleet rather than once per
  replica.
- **Jobs are named, queued, and retried by the framework.** `MeteringArgs` is the
  job kind, `metering` is the queue it runs on, and the schedule is a River periodic
  job at five minutes, declared where the worker is registered. Returning an error
  from `Work` puts the job back with backoff, which is why worker code can push and
  fail rather than track its own retries.

The same machinery is what makes webhook redelivery safe: a job kind carrying
`river:"unique"` on its event ID collapses a repeat into one job. See
[Lifecycle 4](#lifecycle-4-signals-and-the-status-machine).

**LLM, through the collector.** Bifrost exports one OTLP trace per request to
`bifrost-otel`, whose exporter POSTs the batch to `{endpoint}/v1/ingest` with a
plain HTTP client. It sits behind the standard exporterhelper blocks configured in
`collector-config.yaml`: a `sending_queue` persisted to `file_storage/metronome`,
which survives a restart, and `retry_on_failure` backoff. The exporter treats 429
and 5xx as retryable and every other 4xx as permanent. The unprotected hop is
Bifrost to the collector, which is fire-and-forget.

The rest of the pipeline mechanics are in [Lifecycle 2](#lifecycle-2-metering).

---

## Customer identity

One Astro account holds two external customer records, in separate columns on
`accounts`:

| Column | System | Accessor |
|---|---|---|
| `metronome_customer_id` | Metronome | `Get/SetBillingCustomerID(accountID, backend)`, backend-keyed |
| `stripe_customer_id` | Stripe | Its own accessor, deliberately outside that whitelist |
| `bifrost_customer_id` | AI gateway | Aliased onto the Metronome customer |

The backend-keyed accessor means adding a metering backend does not touch call
sites. Stripe sits outside it because Stripe is not a metering backend; it is the
vault.

### Ingest aliases are the attribution trick

The Metronome customer's **ingest alias is the Astro account ID**. Anything
anywhere in the stack can emit a usage event keyed on the plain account ID and
Metronome attributes it correctly. There is no mapping table and no lookup
service.

A second alias is the Bifrost (AI gateway) customer ID, so gateway usage rolls up
to the same customer. `billing.AliasSyncer` keeps it in sync when gateway keys are
minted, best-effort, so a failure never blocks key minting.

### Deletion archives before it deletes

`DeleteAccount` archives the billing customer **before** the soft delete, and a
failed archive aborts the delete. The ordering is the point: an account must never
end up deleted and still billable.

---

## Data model

### `accounts`

| Column | Purpose |
|---|---|
| `billing_provisioned_at` | Set once the account holds a Metronome customer and a contract, with or without signup credit. `NULL` keeps it in the provisioning sweep. |

The three customer-ID columns are covered in [Customer identity](#customer-identity).

### `account_billing_status`

One row per account. Absence means active. This is the only table the request
path reads.

| Column | Meaning |
|---|---|
| `status` | `active`, `past_due`, or `suspended`. Derived, never written directly. |
| `reason` | Why, when suspended. Derived. |
| `dunning_since` | Set on the first payment failure. Survives redelivery. |
| `alert_active` | The customer's spend limit is crossed and uncleared. |
| `force_suspended` | Stripe wrote the invoice off. Bypasses the grace window. |
| `credits_exhausted` | Signup credit is spent. |
| `has_payment_method` | A card is vaulted. |

The four booleans and the timestamp are **latches**. Signals set and clear them.
`status` and `reason` are recomputed from the latches on every write, so the two
can never disagree.

### `deployment_billing_state`

One row per (deployment, component). Carries `last_emitted_at`, the CPU and
memory reservation, and the replica count. The anchor marks the last window
billed, so a tick knows which windows have closed since.

### `billing_credit_grants`

One row per **user**: `user_id` as the primary key, plus `account_id` and
`granted_at`. It records which account took that person's signup credit. The row is
not removed with the account. See [Lifecycle 1](#lifecycle-1-provisioning).

---

## Lifecycle 1: provisioning

An account has to exist in Metronome and sit on a rate card before any usage it
generates can be rated. Whether it also holds signup credit depends on its
creator.

1. Account creation enqueues `billing.provision`.
2. `BillingProvisionWorker` reads or creates the Metronome customer, passing the
   account ID, name, type, and the **owner's WorkOS-verified email**, not the
   requesting user's.
3. `plan()` picks one of three packages. An internal creator is answered first,
   before the ledger is touched; otherwise `claimSignupCredit` decides.

   | Plan | Chosen when | Package |
   |---|---|---|
   | `unlimited` | the creator's verified address matches `BILLING_UNLIMITED_EMAIL_DOMAINS` | `METRONOME_PACKAGE_ID_UNLIMITED` |
   | `credit` | that person's one signup credit is still unclaimed | `METRONOME_PACKAGE_ID` |
   | `no_credit` | they already spent it on another account | `METRONOME_PACKAGE_ID_NO_CREDIT` |
4. `ProvisionCustomer` puts the customer on the rate card with that package. It is
   idempotent, keyed provider-side on the account ID.
5. The worker marks `billing_provisioned_at`.
6. It applies `SignalCreditsGranted`, so an operator credit grant re-running this
   job also lifts an exhaustion latch.

**Signup credit is one per person, not one per account.** The claim row is keyed on
the user ID and outlives the account that took it, so delete-and-recreate does not
restore it. The claim is idempotent for its holder (`ON CONFLICT DO UPDATE …
RETURNING account_id`), so a job that claimed the credit and then failed before
creating the contract still reads true on retry. A creator that cannot be resolved
gets no credit, and the account is still provisioned, because an account with no
contract has its usage rated by nothing at all. A missing credit-free package fails
the job rather than falling back to the credit package, and the hourly sweep
re-runs it once the configuration lands.

**The unlimited plan is a package, not a gate exemption.** It carries the same
rate card and statement schedule as the other two, with every metered product
overridden to a zero multiplier. Usage still meters and still appears on the
statement; the total is always zero. Putting the guarantee in the rating means it
holds whether or not the gate logic is correct.

Matching is exact, on the part after the last `@` of a **verified** address, so
neither a subdomain nor a lookalike like `evil-postman.com` matches. The address
comes from `GetCreatorVerifiedEmail`, which pins to the creator rather than
joining across members: a later member's domain must not decide the plan. A
creator with no verified address takes the standard plan.

An internal account never reaches the credit ledger, so an employee does not
spend their one claim on a plan that has no use for it.

**Provisioning cannot change a plan.** `ProvisionCustomer` returns early whenever
any contract covers now, because a second contract would bill the customer twice.
Re-running the job is therefore a no-op, and moving an existing account is a
Metronome renewal transition against the covering contract. Clearing
`billing_provisioned_at` re-runs only the tail: mark, `SignalCreditsGranted`,
reconcile. That is the lever for releasing a stuck latch without touching money.

**Provisioning is optional on the seam.** `Provisioner` is a separate interface
detected by assertion, so the OSS no-op backend implements nothing and the code
path is inert rather than conditional.

**An hourly sweep backfills.** `BillingProvisionSweepWorker` selects accounts with
`billing_provisioned_at IS NULL`. An account whose signup enqueue was dropped, or
that predates provisioning, gets picked up. A provider that reports "not
configured" leaves the mark unset deliberately, so the account stays in the sweep
until the configuration lands.

**Stripe linking is separate and lazy.** The Stripe customer is created the first
time someone opens the billing page or saves a card. Saving a card also links the
Stripe customer to the Metronome customer, so Metronome's invoices know which
card to charge.

---

## Lifecycle 2: metering

Two independent pipelines feed Metronome. Both attribute by account ID, because
both rely on the ingest alias.

| Pipeline | Event type | Billable metric | Aggregates | Owner |
|---|---|---|---|---|
| Compute reservations | `deployment_compute_usage` | Compute Units | `sum(cu_hours)` | astro-server |
| LLM calls | `ai_gateway_llm_usage` | LLM Usage | `sum(cost_usd)` | astro-infra, `bifrost-otel` |

Resource counts (agents, deployments, members) are **not** metered. They are
DB-backed quotas in `internal/quota`, a separate system with its own 402s.
Knowledge storage and knowledge compute metering exist in the code but are
dormant.

### Pipeline 1: compute, from astro-server

`MeteringWorker` runs every five minutes on the `metering` queue.

**What is measured: reservations, not consumption.** Compute units come from what
a workload reserved, not what it used:

```go
cu := max(cpuCores, memGB/2) * replicas
```

A customer pays for the capacity they asked us to hold. This is deliberate. Usage
would make a bill unpredictable and would charge nothing for an idle agent
occupying a node.

**Billed on closed windows.** Time is divided into five-minute windows aligned to
the clock. `BillingStateManager.RunBillingCycle` reads `deployment_billing_state`
and emits one event per window that has fully closed since `last_emitted_at`,
then advances the anchor to the last window it emitted. A missed tick bills
correctly on the next one instead of silently dropping an interval, because the
windows it skipped are still closed and still unemitted.

A window that is still open is not billed. It has no final value yet, and the
tick that saw it half elapsed would send a different amount than the tick that
saw it whole. Usage is therefore billed up to one window after it is incurred.

Each cycle does four things in order:

1. `healMissingBillingRows` creates state rows for workloads that have none.
2. `emitActiveBilling` bills the closed windows of active, non-stale rows.
3. `reconcileStale` bills a stale row up to its `status_changed_at` and stops.
4. `reconcileStopped` closes rows for workloads that no longer run.

Stale rows are excluded from step 2 so step 3 can bill them to the exact moment
they stopped, rather than to a window boundary.

Catch-up is capped at 24 hours of windows per row per tick. The anchor advances
to the last window emitted, so the next tick continues from there.

**The transaction ID names the window, not the send.** It is
`deployment:component:windowStart`, so the same stretch of time always produces
the same ID and Metronome's 34-day dedupe drops a repeat instead of charging it.
The two closing paths bill up to a recorded stop time rather than the grid, so
their IDs carry both ends of that span. The event is stamped with the end of the
span it covers, because Metronome files usage into a billing period by event
time. The event type is `deployment_compute_usage`, subject is the account ID,
and the payload carries `cu_hours`, `agent_name`, `deployment_id`, `component`,
`cpu`, `memory`, and `replicas`.

**Components are billed separately.** The agent, each model, each knowledge
store, integrations, interfaces, and observability each get their own row and
their own event, so an invoice can break down where the money went.

**The anchor advances only after a clean push,** so nothing is lost when an
ingest fails. It is no longer what keeps the bill correct. The anchor is written
in a separate statement that can fail on its own, and an ingest can succeed while
the response is lost; either leaves the anchor behind while Metronome already
holds the usage. The next cycle re-emits those windows under the IDs they already
carry, and Metronome ignores them. The anchor now decides when work is repeated,
not whether it is right.

### Pipeline 2: LLM tokens, from astro-infra

This one never touches astro-server. Bifrost already emits one OTLP trace per LLM
request. Its `otel` plugin fans out to Langfuse and to `bifrost-otel`, a
purpose-built OTel Collector distribution that POSTs Metronome events. Enabling it
was a Helm-values change; the Bifrost image was untouched.

**Retry dedupe is the correctness-critical part.** A trace holds a root span plus
one span per attempt. The exporter picks only the final successful attempt,
highest retry count with success winning ties. Summing the spans would over-bill
every retried request. All attempts share one request ID, which becomes the
transaction ID.

**The event carries tokens and cost; the billable metric bills the cost.** The
exporter emits `input_tokens`, `output_tokens`, `total_tokens`,
`cached_read_tokens`, `reasoning_tokens`, `model`, `provider`, and
`virtual_key_id`. Verified against the live Metronome environment 2026-08-14, the
`LLM Usage` billable metric sums **`cost_usd`**, with a property filter requiring
it to exist, and the rate card prices a single `AI Gateway` product off that sum.
The token counts are carried but not billed.

Two consequences follow, and neither is obvious from the exporter:

- Bifrost's own computed cost **is** the billing basis, not a cross-check. A
  change to how Bifrost prices a model changes what a customer pays, with no
  change on our side.
- `cost_usd` is set conditionally, only when `gen_ai.usage.cost` is present on the
  span. The billable metric filters on that property existing. A request whose
  cost attribute is missing is ingested and matched by no metric, so it bills
  nothing and raises no error.

Attribution is free: the Bifrost governance customer ID is already the Astro
account ID, which is already the Metronome ingest alias.

**Durability caveat.** Bifrost's OTel export is fire-and-forget, so the
Bifrost-to-collector hop can lose events. Downstream of the receiver the collector
has a file-storage-backed queue plus retry, which survives restarts and Metronome
downtime. The intended safety net is a reconcile job comparing metered usage
against Bifrost's governance spend. That job is not built.

---

## Lifecycle 3: invoicing and collection

This is the part Astro participates in least.

1. Metronome closes the billing period and finalizes an invoice.
2. Metronome delivers it to Stripe through the customer's **billing provider
   configuration**, which names the Stripe account and the Stripe customer.
3. Stripe finalizes its own invoice and charges the default card about an hour
   later.
4. Stripe emits `invoice.paid` or `invoice.payment_failed`.

**The billing provider configuration is the fragile link.** It is what makes an
invoice reach Stripe at all. A customer can have a configuration and still not be
routed, because the *contract* needs a `billing_provider_configuration_schedule`
pointing at it. A missing schedule produces an invoice that finalizes in
Metronome and never appears in Stripe, with no error anywhere. `station metronome
audit` exists to detect exactly this.

The delivery method ID differs per environment. It is resolved at call time from
`listConfiguredBillingProviders`, so no configuration is needed, but an
environment with more than one Stripe connection is ambiguous and the link errors
rather than guessing.

**Astro never charges.** There is one exception, added deliberately: after a
customer saves a card, `billing.collect` asks Stripe to pay their open invoices.
That is not a new charge decision. It is asking Stripe to run the collection it
was already going to run, now rather than on its own retry schedule. See
[Lifecycle 6](#lifecycle-6-recovery).

---

## Lifecycle 4: signals and the status machine

### Webhooks in

Two endpoints, both disabled with 404 when their secret is unset.

| Endpoint | Verification |
|---|---|
| `POST /webhooks/metronome` | HMAC-SHA256 over `X-Metronome-Date + "\n" + rawBody`, hex, compared to `Metronome-Webhook-Signature` |
| `POST /webhooks/stripe` | `stripe-go`'s `webhook.ConstructEvent` against `Stripe-Signature` |

Both read the raw body before any JSON middleware, because both signatures cover
the exact bytes.

Neither handler does any work. Each enqueues a River job and returns. A failed
enqueue returns 500 so the provider redelivers, because an event that is not yet
tracked must not be acknowledged.

**Redelivery is expected and deduped.** `EventID` carries `river:"unique"`, so
River collapses a repeated event into one job even when sibling fields differ.
Providers repeat until they see a 2xx; without this, a redelivered gating event
would apply a suspension twice.

### Event to signal

The workers translate provider vocabulary into Astro's twelve signals.

| Provider event | Signal |
|---|---|
| `invoice.payment_failed` | `SignalPaymentFailed` |
| `invoice.payment_action_required` | `SignalActionRequired` |
| `invoice.marked_uncollectible` | `SignalUncollectible` |
| `invoice.voided` | `SignalVoided` |
| `invoice.paid` | `SignalRecovery` |
| `payment_method.automatically_updated` | `SignalCardUpdated` |
| `payment_method.attached` / `.detached` | `SignalCardAdded` / `SignalCardRemoved` |
| `alerts.spend_threshold_reached` (limit) | `SignalAlert` |
| `alerts.spend_threshold_resolved` | `SignalAlertResolved` |
| `alerts.low_remaining_contract_credit_balance_reached` | `SignalCreditsExhausted` |
| ...`_resolved` | `SignalCreditsGranted` |

`invoice.payment_succeeded` overlaps `invoice.paid`; only one is consumed.

**The credit alert cannot gate an unlimited account.** The unlimited package
grants no credit, so Metronome's zero-threshold balance alert reads an empty
balance as an exhausted one. It fires on the first evaluation and stays in alarm
with nothing to resolve it. The Metronome worker drops `SignalCreditsExhausted`
for an account on the unlimited plan, resolving the plan the same way
provisioning resolved it rather than storing the verdict, because a stale copy
would gate an internal account for money.

The card events are a **backstop**. The card handlers write `has_payment_method`
inline and best-effort; Stripe redelivers, an inline write does not.

### `ApplySignal` and `computeStatus`

`ApplySignal` writes one latch and recomputes. It performs no provider calls and
touches no workloads. The caller reconciles from the returned `(status, changed)`.

`computeStatus` is pure, and first match wins:

| Rank | Condition | Status | Reason |
|---|---|---|---|
| 1 | `force_suspended` | suspended | `uncollectible` |
| 2 | `alert_active` | suspended | `balance_alert` |
| 3 | `credits_exhausted && !has_payment_method` | suspended | `credits_exhausted` |
| 4 | `dunning_since` older than grace | suspended | `payment_failed` |
| 5 | `dunning_since` within grace | past_due | `dunning` |
| 6 | otherwise | active | |

Two properties follow from the ranking that are easy to get wrong:

- **Clearing one latch drops to the next, not to active.** Voiding an invoice
  clears the write-off but leaves an exhaustion latch in place. Telling a
  spend-limited account it is fine because an unrelated invoice was voided would
  resume it over its own threshold.
- **Rank 3 is conditional on the card.** Credits exhausted with a card on file is
  not a problem; it is the pay-as-you-go transition. The latch stays set, because
  the credits really are spent. It just stops gating.
- **Rank 3 is also where an unlimited account would have been caught.** Internal
  accounts hold no card, by design, so the exemption has to happen before the
  signal is applied rather than inside this table. `computeStatus` stays pure and
  plan-blind.

### The dunning sweep

`DunningSweepWorker` runs hourly. It is a pure timer with no provider calls: it
lists accounts with `dunning_since` set and recomputes each against `now`. An
account that crossed the grace boundary flips to suspended, enqueues
`billing.suspend`, and notifies the owner.

Grace defaults to seven days (`BILLING_DUNNING_GRACE_DAYS`).

The clock does not restart. `SetDunningSince` uses `COALESCE`, so a provider
redelivery keeps the original stamp. A clock that restarted on every retry would
walk the deadline forward forever.

---

## Lifecycle 5: gating

### What blocks

`Entitlements.Check` reads the cached status with one keyed lookup. It never calls
a provider on the request path and never reads a balance.

Only `StatusSuspended` blocks. `past_due` is a warning state and still runs.

Nine call sites:

- Deploy, restart deployment, restart pod, wake, rollback
- Connect a knowledge store
- Non-GET on the messaging proxy, ingestion, and chat

Reads are never blocked. A suspended account can still see its agents, its logs,
and its bill. Refusing a GET would turn a billing state into an information
blackout at the moment the owner most needs to understand it.

**Image push is deliberately not gated.** Push terminates at astro-registry and
never reaches astro-server. Gating it was written and closed: the decision is that
only the two Metronome-tracked rates ever stop an account, and they stop
deployments only.

### Two failure modes, both chosen

**Fails open.** A status read error allows the request. A database blip must not
become an outage for accounts that owe nothing.

**Observe mode.** With `BILLING_GATE_ENFORCE=false` the gate evaluates, logs what
it would have refused, and allows. This is how the gate ships into an environment
before anyone trusts it.

### The 402

```json
{
  "error": "Billing suspended",
  "code": "BILLING_SUSPENDED",
  "reason": "credits_exhausted",
  "action": "add_card",
  "details": "This account's free credits are used up. Add a payment method to continue."
}
```

`action` is the contract with every client. Three values: `add_card`,
`update_card`, `contact_support`. An unrecognised reason maps to
`contact_support`, because a build that cannot name the problem must not send an
owner to change a card that may be fine.

The server names the fix; each surface writes its own words. A terminal and a
banner can phrase it differently without disagreeing on what to do.

### Stopping workloads

`BillingSuspendWorker` walks the account's active deployments and, for each,
calls `StopNamespaceWorkloads`, which scales every managed Deployment and
StatefulSet to zero and suspends every managed CronJob. The CronJob branch
matters: a scheduled ingestion has no replica count and would otherwise keep
consuming after the agent stopped.

Deployments are marked `StatusSuspended`, distinct from a user's `StatusStopped`,
so resume restores only what billing stopped.

---

## Lifecycle 6: recovery

Each gating reason has exactly one exit, and they do not substitute for each
other.

| Reason | Clears on |
|---|---|
| `credits_exhausted` | A card is added, or credit is granted |
| `dunning` / `payment_failed` | `invoice.paid`, or a card-network auto-update |
| `balance_alert` | `alerts.spend_threshold_resolved`, the provider's own IN_ALARM to OK edge |
| `uncollectible` | `invoice.voided` |

**A payment does not lower period spend**, so `SignalRecovery` clears dunning and
nothing else. **A void is not a payment**, so it clears the write-off without
implying money moved.

### Adding a card is not, by itself, recovery

`SignalCardAdded` records that a card exists. It does not clear dunning, because
the debt is still unpaid, and the new card might also decline.

For credits-exhausted that is enough: the gate condition includes
`!has_payment_method`, so the card alone lifts it inside the request.

For a failed payment it is not. Nothing in the system asks Stripe to charge the
new card, and Stripe's own retry schedule can be days out or already exhausted.
So `applyCardSignal` enqueues `billing.collect` whenever the recomputed status is
anything but active. That job pays the customer's **open** invoices only: drafts
are not finalized, and paid, void, and uncollectible are settled.

The job writes no status. Success produces `invoice.paid`, which the existing
webhook path already turns into recovery and resume. A payment that lands outside
our window travels the same code, so dunning clears in one place rather than two.

### Resume

`BillingResumeWorker` re-applies the spec rather than remembering a replica
count. It sets deployments back to pending and enqueues the normal wakeup, which
reapplies manifests.

Resume is **not** gated by `BILLING_GATE_ENFORCE`. Suspend is. Resume is
remediation, not enforcement: it only restores deployments left in
`StatusSuspended`, which nothing but billing sets. Gating it would mean that
turning the flag off after a real suspension could not undo it, and an account
that then fixed its card would stay at zero replicas.

---

## Customer-set spend controls

An account can set two numbers for itself.

|  | Spend warning | Spend limit |
|---|---|---|
| Alert name | `astro:spend_warning` | `astro:spend_limit` |
| On crossing | Notifies the owner | Suspends the account |
| Signal | None | `SignalAlert` |

Both are per account. A user in three organizations has three independent sets,
and each notifies that account's managers.

**Metronome is the only store.** Astro persists neither number. The read goes to
`v1/customer-alerts/list` and the write creates or replaces the alert. There is
no local copy to drift, and no migration to write.

**The page shows the number the thresholds measure.** Both alerts fire on
usage-based spend before commit and credit drawdown, so the card reads `UsageSpend`,
the sum of the draft invoice's `usage` line items, not `CurrentSpend`, which is net
of drawdown. `HasUsageSpend` separates an invoice with no usage from one totalling
zero. One response carries two money units on purpose: spend and credit are
converted to their named currency for astro-queen, and the thresholds stay in the
provider's raw cents, which is what the write path sends.

**The two are the same provider alert type at different amounts**, told apart by
the alert name. That makes the name load-bearing:

- A warning must produce no gating signal. `metronomeSignal` returns early on the
  warning name, for the reached edge **and** the resolved edge. Handling the
  resolved edge would clear the latch a limit had set.
- A warning reaches the notifier through its own branch in the worker, which
  returns before the signal logic. The two paths are mutually exclusive by
  construction.
- An operator's hand-made alert is neither, and gates as it did before customer
  controls existed.

**Amounts decode as JSON numbers, not integers.** Metered spend accrues
fractional cents. One envelope decodes every Metronome webhook, including the
limit event that suspends an account, so an `int64` field rejecting `8034.5` would
answer 400 and drop the gating event for the sake of a number only the message
text uses.

---

## Notifications

Seven billing workflows, all triggered through `notify.Deliverer.Deliver` →
`novu.Client.Trigger`.

| Workflow | Fires on |
|---|---|
| `billing.payment_failed` | `invoice.payment_failed` |
| `billing.action_required` | `invoice.payment_action_required` (3DS) |
| `billing.spend_threshold` | The account's own limit |
| `billing.spend_warning` | The account's own warning |
| `billing.credits_exhausted` | Credit balance reaches zero |
| `billing.dunning_suspended` | The sweep suspends the account |
| `billing.recovered` | `invoice.paid` |

Audience is **managers**: owners and admins, resolved through WorkOS with a
fallback to the first account member.

**The workflow ID is the event type.** A mismatch produces a 422 the user never
sees.

**The backend pushes structured data.** All wording lives in Novu templates,
composed from `{{payload.<key>}}`. Two exceptions are prose authored in the repo,
`reason` and `details`, because one workflow covers many conditions.

**Money is pre-formatted.** A template cannot divide minor units into a currency
string, so `threshold` and `spent` arrive as `"$80.00"`.

**CTA URLs starting with `/` are absolutized at delivery** against
`FrontendURL`. Absolute URLs, such as a Stripe hosted-invoice link, pass through
unchanged.

**A trigger that delivers nothing is an error.** Novu answers 201 and reports the
real outcome in the body. Four configuration statuses mean the trigger reached
nobody and cancel the job permanently. Every other status, including Novu's
generic `error`, retries on backoff: a threshold crossing fires once, so
discarding an outcome we cannot prove is permanent loses the only notification
there will be.

---

## The client

All reads go through TanStack Query hooks in `src/api/queries/billing.ts`. No
component calls `api.*` for a read.

| Surface | File |
|---|---|
| Gating banner | `components/BillingStatusBanner.tsx` |
| Billing settings page | `components/settings/BillingView.tsx` |
| Card form (Stripe Elements) | `components/settings/PaymentMethod.tsx` |
| Warning and limit | `components/settings/SpendControls.tsx` |
| Which plan the account is on | `components/settings/PlanSummary.tsx` |
| Shared copy | `lib/billing-copy.ts` |

**The client composes no billing copy and re-derives no rule.** The banner
renders the server's `gated` flag, the button and the agent tooltip come from the
server's `action`, and a refused toggle toasts the server's own message.

Adding a card is a Stripe Elements iframe. The client confirms a SetupIntent with
Stripe.js, then posts the intent ID; the server re-reads the intent from Stripe
rather than trusting the client, saves the card as default, and links the
customer.

---

## The CLI

`ast` renders a refusal rather than dumping the JSON body. A typed `apiError`
crosses `apiCall`, `apiCallWithHeaders`, `apiUpload`, and `apiStream`.

- It keys on the `BILLING_SUSPENDED` **code**, not the status, because
  astro-server answers 402 and other surfaces may answer differently.
- The server supplies the sentence; the CLI adds the next step from `action`. An
  unrecognised action prints the explanation and no next step.
- A billing refusal exits **3**. A script that retries on failure has to tell a
  transient server problem from an account that cannot run anything until someone
  pays.
- Every non-billing error keeps the exact text it had, including a body that is
  not JSON.

---

## Operator tooling

### `station`

The local operator CLI. Reads preview and dev by default.

```
station metronome audit       # every account's contract coverage and delivery routing
station metronome account     # cross-check one astro account against Metronome
station metronome balances    # credit and commit balances, which is what gating fires on
station metronome contracts   # a customer's contracts
station novu workflows        # what the environment holds, and whether it is active
station novu billing-workflows # report or author the seven billing workflows
```

`metronome audit` is the one to run when an invoice does not arrive. It reports a
verdict per account: `ok`, `no-card`, `missing`, `unrouted`, `no-delivery-method`,
or `stripe-mismatch`.

### astro-queen

The admin console exposes `GetAccountBillingDetail`, which shows the cached
status, contract coverage, the provisioning job, and deep links to the Metronome
and Stripe dashboards. Two write operations exist: `RetryBillingProvision` and
`ForceBillingResume`.

---

## Environments and configuration

| Variable | Purpose |
|---|---|
| `BILLING_PROVIDER` | `metronome` or unset for the OSS no-op |
| `METRONOME_API_KEY` | Per environment |
| `METRONOME_WEBHOOK_SECRET` | Unset disables the endpoint with 404 |
| `METRONOME_PACKAGE_ID` | The rate card package, with signup credit |
| `METRONOME_PACKAGE_ID_NO_CREDIT` | Same terms, no grant. Used when the creator's credit is already claimed. Missing means provisioning fails rather than granting again |
| `METRONOME_PACKAGE_ID_UNLIMITED` | Same terms at a zero multiplier. Required whenever a domain list is set, checked at boot |
| `BILLING_UNLIMITED_EMAIL_DOMAINS` | Comma-separated, defaults to `postman.com`. An explicit empty value turns the plan off and drops the package requirement |
| `METRONOME_DASHBOARD_ENV` | Names the environment in astro-queen's Metronome deep links |
| `STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY` | The vault |
| `STRIPE_WEBHOOK_SECRET` | Unset disables the endpoint with 404 |
| `BILLING_GATE_ENFORCE` | `true` enforces; anything else observes |
| `BILLING_DUNNING_GRACE_DAYS` | Default 7 |

**Current state.** `config/astro-server/preview.env` runs
`BILLING_PROVIDER=metronome` with `BILLING_GATE_ENFORCE=true`.
`config/astro-server/prod.env` runs `BILLING_PROVIDER=noop` with
`BILLING_GATE_ENFORCE=false`, so nothing in production is metered or gated today.

Production therefore needs no migration. No production account holds a contract,
so the first provisioning sweep after the flip puts every account on the right
plan. Flipping those two flags is the moment every path in this document starts
acting on real customers, and the packages have to exist in the production
Metronome environment first, because packages do not cross environments.

Before the first production invoice, check "Leave invoices as drafts" in the
production Stripe account. It is dashboard-only, per Stripe account, with no API,
and it was on in the sandbox.

---

## Testing

Four tiers, each catching something the one below cannot.

**Unit.** The status machine, the signal matrix, the reason rank, the copy
mapping. Fast, and where most of the logic lives.

**Integration** (`-tags integration`, real Postgres). The latch matrix, the
dunning clock across a redelivery, the reason rank round trip, and River actually
collapsing a duplicate webhook. sqlmock asserts the SQL we send; these assert what
a database does with it.

**K8s** (`-tags k8s`, kind plus vcluster). `StopNamespaceWorkloads` scaling
Deployments and StatefulSets to zero, suspending CronJobs, staying idempotent, and
a re-apply restoring the original replica counts.

**Live against preview.** Neither harness can catch a wrong belief about an
external API, because both stub it, and a stub encodes the belief. Only a live run
observes `external_invoice.external_status` flipping. All seven notification
workflows and all four failure paths have been exercised this way.

Run them:

```
moon run astro-server:test                # unit
moon run astro-server:test-integration    # Postgres
bash scripts/e2e.sh setup                 # kind + vcluster + Postgres
KUBECONFIG=/tmp/e2e-vcluster-kubeconfig.yaml go test -tags k8s ./e2e/...
```

CI runs the integration and k8s tiers on every PR that touches Go server code.
Note that `scripts/e2e.sh integration` and the CI job both run `./e2e/...` only,
so integration tests under `internal/` never execute anywhere.

---

## Design rules

Six rules explain most of the code. Breaking one is usually the bug.

1. **The provider owns what the provider owns.** Do not cache a balance, a price,
   or a threshold. If you need one, ask.
2. **One cached row, derived not written.** `status` and `reason` come from
   `computeStatus`. Write a latch and recompute; never set a status directly.
3. **The request path reads, it does not call.** The gate does one keyed lookup.
   Every provider call happens in a webhook, a timer, or a card save.
4. **Fail open.** A read error allows. A billing system that breaks under its own
   failures is worse than one that under-collects for an hour.
5. **Each latch has exactly one exit.** Adding a signal means deciding both the
   gate it raises and the signal that lowers it. `AllSignals` exists so a test can
   walk every one and prove it.
6. **The server names the fix; each surface words it.** `action` is the contract.
   No client re-derives a billing rule.

---

## Known gaps

Recorded so nobody rediscovers them.

**Production is not metered or gated.** `BILLING_PROVIDER=noop` and
`BILLING_GATE_ENFORCE=false`. Everything here is inert in production, and no
production account holds a contract.

**Production Novu is unverified.** All seven workflows are active in preview. The
production environment is separate and has not been checked. An inactive workflow
now cancels its job and logs at error level.

**`SignalUncollectible` notifies nobody.** `billingAlert` has no case for it, so
an account written off learns about it by discovering it is suspended.

**One unsourced template variable.** `billing.spend_threshold`'s email references
`{{payload.period}}`, which no event carries, so that line renders blank. Either
the template drops it or the payload grows to carry it. This is a copy decision.

**Organizations per user are still uncapped.** The credit-farming loop is closed by
the per-person grant, but `CreateAccount` still caps only personal accounts
(`HasPersonalAccount`), and `POST /api/v1/accounts` has no rate limiting anywhere in
astro-server. A user can still create organizations without limit; each now
provisions on the credit-free package, or the unlimited one when the creator is
internal.

**An undeployed astro-server is metering into the shared Metronome environment.**
Established 2026-08-14. The preview Metronome environment holds 94 customers
against 56 preview accounts. Customer `saswat` (`65ee41d0`, external id
`4156d7cf`) is unarchived and was still accruing compute usage that day, but a
scan of all 61 tables in the preview database finds no reference to either of its
ingest aliases.

It is not preview and not dev: dev holds no Metronome customers, and production
runs `BILLING_PROVIDER=noop`. The daily series settles it. Deployed accounts accrue
a flat rate, around 25 CU a day for `saswatds` and 52 for `matt`, because a cluster
runs continuously. The orphan is intermittent and spiky: nothing for six days, then
162.7, then nothing, then 88.0. That is a machine being switched on and off.

The likely cause is an astro-server running outside the clusters with
`BILLING_PROVIDER=metronome` and the shared API key, metering deployments out of a
local database. Unattributable usage is 519.6 CU, 8.1% of real compute usage in the
window, and 38 of the 94 customers match no live account.

Nothing bills today, so this costs nothing yet. It matters before production for
two reasons: usage attributed to a customer no account maps to is revenue that
reaches no invoice, and a local server holding a production key would write into
real billing data. Give local development its own Metronome environment, or leave
`BILLING_PROVIDER` unset outside the clusters.

**A gateway request with no cost attribute is unbilled.** See
[Pipeline 2](#pipeline-2-llm-tokens-from-astro-infra).

**The WorkOS manager lookup is unproven live.** Every live notification resolved
through the fallback, because a throwaway account created in Postgres has no
WorkOS organization. For a real organization the primary path is WorkOS.

**Nothing has entered through the app.** Every live proof drove the provider API
directly. No run has attached a card through Stripe Elements, generated an
invoice from metered usage, or observed the banner or the 402 in a browser.

**Chat and the messaging proxy are gated.** Non-GET on both is refused for a
suspended account. Those are inbound requests to a running agent, so the effect
lands on the agent's end users rather than the account that missed a payment.
Worth confirming this is intended.
