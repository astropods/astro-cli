# Link Bifrost customer id as a Metronome ingest alias

## Summary

Gateway usage is metered against the Bifrost (AI Gateway) customer id, while
billing rolls up against the Astro account id. Without a shared alias, gateway
usage events don't attribute to the account's Metronome customer. This links the
two so both the account id and the Bifrost customer id resolve to the same
billing customer.

## Design

Two directions, since the Bifrost customer is created lazily (first deploy / dev
key), after the Metronome customer:

- At Metronome customer creation, `billing.Account.BifrostCustomerID` (when
  known) is added to `IngestAliases` alongside the account id.
- When the Bifrost customer is first created, `aigateway.Provisioner` calls a
  new `billing.AliasSyncer`, which resolves the account's Metronome customer and
  sets its aliases to `[accountID, bifrostCustomerID]`. Best-effort — failures
  are logged and never block key minting.

`SetIngestAliases` was added to the `BillingProvider` interface (metronome calls
the SDK; noop no-ops). The syncer always sends both aliases because the API
replaces the full set.

## Migration

None. No-op for the OSS/noop backend.
