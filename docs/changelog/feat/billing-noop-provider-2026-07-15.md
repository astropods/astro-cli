# No-op billing provider + BILLING_PROVIDER selection (Metronome migration, Phase 2)

## Summary

OSS/self-hosted can now run with no metering backend while quotas stay enforced.
Adds a `noop` billing provider and a `BILLING_PROVIDER` switch that selects the
backend at startup. Stacked on the Phase 1 billing seam.

## Design

- **`internal/billing/noop`** — implements `billing.BillingProvider`: customer
  ops are no-ops, `IngestUsage` discards, `CheckBalance` always allows,
  `GetUsage` returns empty. It deliberately does **not** implement
  `HostedBilling`, so the `, ok` type-assertion in callers (account creation,
  backfill) cleanly skips hosted-only operations like packaging on OSS.

- **`BILLING_PROVIDER`** — `noop` | `openmeter` | `metronome`. `Config.BillingBackend()`
  resolves an empty value to `openmeter` when `OPENMETER_URL` is set, else `noop`,
  so existing deployments are unaffected. `main.go` dispatches on it; the old
  "nil client ⇒ off" pattern becomes an explicit provider value. Metered
  consumption is never balance-gated under noop; DB-backed quotas still apply.

The concrete OpenMeter client remains only for the transitional usage/
infrastructure readers and the entitlement middleware.

## Rollout

Preview and prod are cut over to `BILLING_PROVIDER=noop` for the Metronome
migration. Both also set `OPENMETER_ENFORCE=false` so early users are not
blocked by resource quota limits during the transition — quota checks
degrade to log-only. Re-enable enforcement by flipping `OPENMETER_ENFORCE`
back to `true`; the noop provider does not gate quotas either way.

## Migration

None. Unset `BILLING_PROVIDER` preserves prior behavior (openmeter when
`OPENMETER_URL` is set). Self-hosted deployments can set `BILLING_PROVIDER=noop`
to run unmetered with quotas still enforced.
