# Stripe payment-collection webhooks, processed via the River queue

## Summary

Metronome does not relay Stripe payment events, so dunning/collection state — payment failure, 3DS action, uncollectible write-off, void, card auto-update — had no path into astro-server. This adds a signature-verified `POST /webhooks/stripe` endpoint and routes **both** billing webhooks (Metronome and Stripe) through the River queue as tracked, retryable jobs instead of doing the work inline in the handler.

## Design

**Receive → enqueue → process.** Each webhook handler now does the minimum on the request path: verify the signature, parse a minimal envelope, enqueue a job, return. Account mapping, cached-status writes, and workload reconciliation move into River workers (`webhook.metronome` / `webhook.stripe`), so every accepted event becomes a `river_job` row with retries and history rather than best-effort inline work.

- **Idempotency** is the provider event id, tagged `river:"unique"` and inserted with `UniqueOpts{ByArgs: true}` — redeliveries dedupe against non-cleaned jobs. An empty event id disables dedupe (double-processing is safe; collapsing distinct events is not).
- **Durability.** Enqueue/parse failures return 500 so the provider redelivers; a transient DB error in the worker is returned so River retries (only an unknown customer or unhandled type acks without retry). The worker reconciles workloads to the current status on every handled event and returns enqueue errors — so a dropped `billing.resume` self-heals on the next event (there is no other resume backstop).
- **Backend-gated routes.** The webhook routes register only for the metronome backend, where the draining workers exist; other backends 404 rather than accumulate undrainable jobs.
- **Dedicated `billing` queue.** The gating pipeline (`webhook.metronome`, `webhook.stripe`, `billing.suspend`, `billing.resume`, `billing.dunning_sweep`) runs on its own River queue so a webhook burst can't starve the default worker pool.

**Provider-neutral signal seam.** Event semantics live in one place: each worker maps its event type to a `billing.Signal`, then calls `billing.ApplySignal`, which writes the collection flags and recomputes status. On a transition the worker enqueues `billing.suspend`/`billing.resume`.

| Signal | source | effect |
|---|---|---|
| payment failed / action required | Stripe `invoice.payment_failed`, `invoice.payment_action_required` | enter dunning → `past_due` |
| uncollectible | Stripe `invoice.marked_uncollectible` | force `suspended` (write-off), bypasses grace |
| voided | Stripe `invoice.voided` | clear dunning + alert + force-suspend (debt gone) |
| recovery | Stripe `invoice.paid` | clear dunning only |
| card updated | Stripe `payment_method.automatically_updated` | clear dunning |
| alert | Metronome `alerts.spend_threshold_reached` | `suspended` (balance alert) |

Payment failure/recovery are Stripe-only; Metronome relays no payment events, so its sole gating signal is the spend-threshold alert. Recovery deliberately clears only the payment-failure track (dunning) — not a spend alert (paying an invoice doesn't lower period spend) nor a terminal write-off.

**Force-suspend for write-offs.** `invoice.marked_uncollectible` needs immediate suspension, stronger than dunning-plus-grace. A new `force_suspended` column on `account_billing_status` (reason `uncollectible`) is the state machine's highest-priority rule, cleared when the invoice is voided or by admin — treated as terminal, not lifted by an unrelated payment.

The card-save confirm stays synchronous; only the collection lifecycle is webhook-driven. astro-server still never charges — it mirrors Stripe/Metronome state into the cached gating status that the request-path gate reads.

**Stripe SDK upgrade.** Bumped `stripe-go` v82 → v86 (API version `2026-06-24.dahlia`) so the webhook's `ConstructEvent` release-train check matches a dahlia-configured Stripe endpoint. We consume classic snapshot events (full object embedded, `object:"event"`), not v2 thin event notifications. The only code impact was list iteration (`.List(...).All(ctx)`); the card-vault APIs (`V1Customers`/`V1SetupIntents`/`V1PaymentMethods`) are unchanged.

## Migration

Additive schema change (Atlas applies from `sql/astro-server/schema.sql`): a `force_suspended boolean NOT NULL DEFAULT false` column on `account_billing_status`. No backfill. To enable the Stripe endpoint, set `STRIPE_WEBHOOK_SECRET` (unset ⇒ endpoint 404s); no change required for existing deployments.
