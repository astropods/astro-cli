# Add an owner column to accounts

## Summary

Nothing records who owns an account. WorkOS holds an `owner` role slug, but Astro
never stores it, so server code that needs the owner off the request path guesses
from `account_members`. This adds the column and the job that populates it.

Nothing reads the column yet. It ships ahead of the code so that by the time a
reader exists, every account already has an owner. The change is additive: it
adds no constraint, so it cannot reject a write the server makes today.

## Design

`accounts.owner_user_id` names the single user who owns an account, indexed for
the reverse lookup.

The column is nullable and carries no foreign key. Both would be correct against
the finished system and wrong against this one: the server does not write the
column yet, so `NOT NULL` would reject every account it creates, and a `RESTRICT`
foreign key would reject a member removal it currently allows. Those constraints
land with the reader.

### Backfill

`AccountOwnerBackfillWorker` runs on the maintenance queue, enqueued once when
the queue starts. It resolves owners in two steps, cheapest first.

Accounts with exactly one member are settled in a single statement. That covers
every personal account and any org never shared, and needs no WorkOS call
because one member cannot be the wrong answer.

The rest have several members, so the job asks WorkOS for the holder of the
`owner` role. Exactly one active owner resolves the account. Zero or several are
logged per account and left alone, and the completion line reports the count:

```
account owner backfill: completed sole_member=812 workos_owner=44 unresolved=3
```

An unresolved account is a decision, not a rule. Falling back to "earliest
member" would silently reassign any org that already moved ownership through the
promote-then-demote pair the member endpoints allow today.

The pass only touches NULL owners, so it is idempotent and never revisits an
account whose owner was set deliberately. That is also why one pass per process
start is enough rather than a schedule: the work has a finite end, and a deploy
picks up anything an earlier pass left, including accounts created before the
server writes the column itself.

## Migration

None. Applying the schema and deploying the server is the whole change; the
backfill runs itself.

Check the completion log for a non-zero `unresolved` count, and settle those
accounts before the reader ships:

```sql
SELECT id, name FROM accounts WHERE owner_user_id IS NULL AND deleted_at IS NULL;
```
