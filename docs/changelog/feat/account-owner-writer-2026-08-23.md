# Astro owns account ownership, WorkOS mirrors it

## Summary

`accounts.owner_user_id` had no writer. A startup job derived it from the WorkOS
`owner` role, so ownership lived in a system that permits zero owners or several
and answers by paginated list. Astro now decides ownership, WorkOS reflects the
decision, and the backfill job is gone.

## Design

Account creation writes the owner in the same transaction as the first member,
so an account is never briefly ownerless.

After creation the column decides ownership, and nothing reads the WorkOS
`owner` role to answer the question. The role becomes a projection: Astro writes
it so the WorkOS console agrees, and never consults it.

That inversion is what the column was for. Deriving ownership from WorkOS meant
an org could hold two owner roles at once, or none, and the answer changed
depending on which page of memberships a caller happened to read.

| Question | Answered by |
|---|---|
| Who owns this account? | `accounts.owner_user_id` |
| May this caller transfer ownership? | Their user id equals that column |
| What does the WorkOS console show? | Whatever Astro last projected |

Assigning the owner role through the member endpoint is a transfer. It moves the
column, grants the WorkOS role to the new owner, and demotes the previous owner
to `admin`, so a second owner role cannot exist. Only the current owner may do
it.

Ownership cannot be granted any other way. Adding a member as owner and inviting
one as owner are both rejected: joining an account is not acquiring it. Removing
or demoting the recorded owner is rejected too, so an account with an owner
cannot lose it by attrition. Transfer first, then remove.

The write order follows the authority. The column moves first because it decides
the answer, and the WorkOS role writes follow. A failed role write is logged
rather than returned: the transfer already happened, and the next transfer
repairs the projection.

### Only deletion stays owner-only

`org:admin` gated fifteen actions, and WorkOS grants it to the owner alone. An
org admin could manage members and billing but could not rename the account,
read its audit log, or manage access groups. That is a wider reading of
"irreversible" than the word supports.

Account settings, avatars, the audit log, and access groups now require
`org:manage`, which both owners and admins carry. Deleting an account keeps
`org:admin`: it archives the billing customer and tears down every deployment,
and nothing undoes it once the retention window passes.

The fine-grained-access experiment carried its own `org:admin` check on top of
its route group. It now gates on the group like every other switch, and the
per-experiment permission hook is deleted with it, since nothing else used it.

Removing a member with no WorkOS membership took a shortcut past the owner
guard, which is the one path that could still empty an account. It now runs
under the same lock and keeps the last member. `RemoveUserFromAllAccounts` is
deleted; it deleted memberships without touching the accounts that held them.

Org creation records the creator's WorkOS membership id after WorkOS returns it.
That write no longer discards its error. The request still succeeds, because the
org and its membership exist and login sync writes the id on the creator's next
sign-in, but the drift is now visible in the log rather than silent.

### Membership lookups read the whole organization

`ListAllMemberships` follows the `after` cursor to the end of the org, and the
pending-member lookup uses it. A single page of 100 could not find a membership
past the cursor, so removing a pending member from a large org failed with
"member not found". `ListMemberships` keeps its page-at-a-time shape for callers
that render a page.

Counting owners is gone entirely, which is the deeper fix: the count only
existed because ownership lived in WorkOS, where it could be zero or several.

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
