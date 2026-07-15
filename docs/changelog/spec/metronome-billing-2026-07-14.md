# Metronome billing integration spec

## Summary

Design + implementation plan for integrating Metronome (Stripe) as the rating, contracts, and invoicing backend for hosted Astro, and **fully decommissioning OpenMeter**. Astro today emits usage to OpenMeter and gates on OpenMeter entitlements, but has no payment collection, contracts, or committed spend. This work replaces OpenMeter behind a provider interface, separates per-account quotas from billing (which OpenMeter conflated), and retires OpenMeter entirely — the OSS / self-hosted distribution runs an unmetered no-op provider instead.

Adds `docs/01-spec/metronome-billing-spec.md` (design) and `docs/01-spec/metronome-billing-implementation.md` (exact Metronome Go SDK calls + phased rollout).

## Design

- **Quota is separated from billing.** OpenMeter conflated resource limits with metering. Per-account resource limits (agents, agent builds/period, deployments, members, knowledge stores, endpoints) move to a DB-backed `internal/quota` package — enforced for OSS and hosted alike, no billing dependency. **No plans**: system-wide default limits + per-account overrides (`account_limits` table). Of today's nine meters, 6 become pure quota and 4 pure billing (compute, knowledge_compute, knowledge_storage, AI tokens) — a clean split, no straddlers.
- **One interface, three impls.** A `BillingProvider` abstracts customer lifecycle, usage ingestion, balance/spend gating, and usage readback. Metronome (hosted) and a no-op (OSS) implement it; OpenMeter becomes a transitional impl deleted after cutover. `BILLING_PROVIDER` selects at startup. Metering (CU-hour math, lifecycle state tables, heartbeat reconcile) is provider-agnostic — only the event sink moves.
- **Start in USD; credit unit later.** The build denominates the single balance, rates, commits, and grants in USD (Metronome native), keeping the unverified custom-pricing-unit creation off the critical path. The Astro credit (`1 credit = $0.001`) is a later phase — a single `credits` product all meters draw from. Compute stays metered in CU-hours (the earlier per-request reconciliation is dropped).
- **Payments.** Metronome hands finalized invoices to Stripe for collection (hosted only); a signed webhook handler drives dunning and near-balance warnings. The no-op provider ships none of the collection path.
- **Phased, behavior-preserving.** Phase 1 is a pure refactor (interface seam + quota extraction, OpenMeter unchanged); no observable change until the hosted cutover (Phase 4) deletes OpenMeter from astro-server. Follow-up phases handle the LiteLLM token redirect, astro-queen, infra teardown, and the USD→credit swap.

## Migration

None. Documentation only. Implementation is staged in the doc's phases; OSS moves to the no-op provider (never Metronome/Stripe), and OpenMeter is fully retired rather than retained.
