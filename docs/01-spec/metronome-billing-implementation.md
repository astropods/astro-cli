# Metronome Billing — Implementation Reference (astro-server)

As-built implementation of billing in `apps/astro-server`, behind a
provider-neutral seam. Companion to the product spec
[`metronome-billing-spec.md`](./metronome-billing-spec.md), the code-level flow
doc [`../03-architecture/billing-data-flow.md`](../03-architecture/billing-data-flow.md),
and the forward gating plan [`../06-plan/billing-gating-plan.md`](../06-plan/billing-gating-plan.md).

SDK calls are the official Go SDK (`github.com/Metronome-Industries/metronome-go`, module major **v3**) and the Stripe Go SDK (`github.com/stripe/stripe-go/v86`, API version `2026-06-24.dahlia`), with field names verified against the SDK param structs.

## Decisions (locked)

- **Two backends behind one seam.** `BILLING_PROVIDER=noop|metronome` selects at startup. OSS/self-hosted runs `noop` (unmetered — no collection, no gating); hosted runs `metronome`. OpenMeter is fully removed (code, infra, queen surface, DB column).
- **Quota is separate from billing.** Per-account resource limits (agents, deployments, members, stores, endpoints, builds/period) live in a DB-backed `quota.Checker`, enforced identically for OSS and hosted — no billing dependency. **No plans**: system-wide defaults + per-account overrides only.
- **Metering only on the request path is forbidden.** The provider is called from background workers (heartbeat, webhooks, backfill) and non-critical read endpoints — never inline on a gated request. Consumption gating reads a **cached** `account_billing_status` row, not the provider (see [gating plan](../06-plan/billing-gating-plan.md)).
- **Compute is metered in CU-hours** (not per-request): `max(cpuCores, memGB/2) × replicas × hours`; the compute billable metric is a `SUM` over `cu_hours`.
- **Denomination is entirely Metronome-side.** The server only emits `cu_hours`; rating, balances, and any credit unit are Metronome's concern — astro-server neither creates, reads, nor interprets them.
- **Stripe is a card vault, not a charger.** astro-server collects/saves a card (SetupIntent) behind a separate `internal/payment` seam and links the Stripe customer to Metronome (`charge_automatically`); Metronome does the charging.

## Package layout

```
apps/astro-server/internal/
  quota/            per-account limits (Checker, Wrap middleware) — no billing dependency
  billing/
    provider.go     BillingProvider interface, UsageEvent/Account types, sentinel errors
    status.go       cached account_billing_status + pure state machine (incl. force-suspend write-off)
    signal.go       provider-neutral Signal + ApplySignal (webhook signal → status writes + recompute)
    noop/           OSS: discard usage, empty reads
    metronome/      hosted: Go SDK v3 impl (+ Stripe linkage)
    metering/       CU-hour math, deployment_billing_state, BillingStateManager
  payment/
    payment.go      Provider interface (card vault), Card type
    stripe.go       Stripe SDK v86 impl (SetupIntent, default card, detach)
```

## Provider selection & client init

`main.go` builds one `billing.BillingProvider` from `cfg.BillingBackend()` and threads it into the API server and the worker. `metronome.New` returns `nil` when `METRONOME_API_KEY` is unset, so a misconfigured hosted deploy degrades to "billing disabled" rather than panicking; `noop` is the default.

```go
mc := metronome.NewClient(option.WithBearerToken(cfg.MetronomeAPIKey))
```

Built-in retries (408/409/429/≥500, exponential backoff). Optional primitive fields are `param.Opt[T]` set via `metronome.String()` etc.; required fields are plain.

## Two independent guards

Billing and quota are orthogonal 402 sources.

### Quota — per-account counts (DB, always on, OSS + hosted)

```go
package quota

type Result struct {
    Blocked  bool
    Resource string // first resource over limit
    Limit    int64  // effective limit; 0 = disabled
    Used     int64
}

type Checker interface {
    Check(ctx context.Context, accountID string, resources ...string) (Result, error)
}
```

