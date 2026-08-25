# Exempt postman-preview from billing suspension

## Summary

`BILLING_EXEMPT_ACCOUNTS` has never been set in preview, so the exemption path
has run in no environment: prod carries two org ids but runs
`BILLING_PROVIDER=noop`, and preview runs the billing workers against Metronome
with an empty list. Pointing preview at the `postman-preview` org gives that
path an account to hold, so it can be exercised before the prod cutover relies
on it.

## Design

The exemption is read once at boot and handed to the status store, which checks
it ahead of every suspension reason. That ordering is the point: an exempt
account stays active when the provider is unreachable, when it holds no
contract, and when the dunning sweep runs on its own timer, none of which the
API path alone would cover.

## Migration

Preview gains one key. The value is read at startup, so the running pods keep
the old empty list until they restart.
