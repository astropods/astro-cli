# Fix: stop the insights roll-up retrying deleted accounts

## Summary

The insights roll-up retried forever on accounts that no longer exist, and could
not even record why:

```
Insights rollup: day failed, holding watermark day="2026-05-22"
  error="insights rollup: load account 6929...: account not found: 6929..."
Insights rollup: record failure
  error="pq: insert or update on table \"insights_rollup_state\"
         violates foreign key constraint \"insights_rollup_state_account_id_fkey\""
jobexecutor.JobExecutor: Job errored; retrying job_kind="insights.rollup_account"
```

Two faults combined. Discovery enumerates accounts through `account_langfuse`,
whose credential row outlives a soft delete, so every daily tick fanned out a
job per deleted account. Each job then loaded the account through `GetByID`,
which filters `deleted_at IS NULL` and returns an error rather than a nil
account, so the deleted case escaped as a failure and burned River's retry
budget. The producer already had an `if acct == nil` branch documenting exactly
this case; `GetByID` never returns that shape, so the branch was dead code.

Recording the failure then failed too. After a hard delete, the account row is
gone and `insights_rollup_state.account_id` cascades with it, so writing the
error back violated the foreign key. The job retried on an error it could never
write down, for an account that will never come back.

## Design

Discovery no longer hands out accounts the rest of the pipeline will refuse to
load. `ListAccountIDs` joins the credential table to live accounts:

```sql
SELECT al.account_id
FROM account_langfuse al
JOIN accounts a ON a.id = al.account_id AND a.deleted_at IS NULL
```

That closes the recurring fan-out. It does not close the race where an account
is deleted after its job is enqueued, so the job path handles that separately,
through a sentinel rather than a string match.

`GetByID` now wraps the existing `account.ErrAccountNotFound`, matching the
convention the webhook workers already use to distinguish a deleted account from
a transient DB error. The message is unchanged, so nothing reading the text
moves. The roll-up producer translates it into `insightsrollup.ErrAccountGone`,
and the worker treats that as a terminal skip.

The skip returns before any write to `insights_rollup_state`. Ordering is the
whole fix: there is no coverage to claim, and after a hard delete the row that
the watermark and the failure would both go into is already gone. A soft-deleted
account takes the same path, since there is equally nothing to roll up.

`ErrAccountGone` is a distinct sentinel rather than a reuse of the auth-failure
skip because the two differ in what survives. Rejected credentials leave an
account whose `consecutive_errors` should keep climbing until someone fixes it;
a deleted account leaves nothing to count on.

Tests cover both halves. The worker test asserts `Work` returns nil and issues no
database write at all, so a future reordering that records the failure first
fails the suite rather than reaching production. The store test pins the join, so
dropping it silently reinstates the fan-out.

## Migration

None. Jobs already retrying complete on their next attempt after deploy.
