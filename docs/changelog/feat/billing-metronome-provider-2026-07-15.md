# Metronome billing provider (dark, USD) — Metronome migration, Phase 3

## Summary

Adds a Metronome-backed billing provider behind the `billing.BillingProvider`
seam, selectable via `BILLING_PROVIDER=metronome`. Introduced **dark**: it
compiles and is wired end-to-end but is not enabled in production (prod stays on
openmeter). Stacked on Phase 2 (no-op provider). v1 is USD-denominated.

## Design

- **`internal/billing/metronome`** — implements `BillingProvider`
  against the official Metronome Go SDK (`github.com/Metronome-Industries/metronome-go`):
  - `CreateCustomer` → `V1.Customers.New` with the Astro account ID as an ingest
    alias; `DeleteCustomer` → `Archive`.
  - `IngestUsage` → `V1.Usage.Ingest`, chunked to the batch limit; the event UUID
    is the `transaction_id` for dedupe/idempotency.
  - `CheckBalance` → sums remaining commit/credit balances via
    `V1.Contracts.ListBalances` (prepaid view: allow while balance > 0).
  - `GetUsage` is stubbed for this phase (usage rendering stays on the concrete
    provider client).
  - Packaging/contracts, credit grants, and commits are **not** server
    operations — they are provisioned out-of-band (Metronome admin / Terraform).
    astro-server has a single `BillingProvider` interface; there is no
    hosted-only provisioning surface.

- **Provider selection** — `main.go` dispatches `BILLING_PROVIDER=metronome` to
  the new provider (nil-guarded on `METRONOME_API_KEY`).

- **Config** — `METRONOME_API_KEY`, `METRONOME_WEBHOOK_SECRET`.

- **Data model + provider-agnostic persistence** — `accounts.metronome_customer_id`
  column. Customer-id persistence is now
  backend-aware: `GetBillingCustomerID` / `SetBillingCustomerID` /
  `GetAccountsMissingBillingCustomer` write the right column for the active
  backend (openmeter ↔ metronome), so account creation and account purge operate
  on the correct provider. (Backend-specific `Get/SetMetronomeCustomerID`
  accessors also exist.)

- **Webhook** — `POST /webhooks/metronome` (no auth; HMAC-SHA256 over
  `Metronome-Webhook-Date + "\n" + rawBody`, keyed by `METRONOME_WEBHOOK_SECRET`,
  constant-time compared to `Metronome-Webhook-Signature`). Routes invoice
  finalized / payment failed / threshold events (handlers stubbed).

## Stripe

astro-server does not integrate the Stripe SDK. Metronome owns invoice delivery
through Stripe via its billing-provider configuration (set up once at the
Metronome/admin level); astro-server never calls Stripe directly.

## Follow-ups (not in this phase)

Wiring the webhook event handlers to real reconciliation/dunning.

## Migration

None. Prod is unaffected (`BILLING_PROVIDER` unset → openmeter). Enabling
Metronome requires `BILLING_PROVIDER=metronome` + `METRONOME_*` config and the
one-time Metronome object setup (product, billable metrics, rate card).
