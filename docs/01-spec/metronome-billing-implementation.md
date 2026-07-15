# Metronome Billing — Implementation Plan (astro-server)

Implementation companion to [`metronome-billing-spec.md`](./metronome-billing-spec.md). Scope: replace OpenMeter with Metronome in `apps/astro-server` behind a provider interface, and **fully decommission OpenMeter**. Every call below is the literal official Go SDK method (`github.com/Metronome-Industries/metronome-go`, module major **v3**), with field names verified against the SDK param structs.

## Decisions (locked)

- **OSS/self-hosted → no-op provider.** OpenMeter is *not* retained. Self-hosted ships a `noop` `BillingProvider`: unmetered, no gating, no collection. Metronome is hosted-only.
- **Interface seam.** Extract `BillingProvider`; OpenMeter becomes a transitional impl behind it, deleted after cutover.
- **astro-server first.** LiteLLM token redirect, `astro-queen`, and infra teardown are follow-up phases (§9).
- **Quota is separate from billing.** Per-account resource limits (agents, deployments, members, stores, endpoints, builds/period) live in their own DB-backed `quota.Checker`, enforced identically for OSS and hosted — no billing dependency. **No plans**: system-wide defaults + per-account overrides only. The billing provider (Metronome/noop) handles metering + balance/spend gating. The two are independent 402 sources; today's OpenMeter entitlement path conflated them.
- **Compute is metered in CU-hours** (not per-request). The existing `rawCU`/`knowledgeCU` math (`max(cpuCores, memGB/2)*replicas`) is unchanged; the compute billable metric is a `SUM` over `compute_unit_hours`.
- **Denomination: start in USD, add the credit unit later.** v1 rates the single balance, commits, and grants in **USD** (Metronome's native USD-cents credit type). The Astro credit (`1 credit = $0.001`) is a later phase (Phase 8): introduce the `ASTRO_CREDIT` custom pricing unit and convert, or keep USD and render credits in the UI. This keeps the unverified custom-pricing-unit creation off the critical path; the single-balance model (all meters draw from one balance) is identical either way.
- **Official Go SDK, not hand-rolled HTTP.**

## Package layout

```
apps/astro-server/internal/quota/       per-account limits — no billing dependency
  quota.go           Checker, Wrap() middleware, resource→limit resolution
apps/astro-server/internal/billing/
  provider.go        BillingProvider + HostedBilling interfaces, shared types
  noop/              OSS: allow-all balance, discard usage, no customers
  metronome/         hosted: SDK-backed
  openmeter/         MOVED from internal/openmeter — transitional; deleted in Phase 4
```

`BILLING_PROVIDER=noop|metronome|openmeter` selects at startup (default flips `openmeter`→`noop` after cutover). Today's "nil client ⇒ no-op" pattern becomes the `noop` impl, collapsing every existing nil-guard into a real interface value.

## Client init

```go
import (
    "github.com/Metronome-Industries/metronome-go"
    "github.com/Metronome-Industries/metronome-go/option"
    "github.com/Metronome-Industries/metronome-go/shared"
)

mc := metronome.NewClient(option.WithBearerToken(cfg.MetronomeAPIKey)) // or env METRONOME_BEARER_TOKEN
```

Built-in retries (408/409/429/≥500, exponential backoff). Optional primitive fields are wrapped `param.Opt[T]`, set via `metronome.String()`, `metronome.Bool()`, `metronome.Time()`, etc.; required fields are plain.

## The two interfaces

Today's single OpenMeter-driven `middleware/entitlement.go` splits into two independent guards.

### Quota — per-account limits (no billing dependency)

```go
package quota

// Effective limit resolution: per-account override (account_limits table) → system default (config).
// Current counts come from existing DB queries (the same COUNTs the emit helpers use).
type Result struct {
    Blocked  bool
    Resource string // first resource over limit
    Limit    int64  // effective limit; 0 = unavailable
    Used     int64
}

type Checker interface {
    Check(ctx context.Context, accountID string, resources ...string) (Result, error)
}
```

Resources: `agents`, `agent_builds` (per period), `agent_deployments`, `members`, `knowledge_stores`, `knowledge_endpoints`. Enforced for OSS and hosted alike. `quota.Wrap(handler, "agents", "agent_builds")` replaces the resource-count half of today's `ent.Wrap`.

### Billing — metering + balance gate (Metronome / noop)

```go
package billing

type UsageEvent struct {
    TransactionID string         // idempotency; reuse the CloudEvent UUID
    AccountID     string         // → Metronome ingest alias
    Type          string         // event_type (compute_usage, ai_tokens_in, …)
    Time          time.Time
    Properties    map[string]any // compute_unit_hours, model, component, …
}

// Balance/spend gate only — NOT resource counts (those are quota's job).
type Balance struct {
    Allow        bool    // false when prepaid-overages-off and balance ≤ 0, or over spend cap
    RemainingUSD float64 // v1 is USD-denominated; credits are a later conversion
}

type Account struct{ ID, Name, Type, OwnerEmail string }

// Implemented by noop, metronome, openmeter.
type BillingProvider interface {
    CreateCustomer(ctx context.Context, a Account) (customerID string, err error)
    UpdateCustomer(ctx context.Context, customerID, name string) error
    DeleteCustomer(ctx context.Context, customerID string) error
    IngestUsage(ctx context.Context, events []UsageEvent) error
    CheckBalance(ctx context.Context, customerID string) (Balance, error)
    GetUsage(ctx context.Context, customerID string, from, to time.Time) (UsageReport, error)
}

// Metronome-only; noop returns ErrUnsupported.
type HostedBilling interface {
    GrantCredits(ctx context.Context, customerID string, usd float64, expiry time.Time, reason string) error
    ProvisionPackaging(ctx context.Context, customerID string, plan PackagingPlan) error
}
```

`noop.CheckBalance` always returns `{Allow: true}` — quotas still apply, but consumption is never balance-gated. Paid-consumption paths (deploy compute, knowledge) call `CheckBalance` in addition to their quota check; count-only paths (add member, register agent) call quota alone.

### Current meters → quota vs billing

Today's nine OpenMeter meters (`openmeter.RequiredMeters`) split by aggregation: **count/max** meters are per-account limits → quota; **sum/consumption** meters are metered usage → billing.

| Current meter | Aggregation today | Converts to | Notes |
|---|---|---|---|
| `compute` | sum (CU-hours) | **Billing** | metered → balance; optional spend cap |
| `knowledge_compute` | sum (CU-hours) | **Billing** | metered → balance |
| `knowledge_storage` | sum (GB-day) | **Billing** | metered → balance; no provisioning cap |
| `members` | count | **Quota** | max members / seats. Per-seat subscription pricing (if used) reads the DB member count as a recurring-charge quantity — not a Metronome meter |
| `agents` | count | **Quota** | max registered agents |
| `agent_builds` | count / period | **Quota** | builds per billing period (rate limit, resets each period) |
| `agent_deployments` | count (active) | **Quota** | max active deployments |
| `knowledge_stores` | count | **Quota** | max knowledge stores |
| `knowledge_endpoints` | count | **Quota** | max PrivateLink endpoints |
| `ai_tokens_in` / `ai_tokens_out` *(new)* | sum | **Billing** | metered → balance (added Phase 3/5) |

Net: 6 pure quota, 4 pure billing, no straddlers. The `Billing` rows reach Metronome; the rest are DB-only quota checks with no billing dependency — a clean split.

## Exact SDK calls per interface method

### `IngestUsage` — `mc.V1.Usage.Ingest`

```go
usage := make([]metronome.V1UsageIngestParamsUsage, len(events))
for i, ev := range events {
    usage[i] = metronome.V1UsageIngestParamsUsage{
        TransactionID: ev.TransactionID,                  // = CloudEvent UUID → 34-day dedupe
        CustomerID:    ev.AccountID,                       // ingest alias = Astro account ID
        EventType:     ev.Type,                            // "compute_usage", "ai_tokens_in", …
        Timestamp:     ev.Time.UTC().Format(time.RFC3339), // RFC 3339, 4-digit year
        Properties:    ev.Properties,                      // compute_unit_hours, model, component, …
    }
}
err := mc.V1.Usage.Ingest(ctx, metronome.V1UsageIngestParams{Usage: usage})
```

`V1UsageIngestParams.Usage` is a JSON array; `transaction_id`/`customer_id`/`event_type`/`timestamp` are `api:"required"`. Chunk the slice (batch limit **unverified** — assume ~100/req).

### `CreateCustomer` — `mc.V1.Customers.New` (+ Stripe link)

```go
c, err := mc.V1.Customers.New(ctx, metronome.V1CustomerNewParams{
    Name:          a.Name,
    IngestAliases: []string{a.ID},                        // usage keyed by Astro account ID
    CustomerBillingProviderConfigurations: []metronome.V1CustomerNewParamsCustomerBillingProviderConfiguration{{
        BillingProvider: "stripe",
        DeliveryMethod:  "direct_to_billing_provider",
        Configuration: map[string]any{
            "stripe_customer_id":       stripeCustomerID,
            "stripe_collection_method": "charge_automatically",
        },
    }},
})
// c.Data.ID → store as metronome_customer_id
```

Or link Stripe after creation via `mc.V1.Customers.BillingConfig.New`:

```go
err := mc.V1.Customers.BillingConfig.New(ctx, metronome.V1CustomerBillingConfigNewParams{
    CustomerID:                metronomeCustomerID,
    BillingProviderType:       metronome.V1CustomerBillingConfigNewParamsBillingProviderTypeStripe,
    BillingProviderCustomerID: stripeCustomerID,
    StripeCollectionMethod:    metronome.V1CustomerBillingConfigNewParamsStripeCollectionMethodChargeAutomatically,
})
```

### `UpdateCustomer` — `mc.V1.Customers.SetName`  ·  `DeleteCustomer` — `mc.V1.Customers.Archive`

```go
_, err := mc.V1.Customers.SetName(ctx, metronome.V1CustomerSetNameParams{CustomerID: id, Name: name})
_, err := mc.V1.Customers.Archive(ctx, metronome.V1CustomerArchiveParams{ID: id}) // no hard delete
```

### `CheckBalance` — `mc.V1.Contracts.ListBalances`

```go
bal, err := mc.V1.Contracts.ListBalances(ctx, metronome.V1ContractListBalancesParams{
    CustomerID:          customerID,
    IncludeBalance:      metronome.Bool(true),
    ExcludeZeroBalances: metronome.Bool(true),
    CoveringDate:        metronome.Time(time.Now().UTC()),
})
// Sum remaining commit/credit balance (USD) → Balance.RemainingUSD.
// Prepaid-overages-off: Allow=false when balance ≤ 0. PAYG/enterprise: Allow=true.
```

Returns `*V1ContractListBalancesResponseUnion`. This gates *consumption only*; resource counts are enforced separately by `quota.Checker`. Combine with the contract's `spend_threshold_configuration` for spend caps.

### `GetUsage` — `mc.V1.Customers.ListCosts`

```go
costs := mc.V1.Customers.ListCosts(ctx, metronome.V1CustomerListCostsParams{
    CustomerID:   customerID,
    StartingOn:   from,   // inclusive
    EndingBefore: to,     // exclusive
})
// Auto-paging cursor; pair with ListBalances for remaining credit balance in the UI.
```

### `GrantCredits` — `mc.V1.CreditGrants.New` (v1: denominated in USD cents)

```go
_, err := mc.V1.CreditGrants.New(ctx, metronome.V1CreditGrantNewParams{
    CustomerID: customerID,
    Name:       reason,                 // appears on invoices
    Priority:   1,                      // lower consumes first
    ExpiresAt:  expiry,
    GrantAmount: metronome.V1CreditGrantNewParamsGrantAmount{
        Amount:       usd * 100,         // USD cents (v1); switches to ASTRO_CREDIT later
        CreditTypeID: usdCentsCreditTypeID, // Metronome built-in USD (cents); from /v1/credit-types/list
    },
    PaidAmount: metronome.V1CreditGrantNewParamsPaidAmount{
        Amount:       0,                 // free grant (monthly / 3-mo starting credits)
        CreditTypeID: usdCentsCreditTypeID,
    },
})
```

Free monthly credits = a grant reissued each period (recurring, non-rolling); 3-mo starting credits = one grant with `ExpiresAt = now+3mo`.

## One-time Metronome object setup (per environment)

**Single product, single balance.** Compute, AI tokens, and knowledge storage are *meters*, not separate priced products — each is a billable metric whose usage draws down one balance. v1 denominates that balance in **USD**; the `ASTRO_CREDIT` pricing unit (`$0.001`) is added later (Phase 8), off the critical path.

Provisioned once via script/Terraform + admin console:

| Object | SDK call | Path |
|---|---|---|
| The one product (the single balance) | `mc.V1.Contracts.Products.New` | `POST /v1/contract-pricing/products/create` |
| Billable metrics — one per meter (`compute_usage`, `ai_tokens_in`, `ai_tokens_out`, `knowledge_storage`) | `mc.V1.BillableMetrics.New` | `POST /v1/billable-metrics/create` |
| Rate card `ASTRO_STANDARD` — rates in **USD** | `mc.V1.Contracts.RateCards.New` | `POST /v1/contract-pricing/rate-cards/create` |
| Rates on the card (one per meter) | `mc.V1.Contracts.RateCards.Rates.AddMany` | `POST /v1/contract-pricing/rate-cards/addRates` |

Commits/credit grants apply to the single product, so all metered usage draws from one balance (no per-meter `applicable_product_ids`).

Billable metric example (compute, CU-hours):

```go
_, err := mc.V1.BillableMetrics.New(ctx, metronome.V1BillableMetricNewParams{
    Name:            "Compute (CU-hours)",
    AggregationType: metronome.V1BillableMetricNewParamsAggregationTypeSum,
    AggregationKey:  metronome.String("compute_unit_hours"),
    EventTypeFilter: shared.EventTypeFilterParam{InValues: []string{"compute_usage"}},
    GroupKeys:       [][]string{{"component"}},
})
```

AI tokens: two metrics, `SUM` on the token-count property, `GroupKeys: [][]string{{"model"}}`, filtered to `ai_tokens_in` / `ai_tokens_out`.

## Metering path (unchanged compute, new sink)

CU-hour math, the two billing-state tables, the heartbeat/reconcile state machine, and the inline emit helpers stay. Only the sink changes: `client.IngestEvents([]CloudEvent)` → `provider.IngestUsage([]UsageEvent)`. A thin adapter maps `CloudEvent{ID,Subject,Type,Data}` → `UsageEvent{TransactionID,AccountID,Type,Properties}` — the UUID carries over as `transaction_id`, preserving idempotency and the backfill dedupe.

**New AI-token metric** (astro-server doesn't meter tokens today; LiteLLM emits to OpenMeter directly): add `ai_tokens_in`/`ai_tokens_out` events. The fleet emission redirect is Phase 5 (§9), not astro-server.

## Gating — two independent 402 sources

`middleware/entitlement.go` splits into a quota guard and a balance guard. The existing response codes are preserved so the client needs no change; only their source moves.

**Quota gate (DB, always on, OSS + hosted).** `quota.Check` compares DB counts against the effective limit:
- Over limit → 402 `ENTITLEMENT_LIMIT_REACHED` (`usage`/`limit` from the result).
- Effective limit `0` → 402 `FEATURE_NOT_IN_PLAN` (feature disabled for the account). *(No plans exist; a 0 override is how a feature is switched off.)*

**Balance gate (billing provider, consumption paths only).** `billing.CheckBalance`:
- PAYG / enterprise-overages-on → `Allow=true` (soft-warn near cap via alerts).
- Prepaid-overages-off, balance ≤ 0 → `Allow=false` → 402 (insufficient balance).
- `noop` → always allow.
- Provider error → fail open (preserve current behavior).

Near-cap/low-balance warnings via `mc.V1.Alerts.New` (`POST /v1/alerts/create`) → webhook → in-app banner.

## Packaging → exact primitives

Commits are created inline on `mc.V1.Contracts.New` or standalone via `mc.V1.Customers.Commits.New`:

```go
_, err := mc.V1.Customers.Commits.New(ctx, metronome.V1CustomerCommitNewParams{
    CustomerID:      customerID,
    Type:            metronome.V1CustomerCommitNewParamsTypePrepaid, // or …TypePostpaid
    ProductID:       balanceProductID, // the single product; all meters draw from it
    Priority:        100,
    AccessSchedule:  metronome.V1CustomerCommitNewParamsAccessSchedule{ /* schedule_items: amount (USD cents) + window */ },
    InvoiceSchedule: metronome.V1CustomerCommitNewParamsInvoiceSchedule{ /* charge up front (prepaid) / true-up (postpaid) */ },
})
```

| Model | Primitive |
|---|---|
| Pay as you go | `ASTRO_STANDARD` rate card, no commit, monthly arrears. Free monthly + 3-mo credits via `CreditGrants.New`. |
| Enterprise committed | `Commit{Type: Postpaid}` (or Prepaid) + per-contract rate `overrides` on `Contracts.New`; optional `spend_threshold_configuration`. |
| Prepaid credits | `Commit{Type: Prepaid}` in USD, invoiced up front. Overages-off ⇒ hard stop via payment/balance threshold; auto-recharge via prepaid balance threshold. |
| Subscription | Recurring charges (`platform_fee`, per-`seats`) ± bundled recurring credits, on `Contracts.New`. |

Successor contract: `Contracts.New` with `transition:{type:"renewal", from_contract_id}`. Amendments: `mc.V2.Contracts.Edit` (`POST /v2/contracts/edit`).

## Invoicing & webhooks (hosted only)

New endpoint `POST /api/v1/webhooks/metronome` consuming **invoice finalized**, **payment failed**, **threshold-reached**.

- **Signature:** HMAC-SHA256 over `X-Metronome-Date + "\n" + rawBody`, keyed by `METRONOME_WEBHOOK_SECRET`; hex-compare to header `Metronome-Webhook-Signature`. Use raw bytes (read body before JSON middleware).
- On finalize: `mc.V1.Customers.Invoices.Get` for line items; invoice is USD-native (v1). Once the credit unit lands, credits convert to USD at $0.001/credit and the credit balance is summarized.
- On payment failure: dunning state + gating downgrade; recovery restores.

## Configuration & data model

| Var | Purpose |
|---|---|
| `BILLING_PROVIDER` | `noop` (default post-cutover) \| `metronome` \| `openmeter` (transitional) |
| `METRONOME_API_KEY` | SDK bearer token |
| `METRONOME_WEBHOOK_SECRET` | Webhook HMAC verification |
| `STRIPE_*` | Stripe linkage (via Metronome billing-config) |
| `OPENMETER_*` | Transitional; deleted from astro-server at Phase 4 (OSS uses `noop`, never OpenMeter) |

Account columns: keep `openmeter_customer_id` during bake; add `metronome_customer_id` and `stripe_customer_id` (`internal/account/store.go` gains `Get/SetMetronomeCustomerID`, generalizing the existing accessors); `GetAccountsMissingOpenMeterCustomer` → provider-generic.

**Quota storage (no plans).** System-wide default limits live in config (a `resource → int64` map, e.g. `QUOTA_DEFAULTS`); overrides live in a narrow table:

```sql
CREATE TABLE account_limits (
    account_id  text   NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    resource    text   NOT NULL,   -- agents, agent_deployments, members, …
    limit_value bigint NOT NULL,   -- 0 = disabled; -1 = unlimited
    PRIMARY KEY (account_id, resource)
);
```

`quota.Checker` resolves the effective limit as `override(account_id, resource)` else the config default. Only overridden `(account, resource)` pairs get a row; everything else falls back to the default. Admin-editable (via `astro-queen`).

## Phases

Phases 1–4 are astro-server and each is independently mergeable. Behavior is unchanged until Phase 4 (hosted cutover); Phases 5–8 follow. Every phase lists **Work** (file-level) and **Exit** (verification/done criteria).

### Phase 1 — Interface seam + quota split (pure refactor, zero behavior change)

Carve two interfaces out of the concrete OpenMeter path; OpenMeter stays the only backend.

**Work**
- New `internal/billing/provider.go`: `BillingProvider` + `HostedBilling` interfaces; `UsageEvent`, `Balance`, `Account`, `UsageReport`, `ErrUnsupported`.
- Move `internal/openmeter/*` → `internal/billing/openmeter/`; make `*openmeter.Client` satisfy `BillingProvider` via an adapter — `IngestEvents`→`IngestUsage`, `GetCustomerAccess`→`CheckBalance`, `QueryMeter`→`GetUsage`, `CreateSubscription`→`ProvisionPackaging`, plus the `CloudEvent{ID,Subject,Type,Data}`↔`UsageEvent` mapping (UUID→`TransactionID`).
- New `internal/quota/quota.go`: `Checker` + `Wrap()` middleware. Counts reuse the DB `COUNT`/`GROUP BY` queries already in `openmeter/events.go` (`EmitActive*`); limits from `account_limits` (override) else config default.
- Add `account_limits` to `sql/astro-server/schema.sql`; add `QUOTA_DEFAULTS` to `internal/config/config.go`. Seed `account_limits`/defaults from the limits OpenMeter entitlements enforce today (parity).
- Split `middleware/entitlement.go`: resource-count features (`agents`, `agent_builds`, `agent_deployments`, `members`, `knowledge_stores`, `knowledge_endpoints`) → `quota.Wrap`; consumption/balance (`compute`, `knowledge_compute`, `knowledge_storage`) stays with the billing provider. Preserve `LimitResponse` codes verbatim. (Note: `CreateKnowledgeStore`'s `knowledge_storage` gate becomes billing-only; its `knowledge_stores` count stays quota.)
- Repoint callers off `*openmeter.Client`: `handlers/{usage,infrastructure,accounts,deploy,knowledge,org,agents}.go`; `billing/openmeter/{events,billing,heartbeat}.go`; `riverqueue/{deploy,wakeup,undeploy,reconcile,knowledge_reconcile,heartbeat,openmeter_backfill,purge_accounts,github_build}.go`; `deps.go` (`Clients.OpenMeter`→`Clients.Billing`; add `Quota`); `main.go`.
- `internal/account/store.go`: unchanged (still `openmeter_customer_id`).

**Exit** — compiles; `astro-server:test`/`vet` green; with `OPENMETER_URL` set, metering + 402s byte-identical to pre-refactor; quota 402s reproduce current limits. No functional change.

### Phase 2 — No-op provider (OSS)

OSS runs with no metering backend; quota still enforced.

**Work**
- `internal/billing/noop/`: every `BillingProvider` method no-ops; `CheckBalance`→`{Allow:true}`; `GetUsage`→empty; `HostedBilling`→`ErrUnsupported`.
- `BILLING_PROVIDER=noop|metronome|openmeter` selection in `config.go`/`main.go`; the old "nil client ⇒ off" guards become provider dispatch.

**Exit** — boot `BILLING_PROVIDER=noop`: no outbound metering calls; usage endpoint returns empty; deploy/knowledge/member flows succeed; quota 402s still fire.

### Phase 3 — Metronome provider (dark, USD)

Full Metronome impl behind the flag, unused in prod.

**Work**
- One-time setup (script/Terraform): single product, billable metrics (`compute_usage`, `ai_tokens_in`, `ai_tokens_out`, `knowledge_storage`), `ASTRO_STANDARD` rate card in **USD**; resolve built-in USD-cents `credit_type_id`.
- `internal/billing/metronome/`: SDK client + all methods per **Exact SDK calls**.
- Customer + Stripe linkage in `handlers/accounts.go`; add `metronome_customer_id`, `stripe_customer_id` columns + `account/store.go` accessors (`Get/SetMetronomeCustomerID`).
- Generalize the backfill worker (`riverqueue/openmeter_backfill.go`) → provider-agnostic customer backfill (`GetAccountsMissing<Provider>Customer`).
- New config `METRONOME_API_KEY`, `METRONOME_WEBHOOK_SECRET`, `STRIPE_*`.
- Webhook endpoint `POST /api/v1/webhooks/metronome` (invoice finalized, payment failed, threshold) with HMAC-SHA256 verification (raw body).

**Exit** — in a test env with `BILLING_PROVIDER=metronome`: customer created + Stripe-linked; usage visible in Metronome (dedupe on retry); `CheckBalance` blocks a zero-balance prepaid; webhook signature verifies. Prod remains `openmeter`.

### Phase 4 — Hosted cutover (behavior change)

Hosted moves to Metronome; OpenMeter deleted from astro-server.

**Work**
- Temporary multiplexing provider dual-writes `IngestUsage` to OpenMeter + Metronome during bake.
- Backfill historical usage into Metronome via the generalized backfill; reconcile balances/invoices against the OpenMeter view.
- Flip hosted `BILLING_PROVIDER=metronome`; gating reads Metronome `CheckBalance`.
- Delete `internal/billing/openmeter/`; drop `OPENMETER_*` and `RequiredMeters`/meter-validation at startup. Heartbeat/CU-hour math and billing-state tables stay (now feeding `IngestUsage`).

**Exit** — hosted balances/invoices reconcile with the prior OpenMeter numbers; no OpenMeter references remain in astro-server; OSS on `noop`.

### Phase 5 — LiteLLM token redirect (follow-up, out of astro-server)

Redirect AI-token metering off OpenMeter.

**Work** — repoint the LiteLLM proxy fleet's token/spend emission from OpenMeter to Metronome (fleet config + `internal/aigateway/{client,provisioner}.go` key metadata); land `ai_tokens_in`/`ai_tokens_out` billable metrics end to end, keyed on the Astro account ID.

**Exit** — token usage appears on Metronome invoices; OpenMeter receives no token spend.

### Phase 6 — astro-queen

Remove queen's OpenMeter surface; add quota admin.

**Work** — delete/retarget its OpenMeter client, `ProxyOpenMeter`/`TriggerOpenMeterBackfill` gRPC + proto (`packages/astro-proto/admin/v1`), grant creation, and React pages; add admin editing of `account_limits` overrides.

**Exit** — queen builds without OpenMeter; admins can view/edit per-account limits.

### Phase 7 — Infra teardown

Delete OpenMeter infrastructure.

**Work** — remove `terraform/environments/{prod,preview}/openmeter.tf`, helm values (`preview/openmeter.yaml.tpl`), the meters bootstrap job, the Grafana `openmeter-sink` dashboard, and `scripts/bootstrap-openmeter.sh` / `scripts/backfill-openmeter-subscriptions.sh`.

**Exit** — no OpenMeter deployment or dashboards in any environment.

### Phase 8 — Credit unit

Swap USD → the Astro credit.

**Work** — confirm whether `ASTRO_CREDIT` (`$0.001`) is REST-creatable or app-only; create it; migrate rates/commits/grants/balance from USD to credits (or keep USD and render credits in UI + invoices). Update `Balance.RemainingUSD`→credits and the grant/commit denomination.

**Exit** — balances, invoices, and the usage UI are expressed in credits; `1 credit = $0.001` holds against `pricing.ts`.

## Open questions

- **Credit rollover** — recurring/subscription credit rollover vs expire, per model.
- **Provisioning surface** — enterprise/prepaid/subscription setup in `astro-queen` vs self-serve prepaid top-ups.
- **Rate-card source of truth** — `pricing.ts` vs Metronome rate card; one must derive from the other.
- **Dunning/downgrade policy** — gating on payment failure and prepaid exhaustion (grace window?).
- **Unverified API details** to confirm against OpenAPI before coding: ingest batch/payload limit; the built-in USD-cents `credit_type_id` (via `GET /v1/credit-types/list`, `mc.V1.PricingUnits.List`) for v1; prepaid auto-recharge field name; exact `AccessSchedule`/`InvoiceSchedule` sub-fields on commits. (Whether the `ASTRO_CREDIT` pricing unit is REST-creatable is a Phase 8 concern, not a v1 blocker.)