Resources: `agents`, `agent_builds` (per period), `agent_deployments`, `members`, `knowledge_stores`, `knowledge_endpoints`. `quota.Wrap(handler, "agents", …)` gates the route:
- Over limit → 402 `ENTITLEMENT_LIMIT_REACHED` (`usage`/`limit` from the result).
- Effective limit `0` → 402 `FEATURE_NOT_IN_PLAN` (feature switched off for the account).

Enforcement respects `QUOTA_ENFORCE`; a disabled feature (limit 0) always blocks.

### Billing — metering + consumption gate (hosted only)

The provider seam meters and reads back; it does **not** gate. Consumption gating is a separate layer that reads a cached `account_billing_status` row (`active | past_due | suspended`) written off-path — see [`../06-plan/billing-gating-plan.md`](../06-plan/billing-gating-plan.md). Gating is shipped and proven end to end on preview: alert → webhook → signal → `billing.suspend` → replicas to zero, and back.

## `BillingProvider` interface

```go
package billing

type UsageEvent struct {
    TransactionID string         // idempotency key; reuse the event UUID (34-day dedupe)
    AccountID     string         // → Metronome ingest alias
    Type          string         // event_type (deployment_compute_usage, …)
    Time          time.Time
    Properties    map[string]any // cu_hours, component, cpu, memory, replicas, …
}

type Account struct{ ID, Name, Type, OwnerEmail string }

type BillingProvider interface {
    // Customer lifecycle + usage ingest.
    CreateCustomer(ctx context.Context, a Account) (customerID string, err error)
    DeleteCustomer(ctx context.Context, customerID string) error
    IngestUsage(ctx context.Context, events []UsageEvent) error

    // Read-back for the native Billing UI. Return ErrBillingUnavailable on noop.
    UsageData(ctx context.Context, customerID string, from, to time.Time) (any, error)
    Invoices(ctx context.Context, customerID string) (any, error)
    InvoicePDF(ctx context.Context, customerID, invoiceID string) (io.ReadCloser, error)
    Balances(ctx context.Context, customerID string) (any, error)
}
```

- `noop` returns empty customer IDs, discards usage, and returns `ErrBillingUnavailable` from every read.
- `metronome` implements all methods via the SDK, plus a `LinkStripeCustomer(ctx, metronomeCustomerID, stripeCustomerID)` method surfaced through an optional interface assertion (so the core interface stays metering + read-back only).

Packaging/contracts, credit grants, commits, and spend caps are provisioned out-of-band (Metronome admin / Terraform). `Balances`/`UsageData` are read-only pass-throughs for rendering — astro-server never interprets them for gating. The consumption gate reacts to Metronome **webhook signals** (payment failure, threshold/spend alert), not to any balance the server reads; balance/credit math is Metronome's alone. See the [gating plan](../06-plan/billing-gating-plan.md).

### Meters → quota vs billing

| Meter | Aggregation | Guard | Notes |
|---|---|---|---|
| `deployment_compute_usage` (CU-hours) | sum | **Billing** | the only meter emitted today |
| `knowledge_storage` (GB-day) | sum over window | **Billing** | dormant (builder present, call sites disabled) |
| `knowledge_compute` (CU-hours) | sum | **Billing** | dormant |
| `ai_tokens_in` / `ai_tokens_out` (by model) | sum | **Billing** | not emitted by astro-server yet (LiteLLM redirect, future) |
| `agents` / `agent_builds` / `agent_deployments` / `members` / `knowledge_stores` / `knowledge_endpoints` | count / max | **Quota** | DB-only, no billing dependency |

## Exact SDK calls

### `IngestUsage` — `mc.V1.Usage.Ingest`

