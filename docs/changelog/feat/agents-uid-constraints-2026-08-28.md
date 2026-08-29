# Make a blueprint's authorization identity a real column constraint

## Summary

`agents.uid` is the immutable external ID that WorkOS authorization uses to name
a blueprint. It shipped nullable, with no default and no unique constraint,
because the column was added ahead of the data that fills it: a volatile
`DEFAULT gen_random_uuid()` on `ADD COLUMN` would have rewritten the table under
an `ACCESS EXCLUSIVE` lock, and `NOT NULL` could not hold until every historical
row had an ID.

Both conditions are met now. Every write path supplies a UUID, and the
authorization backfill fills the historical rows. This change turns the
convention into a constraint, so a blueprint cannot exist without an
authorization identity and two blueprints cannot share one.

## Design

`uid` becomes `NOT NULL DEFAULT gen_random_uuid()` with a unique constraint. The
default is the safety net for any insert that does not name an ID; the unique
constraint is what makes the column usable as an external identity, since a
duplicate would point two blueprints at one WorkOS resource.

The two blueprint upserts previously wrote `NULLIF($2, '')::uuid`, which turns a
caller that supplies no ID into an explicit `NULL`. An explicit `NULL` is not
replaced by a column default, so that write would now fail. They coalesce to
`gen_random_uuid()` instead, which keeps "no ID supplied" working and keeps the
rule in the query rather than at each call site. Re-registering an existing
blueprint still keeps its original ID.

Nothing else changes. Reads that tolerate a null `uid` stay as they are, because
the schema and the binary roll out separately and a schema rollback has to
remain survivable.

## Migration

**Confirm the column is filled before merging.** The deploy applies `schema.sql`
before the new pods roll, so nothing in this change can fill the column on the
way through:

```sql
SELECT count(*) FROM agents WHERE uid IS NULL;
```

Zero and the migration applies cleanly. Anything else and Atlas fails the apply
with `column "uid" of relation "agents" contains null values` and changes
nothing, blocking that deploy until a real (not dry run) authorization backfill
has run from Queen. Note that a dry-run backfill also records `succeeded` in
`authorization_admin_operations`, so the operation log does not answer this
question; the count above does.
