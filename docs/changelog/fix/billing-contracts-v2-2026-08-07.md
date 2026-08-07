# Recognise our own contracts by uniqueness key

## Summary

Provisioning listed a customer's contracts on an endpoint Metronome has
retired, so every job failed 403 and no account was ever put on the rate card.
The read moves to the supported endpoint. Contract creation is unaffected.

The supported list response carries no package, which changes how provisioning
decides whether a customer is already covered.

## Design

**A contract is ours by uniqueness key alone.** Every contract we create carries
`contract:<account>`. Matching on it was already the load-bearing half of the
check, because it is what lets a retry after a partial failure find the contract
it just made instead of cancelling the account for good.

What is gone is the ability to recognise a contract on our own package that we
did not create. Such a contract now reads as foreign and blocks provisioning.

That is the safe direction. The alternative on an unrecognised contract is to
create a second one alongside it, which double-bills a customer already being
charged. Refusing and naming the contract ID for an operator is the right
failure, and `ErrProvisionBlocked` cancels the job rather than burning its
backoff schedule on every sweep.

## Migration

Nothing to configure. The sweep provisions any account with no contract.

An account holding a contract created outside this path will block, by design:
with no uniqueness key it cannot be told apart from a contract on someone else's
package. Archive that contract in the Metronome dashboard and the next sweep
puts the account through the standard flow, which is where it should be anyway —
same package, but keyed, so later re-checks recognise it. Two preview accounts
are in this state.

Jobs that already failed are `retryable`, not `discarded`, and River's backoff
grows past an hour; the sweep will not re-enqueue them because `retryable` is in
their unique-state set. To pick them up on the next tick:

```sql
UPDATE river.river_job
   SET state = 'available', scheduled_at = now(), attempt = 0
 WHERE kind = 'billing.provision' AND state = 'retryable';
```