```go
usage := make([]metronome.V1UsageIngestParamsUsage, len(events))
for i, ev := range events {
    usage[i] = metronome.V1UsageIngestParamsUsage{
        TransactionID: ev.TransactionID,                  // event UUID → 34-day dedupe
        CustomerID:    ev.AccountID,                       // ingest alias = Astro account ID
        EventType:     ev.Type,                            // "deployment_compute_usage"
        Timestamp:     ev.Time.UTC().Format(time.RFC3339), // RFC 3339, 4-digit year
        Properties:    ev.Properties,
    }
}
err := mc.V1.Usage.Ingest(ctx, metronome.V1UsageIngestParams{Usage: usage})
```

Chunked at **100 events/request** (`ingestBatchLimit`). `transaction_id`/`customer_id`/`event_type`/`timestamp` are required.

### `CreateCustomer` — `mc.V1.Customers.New`

```go
resp, err := mc.V1.Customers.New(ctx, metronome.V1CustomerNewParams{
    Name:          a.Name,
    IngestAliases: []string{a.ID}, // usage keyed by Astro account ID
})
// resp.Data.ID → accounts.metronome_customer_id
```

### `LinkStripeCustomer` — `mc.V1.Customers.BillingConfig.New`

```go
err := mc.V1.Customers.BillingConfig.New(ctx, metronome.V1CustomerBillingConfigNewParams{
    CustomerID:                metronomeCustomerID,
    BillingProviderType:       metronome.V1CustomerBillingConfigNewParamsBillingProviderTypeStripe,
    BillingProviderCustomerID: stripeCustomerID,
    StripeCollectionMethod:    metronome.V1CustomerBillingConfigNewParamsStripeCollectionMethodChargeAutomatically,
})
```

### `DeleteCustomer` — `mc.V1.Customers.Archive` (no hard delete)

```go
_, err := mc.V1.Customers.Archive(ctx, metronome.V1CustomerArchiveParams{ID: shared.IDParam{ID: id}})
```

### Read-back — `Usage.ListAutoPaging` · `Customers.Invoices` · `Customers.Credits`/`Commits`

`UsageData` pages `mc.V1.Usage.ListAutoPaging` (daily windows). `Invoices`/`InvoicePDF` use `mc.V1.Customers.Invoices.{ListAutoPaging,GetPdf}` (404 PDF → `ErrInvoiceNotAvailable`). `Balances` returns `{credits, commits}` from `mc.V1.Customers.Credits`/`Commits.ListAutoPaging`. All pass raw SDK rows through for the client to render.

## Ingest event catalog (as-built)

Every event serializes to one envelope; only `event_type` and `properties` vary. Built by `usageEvent()` (`internal/billing/metering`) which stamps a fresh UUID, the account ID, and a UTC timestamp. **Only `deployment_compute_usage` is emitted today.**

```json
{
  "transaction_id": "9f1c8e2a-3b7d-4a11-9c2e-8d5f4b0a1e77",
  "customer_id": "acct_2h4Kd9",
  "event_type": "deployment_compute_usage",
  "timestamp": "2026-07-17T12:35:00Z",
  "properties": {
    "cu_hours": 0.08333333333333333,
    "agent_name": "support-bot",
    "deployment_id": "dep_7Qk2mZ",
    "component": "server",
    "cpu": "500m",
    "memory": "1Gi",
    "replicas": 2
  }
}
```

- Emitted per active workload per heartbeat. `cu_hours = CU × intervalHours`; the `0.0833…` is `1 CU × (5/60) h`.
- `cpu`/`memory` are raw K8s quantity **strings**; `replicas` is an int.
- Request body is `{ "usage": [ <event>, … ] }`, ≤ 100 events per POST.
- Knowledge metering (`knowledge_storage_provisioned`, `knowledge_compute_usage`) is dormant — builders exist, call sites disabled. When re-enabled, billable-metric `EventTypeFilter`s must match those exact strings.

## Metering path

