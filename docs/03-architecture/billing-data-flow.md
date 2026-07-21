# Billing — Code-Level Data Flow (as built)

How billing data actually moves through `apps/astro-server` today. This is the
**as-built** view (function-by-function, with file references), complementing the
design intent in [`../01-spec/metronome-billing-spec.md`](../01-spec/metronome-billing-spec.md).
Where the code diverges from the spec's target design, this doc says so.

All paths depend on the `billing.BillingProvider` interface
(`internal/billing/provider.go`); the concrete backend is chosen once at startup.

## Startup wiring

`main.go` builds one `billing.BillingProvider` from `BILLING_PROVIDER` and threads
it into both the API server and the worker.

```mermaid
flowchart TB
    CFG["cfg.BillingBackend()<br/>internal/config/config.go"] --> SW{"switch"}
    SW -->|"noop (default)"| N["noop.New()"]
    SW -->|metronome| M["metronome.New(Config{APIKey})"]
    N --> BP["var billingProvider billing.BillingProvider"]
    M --> BP
    BP --> API["runAPI(...)"]
    BP --> WRK["runWorker(...)"]
```

Note: `metronome.New` returns `nil` when `METRONOME_API_KEY` is absent, so
downstream `if billingProvider != nil` guards degrade to "billing disabled"
rather than panicking. `BillingBackend()` defaults to `noop` when
`BILLING_PROVIDER` is unset.

## The interface

`internal/billing/provider.go` — a single `BillingProvider` interface (all backends): `CreateCustomer`, `DeleteCustomer`, `IngestUsage`, `CheckBalance`, `GetUsage`.

There is no hosted-only provisioning surface. Packaging/contracts, credit grants, and commits are **not** server operations — they are provisioned out-of-band (Metronome admin / Terraform). `noop` returns zero values (and is the default backend); `metronome` implements the real calls.

---

## Flow 1 — Customer creation (inline, on account create)

Non-blocking: a billing failure is logged, never fatal to account creation.

```mermaid
sequenceDiagram
    autonumber
    participant C as CreateAccount<br/>handlers/accounts.go:245
    participant P as billingProvider
    participant DB as accountStore
    C->>P: CreateCustomer(billing.Account{ID,Name,Type,OwnerEmail})
    Note right of P: metronome → V1.Customers.New<br/>(IngestAliases = account.ID)<br/>metronome.go:61
    P-->>C: customerID
    C->>DB: SetBillingCustomerID(acct.ID, backend, customerID)
```

## Flow 2 — Account delete (billing archive)

The billing customer is **archived before** the soft-delete, so charging stops
immediately and a **failed archive aborts the delete** — the account is never
left deleted-but-still-billable. Only after a successful archive does
`MarkDeleted` run. The purge worker does no billing work — it only tears down
deployments/other external resources and hard-deletes the row.

```mermaid
sequenceDiagram
    participant H as DeleteAccount<br/>handlers/accounts.go:366
    participant DB as accountStore
    participant P as billingProvider

    H->>DB: GetBillingCustomerID(accountID, backend)
    opt billingProvider != nil && customerID != ""
        H->>P: DeleteCustomer(customerID)
        Note right of P: metronome → V1.Customers.Archive (no hard delete)
        alt archive fails
            H-->>H: 500, abort (no soft-delete)
        end
    end
    H->>DB: MarkDeleted(accountID)
```

---

## Flow 3 — Usage metering (compute CU-hours)

The largest path, and fully provider-agnostic. Deployment lifecycle transitions
write **anchor rows** to `deployment_billing_state`; a periodic river heartbeat
computes CU-hour **deltas** and pushes them through `IngestUsage`. No events are
emitted at start/stop — only the heartbeat emits.

```mermaid
sequenceDiagram
    autonumber
    participant D as deploy / undeploy / wakeup / reconcile workers
    participant BSM as BillingStateManager<br/>billing/metering/billing.go
    participant HB as metering.heartbeat river job<br/>riverqueue/heartbeat.go
    participant P as billingProvider
    participant BE as backend

    D->>BSM: StartBilling / StopBilling
    Note over BSM: writes deployment_billing_state<br/>(anchor timestamp / stopped_at) — no events

    loop periodic heartbeat
        HB->>HB: NewHeartbeat(provider, db, log, billing)
        HB->>BSM: RunBillingCycle(ctx) (via Heartbeat.emitComputeUsage:92)
        Note over BSM: emitActiveBilling — for each active row:<br/>cu = rawCU(cpu,mem,replicas), value = cu x elapsedHours<br/>build UsageEvent batch (TransactionID = event UUID)
        BSM->>P: IngestUsage(events)
        P->>BE: noop→discard · metronome→V1.Usage.Ingest (chunk 100)
        Note over BSM: only advance last_emitted_at after IngestUsage succeeds
    end
```

Key properties:
- **Idempotency**: `UsageEvent.TransactionID` is the event UUID; Metronome dedupes within its 34-day window, so retries/backfill are safe.
- **Failure handling**: if `IngestUsage` errors, the anchor timestamps are *not* advanced, so the next cycle re-emits the same delta (`billing.go:143-146`).
- **Package location**: this machinery lives in `internal/billing/metering`, written against the `billing.BillingProvider` interface — it drives any backend.

---

## Flow 4 — Balance gating (currently a no-op)

Metered-consumption gating (compute, knowledge_storage) has no balance source
wired, so the `middleware.Entitlements` gate is a **no-op**: `Wrap` passes through
and `Check` never blocks. DB-backed resource quotas (`internal/quota`) still
enforce counts and still return 402s. `CheckBalance` is implemented on every
provider but has no request-path caller; the seam is retained so it can be wired
later.

