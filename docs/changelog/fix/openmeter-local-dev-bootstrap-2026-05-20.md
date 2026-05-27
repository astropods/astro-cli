## Summary

`moon run astro-server:dev` was no longer self-sufficient for OpenMeter setup. PR #1033 moved the bootstrap call out of `dev.sh` into the top-level orchestrator, so anyone starting just the server skipped OpenMeter seeding entirely. Compounding this, `OPENMETER_DEFAULT_PLAN` had been unset for a while, leaving a backlog of accounts with an `openmeter_customer_id` but no active subscription — entitlement checks then rejected their Deployments. The existing customer-creation backfill worker queries `WHERE openmeter_customer_id IS NULL`, so it never repaired those accounts.

## Design

Three pieces, all gated on `OPENMETER_URL` so no-OpenMeter setups stay no-op:

**Bootstrap reacquired by `dev.sh`.** The call to `scripts/bootstrap-openmeter.sh` is restored to `astro-server:dev`, with `OPENMETER_DEFAULT_PLAN=private_beta` added to `.env.example` so the auto-subscribe path in `handlers/accounts.go` and the `openmeter_backfill` worker stop silently no-op'ing for new accounts.

**Plan lifecycle made truly idempotent.** `bootstrap-openmeter.sh` previously re-`POST`ed to `/plans` and assumed that was idempotent; in practice a prior half-finished run could leave a draft plan that subsequent runs never published. New `om_ensure_plan` helper:

```
GET /api/v1/plans/{key}?includeLatest=true
  ├─ 404    → POST /api/v1/plans (creates draft) → fall through
  └─ 200    → status:
       ├─ active → no-op
       ├─ draft  → POST /api/v1/plans/{id}/publish
       └─ other  → log + continue
```

JSON parsing is grep/sed-based (no jq dependency) and relies on plan-level fields appearing before nested phase/rateCard fields in the response.

**Subscription backfill for the existing gap.** New `scripts/backfill-openmeter-subscriptions.sh` iterates every account with an `openmeter_customer_id`, checks `GET /customers/{id}/subscriptions?status=active`, and `POST`s a subscription when missing. Auto-loads `DATABASE_URL` / `OPENMETER_URL` / `OPENMETER_DEFAULT_PLAN` from `apps/astro-server/.env` so the common case is a one-liner. Idempotent and safe to re-run; exits non-zero on any per-account failure. `dev.sh` runs it after bootstrap so dev environments self-heal on every server start.

The backfill script's `psql` precondition is a soft skip rather than fatal — without that, `dev.sh`'s `set -euo pipefail` aborted the whole server start for developers who didn't have `libpq` locally, which is strictly worse than skipping an idempotent repair.

## Migration

No action required for production — these scripts are local-dev only. Developers should `cp apps/astro-server/.env.example apps/astro-server/.env` (or set `OPENMETER_DEFAULT_PLAN=private_beta`) to enable auto-subscribe for new local accounts. Existing local accounts missing a subscription will be repaired automatically on the next `astro-server:dev`.