Deployment lifecycle transitions (deploy/undeploy/wakeup) write **anchor rows** to `deployment_billing_state` via `BillingStateManager` — no events at start/stop. A periodic river job **`metering.heartbeat`** (`riverqueue/heartbeat.go`, 5-minute period) runs `BillingStateManager.RunBillingCycle`, computes CU-hour deltas per active workload, and pushes them through `provider.IngestUsage`. Anchor timestamps advance only after ingest succeeds, so a failed cycle re-emits the same delta (idempotent via the event UUID). See [billing-data-flow.md](../03-architecture/billing-data-flow.md) Flow 3.

## Payment method collection (Stripe card vault)

Stripe collects and saves a card; astro-server never charges. Enabled only when `STRIPE_SECRET_KEY` is set; otherwise the UI shows "coming soon". The card-save **confirm** is synchronous and authoritative (no webhook needed for it); payment-**collection** lifecycle (charge failures, 3DS, uncollectible/void) is a separate inbound Stripe webhook — see [Webhooks](#webhooks-hosted-only).

1. `POST …/billing/setup-intent` — ensure a Stripe customer (persisted on `accounts.stripe_customer_id`), return a SetupIntent client secret + publishable key.
2. Client confirms the card with Stripe's embedded `PaymentElement` (name/address collected client-side; card never touches our server).
3. `POST …/billing/payment-method` — server **re-reads** the SetupIntent from Stripe (SDK `V1SetupIntents.Retrieve`), verifies `succeeded`, sets the card as the customer default, detaches prior cards, and links the Stripe customer to Metronome via `LinkStripeCustomer`.
4. `GET`/`DELETE …/billing/payment-method` read/detach the saved card.

Saving a card lets Metronome auto-charge (`charge_automatically`); whether an account is out of funds and what to do about it is Metronome's call, surfaced to gating via webhook signals — not something astro-server derives from card presence. The card-save **confirm** stays synchronous; the **collection** lifecycle arrives on the separate Stripe webhook (below).

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/billing/usage` | metered usage over `[from,to)` (defaults to current month) |
| GET | `/billing/usage/daily-spend` | rated spend per day over `[from,to)`, from the invoice breakdown |
| GET | `/billing/invoices` | invoices + line items |
| GET | `/billing/invoices/:invoiceId/pdf` | invoice PDF stream |
| POST | `/billing/setup-intent` | start card setup |
| POST | `/billing/payment-method` | confirm + save card |
| GET | `/billing/payment-method` | saved card summary |
| DELETE | `/billing/payment-method` | remove card |
| POST | `/webhooks/metronome` | inbound Metronome events (HMAC-verified → River job) |
| POST | `/webhooks/stripe` | inbound Stripe payment-collection events (signature-verified → River job) |

Read endpoints return `{available:false}` when the backend is `noop`/unconfigured, so the client renders a "not available" state rather than an error.

## Webhooks (hosted only)

Two independent inbound sources, no overlap. **Metronome** delivers contract/invoice/alert lifecycle; **Stripe** delivers payment-collection lifecycle. Metronome explicitly does **not** relay Stripe payment events (*"no Metronome webhook is triggered for errors residing entirely within Stripe, such as payment failures"*), so dunning/collection state must come from Stripe directly — payment failure, 3DS, uncollectible, and void have no Metronome equivalent. Both endpoints verify a signature and 404 when their secret is unset. Sources: [Metronome webhooks](https://docs.metronome.com/guides/platform-configuration/setup-webhooks), [Stripe event types](https://docs.stripe.com/api/events/types).

### Receive → enqueue → process (both sources)

Each handler does the minimum on the request path: **verify the signature, parse a minimal envelope, enqueue a River job, return.** Account mapping, status writes, and workload reconciliation run in the worker — so every accepted event is a tracked, retryable `river_job` row rather than best-effort inline work.

- **Idempotency.** The job args carry the provider **event id**, tagged `river:"unique"`; inserted with `UniqueOpts{ByArgs: true}`. Provider redeliveries of the same event dedupe against non-cleaned jobs (River's default state set includes `completed`), so a redelivery within the retention window is a no-op. **Empty event id ⇒ no dedupe** (safer to double-process — `ApplySignal` is idempotent — than to collapse distinct id-less events into one job).
- **Enqueue failure ⇒ 500.** If the insert fails the handler returns 500 so the provider redelivers — nothing is silently dropped. Likewise, a verified Stripe event whose object fails to parse returns 500 rather than acking an event we couldn't read.
- **Worker error handling.** Unknown customer (`account.ErrAccountNotFound`) or unhandled event type ⇒ permanent no-op (ack, no retry). A **transient DB error** (lookup, status write) is returned so River retries — a dropped `payment_failed`/`uncollectible` would bypass gating.
- **Routes gated on backend.** `/webhooks/{metronome,stripe}` register only for `BILLING_PROVIDER=metronome` (where the workers exist); on other backends they 404 rather than enqueue jobs nothing will drain. Job kinds register unconditionally for the admin catalog.
- **Dedicated `billing` queue.** The whole gating pipeline — `webhook.metronome`, `webhook.stripe`, `billing.suspend`, `billing.resume`, `billing.dunning_sweep` — runs on an isolated River queue (`MaxWorkers: 5`) so a provider webhook burst can't starve the default pool. (`metering.heartbeat` stays on the default queue.)
- **Shared status logic.** Each worker maps its event type → a provider-neutral `billing.Signal`, then calls `billing.ApplySignal` (writes the collection flags, recomputes status). The worker then reconciles workloads to the **current** status on every handled event (not only on a transition) via `billing.suspend`/`billing.resume`, and **returns any enqueue error so River retries** — the only backstop for a dropped *resume* (the dunning sweep re-enqueues suspends only). Suspend/resume are idempotent, so a no-op reconcile is cheap.

```
Stripe/Metronome ──POST──▶ handler (verify sig) ──enqueue──▶ webhook.* job
                                                                  │
                          map customer→account, ApplySignal ◀─────┘
                          (status writes + Recompute)
                                  │ on transition
                                  ▼
                          billing.suspend / billing.resume
```

### `billing.Signal` → status writes

`ApplySignal` (`internal/billing/signal.go`) is the one place event semantics turn into status flags:

| Signal | source events | status writes |
|---|---|---|
| `SignalPaymentFailed` | Stripe `invoice.payment_failed` | `SetDunningSince` → past_due (→ suspended after grace) |
| `SignalActionRequired` | Stripe `invoice.payment_action_required` | `SetDunningSince`; handler logs `hosted_invoice_url` for the 3DS link |
| `SignalAlert` | Metronome `alerts.spend_threshold_reached` | `SetAlert` → suspended (balance_alert) |
| `SignalAlertResolved` | Metronome `alerts.spend_threshold_resolved` | `ClearAlert` only; a write-off or open dunning still outranks it |
| `SignalUncollectible` | Stripe `invoice.marked_uncollectible` | `SetForceSuspend` → suspended immediately (uncollectible), bypassing grace |
| `SignalVoided` | Stripe `invoice.voided` | clear dunning + alert + force-suspend (debt gone) |
| `SignalRecovery` | Stripe `invoice.paid` | clear dunning **only** |
| `SignalCardUpdated` | Stripe `payment_method.automatically_updated` | clear dunning (leave balance alert) |
| `SignalCreditsExhausted` | Metronome `alerts.low_remaining_contract_credit{,_and_commit}_balance_reached` | `SetCreditsExhausted` → suspended (credits_exhausted) while no card |
| `SignalCreditsGranted` | Metronome `..._resolved`, or the provisioning job | `ClearCreditsExhausted` |
| `SignalCardAdded` / `SignalCardRemoved` | Stripe `payment_method.attached` / `detached` | `SetPaymentMethod`; exhaustion stops/resumes gating |

Payment failure/recovery are Stripe-only — Metronome relays no payment events. Metronome's status-changing signals are the spend-threshold alert and the contract-credit balance alert, in both directions. All other Metronome alerts (usage/commit/invoice-total) are UI banners, not gating signals, and stay unhandled.

**Every flag is two-way.** A gate that only sets is a gate only an operator can lift, which is how the forced provisioning re-run came to exist. `dunning_since` clears on recovery/card-update/void, `force_suspended` on void, `alert_active` on the alert's own resolved event or a void, `credits_exhausted` on the credit alert's resolved event or a grant.

`invoice.marked_uncollectible` uses a `force_suspended` flag on `account_billing_status` and reason `uncollectible` — the state machine's highest-priority rule (before alert and dunning-grace). Cleared on void, not on an unrelated payment: the write-off is terminal until the debt itself is resolved.

### `POST /webhooks/metronome`

HMAC-SHA256 over `Metronome-Webhook-Date + "\n" + rawBody` (keyed by `METRONOME_WEBHOOK_SECRET`, hex-compared to `Metronome-Webhook-Signature`; raw body read before JSON middleware). Enqueues `webhook.metronome`; the worker maps via `GetByMetronomeCustomerID`. Full catalog per [Metronome docs](https://docs.metronome.com/guides/platform-configuration/setup-webhooks); the `—` action means received-but-not-consumed.

> Two conditions gate: the contract-credit balance hitting zero (the free-tier floor, which only bites without a card) and the spend threshold (a backstop against runaway spend on a card). Resolved notifications are enabled per account and cover every threshold type at once, so each `_reached` has a `_resolved` twin. Payment failure/recovery are Stripe concerns — see the Stripe endpoint.

| Metronome event | fires when | astro-server action |
|---|---|---|
| `alerts.spend_threshold_reached` | spend exceeds configured limit | `SetAlert` → `suspended` |
| `alerts.spend_threshold_resolved` | spend back under the limit (period rollover) | `ClearAlert` |
| `alerts.low_remaining_contract_credit_balance_reached` | contract credit spent | `SetCreditsExhausted` → `suspended` while no card |
| `alerts.low_remaining_contract_credit_balance_resolved` | credit balance restored | `ClearCreditsExhausted` |
| `alerts.usage_threshold_reached` | billable-metric usage over threshold | near-cap banner |
| `alerts.low_remaining_credit_balance_reached` | prepaid credit balance low | top-up banner |
| `alerts.low_remaining_commit_balance_reached` | commit balance low | top-up banner |
| `alerts.invoice_total_reached` | invoice total over threshold | informational banner |
| `invoice.finalized` | invoice finalized after grace | fetch line items, reconcile *(planned)* |
| `invoice.billing_provider_error` | error sending invoice to Stripe (no customer / no valid PM) | alert billing owner, surface banner |
| `payment_gate.payment_pending_action_required` | additional payment action needed (3DS) | surface action link — Metronome analog of Stripe `invoice.payment_action_required` |
| `payment_gate.payment_status` | payment attempt status update | consume only if payment gating enabled Metronome-side |
| `payment_gate.threshold_reached` | payment-gating threshold triggered | consume only if payment gating enabled |
| `payment_gate.external_initiate` | payment via external gateway | consume only if payment gating enabled |
| `integration.issue` | third-party integration error | log / alert |
| `contract.create` · `.start` · `.edit` · `.end` · `.archive` | contract lifecycle | — |
| `commit.create` · `.edit` · `.archive` · `.segment.start` · `.segment.end` | commit lifecycle | — |
| `credit.create` · `.edit` · `.archive` · `.segment.start` · `.segment.end` | credit-grant lifecycle | — |
| `marketplaces.aws_metering_disabled` · `.azure_metering_disabled` · `.gcp_metering_disabled` | marketplace customer disabled | — (no marketplace billing) |

### `POST /webhooks/stripe`

The only source of payment-collection state. Verified with `stripe-go/v86/webhook.ConstructEvent` (classic snapshot events — the payload carries `object:"event"` and the full object; not v2 thin event notifications) against the `Stripe-Signature` header, keyed by `STRIPE_WEBHOOK_SECRET`; disabled (404) when unset. The handler reads `data.object.customer` (a plain id string in webhook payloads) and `hosted_invoice_url` from a minimal struct, then enqueues `webhook.stripe`; the worker maps Stripe customer → account via `GetByStripeCustomerID`. astro-server **never charges** — collection and retry stay Stripe/Metronome's; the server only mirrors state into `account_billing_status`. Full catalog per [Stripe event types](https://docs.stripe.com/api/events/types); `—` means not consumed (Metronome/Stripe own that lifecycle). Non-`invoice`/`payment_method` families are collapsed to one row each since none are consumed. `✓` = consumed today.

| Stripe event | fires when | astro-server action |
|---|---|---|
| `invoice.payment_failed` ✓ | auto-charge attempt fails | `SignalPaymentFailed` → `SetDunningSince` → `past_due` |
| `invoice.payment_action_required` ✓ | payment needs 3DS/further action (Stripe does **not** email on `charge_automatically`) | `SignalActionRequired` → keep/raise dunning; `hosted_invoice_url` carried on the job + logged for the in-app 3DS link |
| `invoice.marked_uncollectible` ✓ | invoice marked uncollectible (auto-advance after retries) | `SignalUncollectible` → `SetForceSuspend` → `suspended` (write-off), bypasses grace |
| `invoice.voided` ✓ | invoice voided | `SignalVoided` → clear dunning + alert + force-suspend |
| `invoice.paid` ✓ | payment attempt succeeds / marked paid out-of-band | `SignalRecovery` → clear dunning + alert |
| `payment_method.automatically_updated` ✓ | card network auto-updates an expired card | `SignalCardUpdated` → clear dunning; retry-charge is Stripe/Metronome-side |
| `invoice.payment_succeeded` | payment attempt succeeds | — (overlaps `invoice.paid`; we consume `invoice.paid` only, not both) |
| `invoice.finalization_failed` | draft invoice cannot be finalized | — *(candidate: alert billing owner)* |
| `invoice.overdue` | due date passed by configured days | — *(candidate: escalate dunning)* |
| `invoice.will_be_due` | N days before due | — *(candidate: pre-dunning reminder)* |
| `invoice.created` · `.updated` · `.deleted` · `.finalized` · `.sent` · `.upcoming` · `.overpaid` | invoice lifecycle owned by Metronome | — |
| `payment_method.attached` · `.updated` · `.detached` | payment-method CRUD (handled synchronously by the card vault) | — |
| `charge.*` (`succeeded`, `failed`, `refunded`, `dispute.*`, …) | charge lifecycle | — (invoice-level events drive state; disputes may be a future signal) |
| `payment_intent.*` (`succeeded`, `payment_failed`, `requires_action`, …) | PaymentIntent lifecycle | — (subsumed by invoice events) |
| `customer.subscription.*`, `customer.source.*` | subscription / source lifecycle | — (Metronome owns subscriptions; card vault owns sources) |

## Packaging → Metronome primitives (out-of-band)

Products, billable metrics, rate cards, commits, and credit grants are configured in the Metronome dashboard / Terraform per environment, not by astro-server. The billable metrics must filter/aggregate on the emitted event shape (e.g. compute filters `event_type = deployment_compute_usage`, sums `cu_hours`).

| Model | Primitive |
|---|---|
| Pay as you go | standard rate card, no commit, monthly arrears; free/starting credits via credit grants |
| Enterprise committed | `Commit{Postpaid|Prepaid}` + per-contract rate overrides; optional spend threshold |
| Prepaid credits | `Commit{Prepaid}`, invoiced up front; overages-off ⇒ hard stop via balance threshold |
| Subscription | recurring charges (platform fee, per-seat) ± bundled recurring credits |

## Configuration & data model

| Var | Purpose |
|---|---|
| `BILLING_PROVIDER` | `noop` (default) \| `metronome` |
| `METRONOME_API_KEY` | SDK bearer token (metronome provider is nil without it) |
| `METRONOME_WEBHOOK_SECRET` | Metronome webhook HMAC verification |
| `STRIPE_SECRET_KEY` | enables the Stripe card vault |
| `STRIPE_PUBLISHABLE_KEY` | surfaced to the client for the embedded PaymentElement |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signature verification (payment-collection events); endpoint 404s when unset |
| `QUOTA_ENFORCE` | enable DB-quota blocking (default false) |
| `QUOTA_DEFAULTS` | system-wide default limits (`agents=10,members=5,…`) |
| `BILLING_GATE_ENFORCE` | consumption gate: observe vs enforce *(planned)* |
| `BILLING_DUNNING_GRACE_DAYS` | `past_due` → `suspended` window, default 7 *(planned)* |

Account columns (as-built): `metronome_customer_id`, `stripe_customer_id` (each with dedicated `Get/Set` accessors + a `GetBy…CustomerID` reverse lookup used by the webhook workers; the metronome customer-backfill worker is provider-generic). Gating state lives in a separate `account_billing_status` table (one row per account, `status`/`reason`/`dunning_since`/`alert_active`/`force_suspended`; absence ⇒ `active`; no balance column) — kept off `accounts` since billing state churns independently. `force_suspended` is the write-off flag set by `invoice.marked_uncollectible`. See the [gating plan](../06-plan/billing-gating-plan.md).

Quota storage (no plans): system-wide defaults in config; per-account overrides in a narrow table, admin-editable via `astro-queen`.

```sql
CREATE TABLE account_limits (
    account_id  text   NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    resource    text   NOT NULL,   -- agents, agent_deployments, members, …
    limit_value bigint NOT NULL,   -- 0 = disabled; -1 = unlimited
    PRIMARY KEY (account_id, resource)
);
```

Effective limit = `override(account, resource)` else config default.

## Remaining work

- **Consumption gating** — cached `billing_status`, periodic status job, webhook writes, `middleware.Entitlements` reader + `ent.Wrap` on deploy, workload suspend/resume. Full plan: [`../06-plan/billing-gating-plan.md`](../06-plan/billing-gating-plan.md).
- **AI-token metering** — repoint the LiteLLM proxy fleet's token/spend emission to Metronome; land `ai_tokens_in`/`ai_tokens_out` end to end, keyed on the account ID.
- **Knowledge metering** — re-enable the dormant `knowledge_storage`/`knowledge_compute` emit paths.

> Rating, credits, balances, and the pricing unit are **Metronome/product concerns, not astro-server work** — the server emits usage and reacts to Metronome's webhook signals; it does not model money.

## Open questions

- **Provisioning surface** — enterprise/prepaid/subscription setup in `astro-queen` vs self-serve prepaid top-ups.
- **Spend-threshold event name** — only `low_remaining_contract_credit_and_commit_balance_resolved` is documented by name; the rest are inferred from the enum's `<alert_type>_resolved` form. An inferred name that misses falls through to the unhandled-event log, which leaves the latch set, so confirm the real names from the first live event before relying on the spend gate.
- **3DS link surfacing** — `invoice.payment_action_required` carries `hosted_invoice_url` on the job and logs it today; the in-app surface (banner/email from the pay-link) is not yet built.
- **Unverified API details** to confirm against the SDKs: Metronome `payment_gate.*` payloads; exact `AccessSchedule`/`InvoiceSchedule` sub-fields on commits. (Stripe `hosted_invoice_url`/`customer` paths and `webhook.ConstructEvent` are verified as-built.)