```mermaid
flowchart TD
    REQ["protected route<br/>ent.Wrap(handler, features...)"] --> PASS["handler runs (no consumption gate)"]
    QUOTA["quota.Wrap / quota.Check<br/>internal/quota"] --> Q402{"over DB limit?"}
    Q402 -->|yes| R402["402 (resource-count limit)"]
    Q402 -->|no| PASS
    CB["provider.CheckBalance(...)<br/>implemented, no request-path caller"]:::todo
    classDef todo stroke-dasharray: 5 5;
```

## Flow 5 — Metronome webhook (inbound)

Standalone endpoint, independent of the provider interface. Verifies an HMAC
signature, then dispatches on event type. Handlers are stubbed (log-only) today.

```mermaid
sequenceDiagram
    participant MT as Metronome
    participant H as MetronomeWebhook<br/>handlers/webhooks_metronome.go:29
    MT->>H: POST /webhooks/metronome + Metronome-Webhook-Signature
    alt secret == ""
        H-->>MT: 404 (endpoint disabled)
    else
        H->>H: verifyMetronomeSignature(secret, date, body, sig)<br/>HMAC-SHA256(date + "\n" + body), constant-time · :76
        alt invalid
            H-->>MT: 401
        else valid
            H->>H: switch env.Type
            Note right of H: invoice.finalized · payment.failed ·<br/>alert.threshold_reached → log-only (TODO)
            H-->>MT: 200 {status: ok}
        end
    end
```

---

## Backend-aware persistence

Customer IDs are stored per-backend in `accounts`. The generic accessors resolve
the column from a whitelist, so a single call site serves any backend.

```mermaid
flowchart LR
    CALL["Get/SetBillingCustomerID(accountID, backend)<br/>GetAccountsMissingBillingCustomer(backend, limit)<br/>account/store.go"] --> MAP{"billingCustomerColumns[backend]"}
    MAP -->|metronome| C2["accounts.metronome_customer_id"]
    MAP -->|noop / unknown| NOP["no column → get returns empty string, set no-ops"]
```

The Stripe customer id is stored separately on `accounts.stripe_customer_id`
(dedicated `Get/SetStripeCustomerID`), because Stripe is a **payment** provider,
not a metering backend — it isn't part of the `billingCustomerColumns` whitelist.

---

## Flow 6 — Payment method (Stripe card vault)

Stripe is used **only to collect and save a card**; astro-server never charges.
The card is confirmed synchronously (no webhook): the server re-reads the
SetupIntent from Stripe rather than trusting the client, then links the Stripe
customer to the Metronome customer so Metronome charges the saved card when it
finalizes invoices.

```mermaid
sequenceDiagram
    autonumber
    participant UI as PaymentMethod.tsx (Stripe Elements)
    participant H as payment handlers<br/>handlers/payment_methods.go
    participant SP as payment.Provider (Stripe)
    participant BP as billingProvider (Metronome)
    participant DB as accountStore

    UI->>H: POST /billing/setup-intent
    H->>DB: GetStripeCustomerID (create + persist if absent)
    H->>SP: CreateSetupIntent(customerID)
    SP-->>UI: {client_secret, publishable_key}
    UI->>UI: stripe.confirmSetup (SCA, card never touches our server)
    UI->>H: POST /billing/payment-method {setup_intent_id}
    H->>SP: ConfirmSetup — re-read intent, verify succeeded,<br/>detach old cards, set default
    H->>BP: LinkStripeCustomer(metronomeID, stripeID)<br/>billing config: stripe + charge_automatically (best-effort)
    H-->>UI: {card}
```

Notes:
- **Card-only, no webhook.** `confirmSetup` returns synchronously for cards, so
  the confirm endpoint is authoritative — no `STRIPE_WEBHOOK_SECRET`.
- **Linkage is optional.** Detected via an interface assertion
  (`LinkStripeCustomer`) so the core `BillingProvider` seam stays metering-only;
  a link failure doesn't fail the save (the card is already vaulted).
- **Enablement.** The provider is nil unless `STRIPE_SECRET_KEY` is set; handlers
  then report `available:false` and the UI hides the section.

---

## What is / isn't behind the interface today

| Concern | On the `BillingProvider` seam? | Live call path |
|---------|-------------------------------|----------------|
| Customer create / delete | ✅ | `handlers/accounts.go`, purge worker |
| Usage metering (compute CU-hours) | ✅ | `BillingStateManager.RunBillingCycle` → `IngestUsage` |
| Customer-id persistence | ✅ (backend-aware) | `account/store.go` |
| Consumption gating (compute / knowledge_storage 402) | ➖ no-op | `middleware.Entitlements` passes through; `CheckBalance` unwired |
| Resource-count limits (agents, deployments, …) | ✅ DB-backed | `internal/quota` (`quota.Wrap`/`Check`) |
| Usage readback | ➖ quota counts only | `/usage` returns DB counts; `/usage/infrastructure` returns empty; `metronome.GetUsage` returns empty |
| Packaging / contracts / credit grants | ➖ not a server concern | provisioned out-of-band (Metronome admin / Terraform) |
| Payment method (card collection) | ➖ separate `payment.Provider` (Stripe) | `handlers/payment_methods.go`; linked to Metronome via `LinkStripeCustomer` |
| AI-token metering | ❌ not yet emitted at the billing layer | — |

Re-enabling consumption gating and usage readback (on `CheckBalance`/provider usage APIs) is the remaining hosted-cutover work.
