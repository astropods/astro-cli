# Remove OpenMeter from astro-server

## Summary

OpenMeter is fully removed from `apps/astro-server`. It was the transitional
metering/entitlement backend behind the `billing.BillingProvider` seam; with the
Metronome provider and the no-op OSS provider in place, the OpenMeter client and
its provider adapter are no longer needed. Billing now runs on two backends —
`metronome` (hosted) and `noop` (OSS, default).

## Design

- **Metering engine kept, moved.** The provider-agnostic compute/knowledge
  CU-hour engine (`BillingStateManager`, `Heartbeat`, event builders) previously
  lived in the `internal/billing/openmeter` package but was written entirely
  against `billing.BillingProvider`. It moved unchanged to a new
  `internal/billing/metering` package. The periodic heartbeat river job kind is
  now `metering.heartbeat` (was `openmeter.heartbeat`).

- **OpenMeter deleted.** The `internal/billing/openmeter` package (HTTP client +
  provider adapter, CloudEvent format, meter validation, `GetCustomerAccess`,
  `QueryMeter`) is gone. `BILLING_PROVIDER` accepts `noop` | `metronome`;
  `BillingBackend()` defaults to `noop` when unset. `OPENMETER_URL` and the
  customer-backfill worker are removed. `OPENMETER_ENFORCE` is retained — it
  gates the DB-backed resource quota checker, which is unaffected.

- **Consumption gating is now a no-op.** Metered-consumption 402s (compute,
  knowledge storage) were sourced only from OpenMeter entitlements. The
  `middleware.Entitlements` gate now passes through and never blocks; the seam is
  retained for a future `billing.BillingProvider.CheckBalance` path. DB-backed
  resource-count quotas (`internal/quota`) still enforce limits and still return
  402s.

- **Usage reads.** The usage page reports DB resource counts only; the
  infrastructure-usage endpoints return an empty payload (routes retained). These
  consumption readbacks had no non-OpenMeter data source.

- **Admin RPCs.** `ProxyOpenMeter` and `TriggerOpenMeterBackfill` bodies were
  removed; the embedded `UnimplementedAdminServiceServer` satisfies the proto
  interface (they now return `Unimplemented`). The proto is unchanged.

## Migration

None for OSS/default (boots `noop`). For hosted, set `BILLING_PROVIDER=metronome`
+ `METRONOME_*`. `OPENMETER_URL` can be unset in deployment env; `OPENMETER_ENFORCE`
remains the DB-quota enforcement flag. The `accounts.openmeter_customer_id`
column is left in place (no migration). Full proto cleanup and astro-queen's
OpenMeter admin surface are follow-ups.
