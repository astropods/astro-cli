# Make account ownership an invariant

## Summary

`accounts.owner_user_id` exists and every writer populates it. Ownership is still
a convention the server maintains, so a bug that skips the column or removes the
owner's membership leaves the account in a state nothing rejects. This turns the
convention into a rule the database enforces.

## Design

The column becomes `NOT NULL` and gains a composite foreign key to
`account_members(account_id, user_id)`:

```sql
FOREIGN KEY (id, owner_user_id) REFERENCES account_members(account_id, user_id)
  ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
```

Three invariants stop depending on application code being correct:

| Invariant | Enforced by |
|---|---|
| Exactly one owner | the column holds one value |
| The owner is a member of the account | the composite FK |
| The owner cannot be removed | `ON DELETE RESTRICT` |

The constraint is the schema file's first `ALTER TABLE`, which the cycle forces:
`accounts` and `account_members` each reference the other, and Atlas replays the
file in order, so neither can name the other inline. The file stays a desired
state. Atlas normalizes every foreign key on `accounts` into a trailing
`ALTER TABLE` regardless of how the source writes it.

Deferral matters in two places: an account and its first member insert in one
transaction and reference each other, and deleting an account cascades the
membership away. Checking at commit lets both settle as a whole. `NOT NULL` is
checked per statement instead, which is why account creation names the owner in
its initial `INSERT` rather than a follow-up `UPDATE`.

With NULL no longer reachable, `OwnerUserID` reads the column as a plain string.

### The ownership audit checks retire

`account.no_members`, `account.no_owner` and `account.owner_not_member` each
describe a row the constraints now reject, so none of them can report anything
again. They existed to measure how far the data was from this change, and they go
with it. The audit keeps its four remaining checks.

## Migration

This apply can be rejected, unlike the ones before it. Both constraints validate
existing rows, so confirm the table already satisfies them:

```sql
SELECT count(*) FROM accounts a
 WHERE a.deleted_at IS NULL
   AND (a.owner_user_id IS NULL
        OR NOT EXISTS (SELECT 1 FROM account_members m
                        WHERE m.account_id = a.id AND m.user_id = a.owner_user_id));
```

Run this before deploying. The currently deployed audit answers the same question
under `account.no_owner` and `account.owner_not_member`, so a clean audit means
the apply succeeds. Both checks retire with this change, which is why the query
above is the durable form.

Settle any account the query returns first. An account with no owner needs one
chosen, and an owner who is not a member needs either the membership restored or
ownership transferred.
