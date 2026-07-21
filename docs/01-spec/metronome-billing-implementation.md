# Metronome Billing — Implementation Reference (astro-server)

As-built implementation of billing in `apps/astro-server`, behind a
provider-neutral seam. Companion to the product spec
[`metronome-billing-spec.md`](./metronome-billing-spec.md), the code-level flow
doc [`../03-architecture/billing-data-flow.md`](../03-architecture/billing-data-flow.md),
and the forward gating plan [`../06-plan/billing-gating-plan.md`](../06-plan/billing-gating-plan.md).

SDK calls are the official Go SDK (`github.com/Metronome-Industries/metronome-go`, module major **v3**) and the Stripe Go SDK (`github.com/stripe/stripe-go/v82`), with field names verified against the SDK param structs.

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
    noop/           OSS: discard usage, empty reads
    metronome/      hosted: Go SDK v3 impl (+ Stripe linkage)
    metering/       CU-hour math, deployment_billing_state, BillingStateManager
  payment/
    payment.go      Provider interface (card vault), Card type
    stripe.go       Stripe SDK v82 impl (SetupIntent, default card, detach)
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

The provider seam meters and reads back; it does **not** gate. Consumption gating is a separate layer that reads a cached `account_billing_status` row (`active | past_due | suspended`) written off-path — see [`../06-plan/billing-gating-plan.md`](../06-plan/billing-gating-plan.md). Status is planned; the seam and metering below are shipped.

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

Stripe collects and saves a card; astro-server never charges. Enabled only when `STRIPE_SECRET_KEY` is set; otherwise the UI shows "coming soon". No Stripe webhook — the confirm is synchronous and authoritative.

1. `POST …/billing/setup-intent` — ensure a Stripe customer (persisted on `accounts.stripe_customer_id`), return a SetupIntent client secret + publishable key.
2. Client confirms the card with Stripe's embedded `PaymentElement` (name/address collected client-side; card never touches our server).
3. `POST …/billing/payment-method` — server **re-reads** the SetupIntent from Stripe (SDK `V1SetupIntents.Retrieve`), verifies `succeeded`, sets the card as the customer default, detaches prior cards, and links the Stripe customer to Metronome via `LinkStripeCustomer`.
4. `GET`/`DELETE …/billing/payment-method` read/detach the saved card.

Saving a card lets Metronome auto-charge (`charge_automatically`); whether an account is out of funds and what to do about it is Metronome's call, surfaced to gating via webhook signals — not something astro-server derives from card presence.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/billing/usage` | metered usage over `[from,to)` (defaults to current month) |
| GET | `/billing/invoices` | invoices + line items |
| GET | `/billing/invoices/:invoiceId/pdf` | invoice PDF stream |
| GET | `/billing/balances` | credits + commits |
| POST | `/billing/setup-intent` | start card setup |
| POST | `/billing/payment-method` | confirm + save card |
| GET | `/billing/payment-method` | saved card summary |
| DELETE | `/billing/payment-method` | remove card |
| POST | `/webhooks/metronome` | inbound Metronome events (HMAC-verified) |

Read endpoints return `{available:false}` when the backend is `noop`/unconfigured, so the client renders a "not available" state rather than an error.

## Webhooks (hosted only)

`POST /webhooks/metronome` verifies HMAC-SHA256 over `Metronome-Webhook-Date + "\n" + rawBody` (keyed by `METRONOME_WEBHOOK_SECRET`, hex-compared to `Metronome-Webhook-Signature`; raw body read before JSON middleware). Disabled (404) when the secret is unset. Event handlers are **log-only stubs today**:

- `invoice.finalized` → (planned) fetch line items, reconcile.
- `payment.failed` / `invoice.payment_failed` → (planned) map Metronome customer → account (`GetByMetronomeCustomerID`), set `dunning_since`, move `billing_status` → `past_due`; recovery clears it.
- `alert.threshold_reached` → (planned) low-balance banner.

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
| `QUOTA_ENFORCE` | enable DB-quota blocking (default false) |
| `QUOTA_DEFAULTS` | system-wide default limits (`agents=10,members=5,…`) |
| `BILLING_GATE_ENFORCE` | consumption gate: observe vs enforce *(planned)* |
| `BILLING_DUNNING_GRACE_DAYS` | `past_due` → `suspended` window, default 7 *(planned)* |

Account columns (as-built): `metronome_customer_id`, `stripe_customer_id` (each with dedicated `Get/Set` accessors; the metronome customer-backfill worker is provider-generic). **Planned for gating:** a separate `account_billing_status` table (one row per account, `status`/`reason`/`dunning_since`/`alert_active`; absence ⇒ `active`; no balance column) — kept off `accounts` since billing state churns independently. See the [gating plan](../06-plan/billing-gating-plan.md).

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
- **Metronome alert config** — which balance/spend condition Metronome is configured to fire `alert.threshold_reached` on (drives the `suspended` transition); defined Metronome-side, confirmed with the billing owner.
- **Unverified API details** to confirm against the SDK before coding: the recovery event type(s) that clear dunning; exact `AccessSchedule`/`InvoiceSchedule` sub-fields on commits.
