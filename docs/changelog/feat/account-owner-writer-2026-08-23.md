# Write the account owner on the paths that decide it

## Summary

`accounts.owner_user_id` had no writer. A startup job derived it instead, which
meant every account created after a pass sat with a NULL owner until the next
deploy. The write paths now set the column at the moment ownership is decided,
and the backfill job is gone.

## Design

Account creation writes the owner in the same transaction as the first member,
so an account is never briefly ownerless.

After creation the column follows the WorkOS `owner` role, because WorkOS is
where ownership changes:

| Event | Effect on the column |
|---|---|
| Member promoted to owner | Becomes the owner of record |
| Recorded owner demoted or removed | Passes to the earliest-joined remaining active owner |
| Member added as owner | Recorded only if the account has none |
| Owner logs in and the account has none recorded | Recorded |

A promotion takes the record because an admin promoting someone is stating who
owns the account. An addition does not, because inviting a second owner is not a
transfer. Demotion and removal cannot leave the column pointing at someone who
is no longer an owner, so ownership passes on, and the last-owner guard
guarantees a successor exists.

Login sync recording an owner is what replaces the backfill's WorkOS lookup. An
account that somehow reaches an ownerless state settles the next time its owner
signs in, rather than waiting for a deploy.

### Counting owners reads the whole organization

Every caller that counts the `owner` role now uses `ListAllMemberships`, which
follows the `after` cursor to the end of the org. Reading one page of 100 was
wrong in three places: the last-owner guard blocked a legitimate demotion or
removal in any org larger than a page, the pending-member lookup could not find
a membership past the cursor, and the retired backfill reported "no active
WorkOS owner" for an org that had two. `ListMemberships` keeps its
page-at-a-time shape for callers that render a page.

## Migration

Set an owner on any account created since the last backfill pass, then drop the
job rows for the retired kind:

```sql
UPDATE accounts a
   SET owner_user_id = m.user_id, updated_at = now()
  FROM (
    SELECT account_id, min(user_id) AS user_id
      FROM account_members GROUP BY account_id HAVING count(*) = 1
  ) m
 WHERE m.account_id = a.id AND a.owner_user_id IS NULL AND a.deleted_at IS NULL;

DELETE FROM river_job WHERE kind = 'account.owner_backfill';
```

Accounts with several members and no owner need one chosen by hand. The column
stays nullable and carries no foreign key; both land with the reader.
