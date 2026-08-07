# Enable Metronome signup provisioning in preview

## Summary

Preview has run with `BILLING_PROVIDER=metronome` and a valid API key for a
while, so billing pages render and Metronome customers exist. But
`METRONOME_PACKAGE_ID` was never set, and the provisioner returns before
creating a contract when it is empty. No account has ever received a contract
or a signup credit grant, so every customer's contract credit balance reads
zero because it was never funded rather than because anything was spent.

That is indistinguishable from a spent balance to anything downstream. A
credit-balance alert reads the same zero either way, which makes the gap
actively dangerous once exhaustion gating ships: every account would be
suspended on the first webhook, having never held credit at all.

## Design

The four values are non-secret identifiers and policy numbers, so they belong
in the ConfigMap source rather than the secret. `config/<service>/<env>.env` is
applied as `<service>-config` by the Sync Secrets & Config workflow, which
diffs the data and rolls only the services whose ConfigMap actually changed.
The two genuine secrets, `METRONOME_API_KEY` and `METRONOME_WEBHOOK_SECRET`,
are already synced from 1Password and are untouched here.

Nothing about the state machine changes. Setting the package id simply lets
`Provision` run to completion, and the hourly provision sweep runs on worker
start, so existing accounts are backfilled without a manual pass.

Two values are worth stating plainly because both have a silent failure mode:

`METRONOME_SIGNUP_CREDIT` is denominated in the credit type's own unit. Credit
type `2714e483` is USD, so `50` grants $50. The amount is set low deliberately:
at $0.06 per compute unit and gateway spend billed at cost, a large grant would
leave exhaustion unreachable in preview, and the gate would ship unexercised.

`METRONOME_CREDIT_EXPIRY_DAYS` is a calendar deadline on unused credit, not the
behavior on reaching zero. Zero does not mean "never expires"; it computes
`now + 0 days` and expires every grant the moment it is created, which zeroes
balances with no usage and reads downstream exactly like exhaustion.

## Migration

Merge, then run **Sync Secrets & Config** against `preview` with `sync_config`
enabled. Confirm balances are non-zero in Metronome before enabling
credit-exhaustion gating, since the gate cannot distinguish an unfunded account
from a spent one.
