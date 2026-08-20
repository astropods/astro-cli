# Fix: revoke Langfuse API keys on account purge

## Summary

Account purge could never finish once it reached Langfuse cleanup. `DeleteProject`
soft-deleted the project's API keys with `UPDATE api_keys SET deleted_at = ...`,
but Langfuse's `api_keys` table has no `deleted_at` column. Postgres rejected the
statement, the transaction rolled back, and the purge worker failed on every tick:

```
Failed to purge account, will retry next tick
error="delete langfuse project: delete api keys: pq: column \"deleted_at\" does not exist"
```

The purge job runs before the account hard-delete, so a stuck Langfuse step held
back the whole teardown indefinitely.

## Design

Only `projects` carries `deleted_at` in Langfuse's schema; `api_keys` does not.
`DeleteProject` now hard-deletes the key rows and keeps the project soft-delete:

```sql
DELETE FROM api_keys WHERE project_id = $1;
UPDATE projects SET deleted_at = now() WHERE id = $2 AND deleted_at IS NULL;
```

Removing the rows is also the only thing that revokes the credentials. Langfuse
authenticates a request by looking up `fast_hashed_secret_key` in `api_keys`
without filtering on a delete marker, so a soft-delete column would leave the
purged account's keys accepting traces.

Both statements stay idempotent under retry. The delete matches no rows the
second time, and the project update keeps its `deleted_at IS NULL` guard, so a
re-run after a partial failure still commits.

Tests cover the delete shape, retry idempotency, and rollback on either
statement failing. The happy-path test pins the `DELETE FROM api_keys` text, so
reintroducing a soft-delete against that table fails the suite instead of
reaching production.

## Migration

None. Accounts already stuck in the purge queue complete on their next tick.
Langfuse projects that were soft-deleted while their keys survived lose those
keys the next time the worker runs.
