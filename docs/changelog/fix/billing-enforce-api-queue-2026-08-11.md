# Give the API process the billing enforce flag

## Summary

`BILLING_GATE_ENFORCE=true` had no effect on anything the API process did.
Billing actions taken from a request path always ran in observe mode: they
computed the right status, logged the action they would have taken, and did not
take it.

The visible failure is the one the flag exists to prevent. Removing the last
card from an account whose free credit is spent recomputes the status to
`suspended` with reason `credits_exhausted`, and then logs
`billing gate (observe): would suspend` instead of enqueueing the job. The
account keeps running with no credit and no card, while the client banner
correctly tells the owner their agents are stopped.

## Design

The API and worker processes build their queues from different constructors.
`New` sets `billingEnforce` from `Config.ServerConfig`; `NewInsertOnly` did not
take the config at all, so the field took Go's zero value and `billingActs`
refused every action. Only request paths were affected: webhook-driven
suspend and resume run on the worker and were always correct.

`NewInsertOnly` now takes `billingEnforce` as a parameter rather than reading it
from a config struct. A parameter cannot be left unset by omission, which is how
the original defect survived: nothing about a struct literal missing one field
looks wrong at the call site.

`TestNewInsertOnly_CarriesBillingEnforce` covers the wiring rather than the gate
logic, which was already tested. `pgxpool` connects lazily, so it asserts the
constructor's behaviour without a database. It fails against the previous
constructor with `billingActs = false, want true`.

## Migration

None. No configuration changes. Preview already sets `BILLING_GATE_ENFORCE=true`
and starts enforcing on request paths once this ships; production is unchanged
at `false`.
